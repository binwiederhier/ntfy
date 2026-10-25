package limit

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
	"heckel.io/ntfy/v2/log"
)

const (
	schemaStore = "usage" // Store name in the shared schema_version table (see db/schema)

	// flushChunkRows caps the rows of one upsert statement, so a flush with many keys is a few
	// bounded statements rather than one huge one (SQLite also caps the variables per statement)
	flushChunkRows = 500

	// readKeysChunk caps the keys of one read-back query
	readKeysChunk = 1000
)

// usageRow is one (key, kind, count) of one day: a delta on the way to the store, a sum on the way back
type usageRow struct {
	key   string
	kind  Kind
	count int64
}

// Store persists the daily usage counts in the usage table, on SQLite or Postgres. Timestamps
// (updated_at) come from the database clock, never from a node's, so that nodes with different
// clocks agree on what changed since when.
type Store struct {
	db      *db.DB
	dialect schema.Dialect
	queries queries
	closer  *sql.DB // Closed by Close; nil when the pool is shared and owned by the caller
}

// queries are the dialect's SQL statements
type queries struct {
	selectClock    string
	selectSince    string
	selectKeys     string // Postgres only: the keys as an array parameter
	selectKeysIn   string // SQLite only: prefix of an IN (...) list built per chunk
	upsert         string // Postgres only: one statement for a whole chunk (unnest)
	upsertPrefix   string // SQLite only: prefix of a multi-row VALUES statement built per chunk
	upsertRow      string // SQLite only: one row of it
	upsertConflict string // SQLite only: its ON CONFLICT clause
	prune          string
	topics         topicQueries
}

// topicQueries are the dialect's statements for the topic table (see topics.go)
type topicQueries struct {
	upsertSubscriber  string
	upsertPublish     string
	upsertRateVisitor string
	insert            string
	selectTopic       string
	prune             string
}

// query returns the dialect's statement of the given kind
func (q *topicQueries) query(statement topicStatement) string {
	switch statement {
	case topicUpsertSubscriber:
		return q.upsertSubscriber
	case topicUpsertPublish:
		return q.upsertPublish
	case topicUpsertRateVisitor:
		return q.upsertRateVisitor
	case topicInsert:
		return q.insert
	default:
		return q.prune
	}
}

// clock returns the database's time as unix seconds
func (s *Store) clock(ctx context.Context) (int64, error) {
	var now int64
	err := s.db.QueryRowContext(ctx, s.queries.selectClock).Scan(&now)
	return now, err
}

// flush adds the rows' counts to the day's rows (creating them as needed), in chunks of one
// statement each. It returns how many rows are committed: on an error, the rows before that are.
func (s *Store) flush(ctx context.Context, day string, rows []usageRow) (int, error) {
	written := 0
	for chunk := range slices.Chunk(rows, flushChunkRows) {
		var err error
		if s.dialect == schema.Postgres {
			err = s.flushPostgres(ctx, day, chunk)
		} else {
			err = s.flushSQLite(ctx, day, chunk)
		}
		if err != nil {
			return written, err
		}
		written += len(chunk)
	}
	return written, nil
}

// readSince returns the day's rows written at or after the given database time
func (s *Store) readSince(ctx context.Context, day string, since int64) ([]usageRow, error) {
	rows, err := s.db.QueryContext(ctx, s.queries.selectSince, day, since) // Deliberately the primary: the read must see this node's own just-flushed rows (replicas may lag)
	if err != nil {
		return nil, err
	}
	return scanUsageRows(rows)
}

// readKeys returns the day's rows of the given keys
func (s *Store) readKeys(ctx context.Context, day string, keys []string) ([]usageRow, error) {
	result := make([]usageRow, 0)
	for chunk := range slices.Chunk(keys, readKeysChunk) {
		rows, err := s.readKeysChunk(ctx, day, chunk)
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}
	return result, nil
}

// prune deletes the rows of days before the given one
func (s *Store) prune(ctx context.Context, day string) error {
	_, err := s.db.ExecContext(ctx, s.queries.prune, day)
	return err
}

// Close closes the store's own database, if it has one
func (s *Store) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func (s *Store) flushPostgres(ctx context.Context, day string, rows []usageRow) error {
	keys := make([]string, len(rows))
	kinds := make([]string, len(rows))
	counts := make([]int64, len(rows))
	for i, r := range rows {
		keys[i], kinds[i], counts[i] = r.key, r.kind.String(), r.count
	}
	_, err := s.db.ExecContext(ctx, s.queries.upsert, day, keys, kinds, counts)
	return err
}

func (s *Store) flushSQLite(ctx context.Context, day string, rows []usageRow) error {
	var query strings.Builder
	query.WriteString(s.queries.upsertPrefix)
	args := make([]any, 0, 4*len(rows))
	for i, r := range rows {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteString(s.queries.upsertRow)
		args = append(args, r.key, day, r.kind.String(), r.count)
	}
	query.WriteString(s.queries.upsertConflict)
	_, err := s.db.ExecContext(ctx, query.String(), args...)
	return err
}

func (s *Store) readKeysChunk(ctx context.Context, day string, keys []string) ([]usageRow, error) {
	var rows *sql.Rows
	var err error
	if s.dialect == schema.Postgres {
		rows, err = s.db.QueryContext(ctx, s.queries.selectKeys, day, keys)
	} else {
		args := make([]any, 0, len(keys)+1)
		args = append(args, day)
		for _, key := range keys {
			args = append(args, key)
		}
		query := s.queries.selectKeysIn + strings.Repeat("?, ", len(keys)-1) + "?)"
		rows, err = s.db.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, err
	}
	return scanUsageRows(rows)
}

// scanUsageRows reads (key, kind, count) rows; a kind this build does not know (a newer node
// wrote it) is skipped, not fatal
func scanUsageRows(rows *sql.Rows) ([]usageRow, error) {
	defer rows.Close()
	result := make([]usageRow, 0)
	for rows.Next() {
		var key, kindName string
		var count int64
		if err := rows.Scan(&key, &kindName, &count); err != nil {
			return nil, err
		}
		kind, ok := ParseKind(kindName)
		if !ok {
			log.Tag(tag).Debug("Skipping usage row of unknown kind %s", kindName)
			continue
		}
		result = append(result, usageRow{key: key, kind: kind, count: count})
	}
	return result, rows.Err()
}
