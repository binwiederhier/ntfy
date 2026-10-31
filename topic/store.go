package topic

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"heckel.io/ntfy/v2/db"
)

const (
	// writeChunkRows caps the rows of one write statement (Postgres) or transaction (SQLite), so
	// a big sync is a few bounded statements rather than one huge one
	writeChunkRows = 1000

	// pruneChunkRows caps the rows of one prune statement, so no statement holds locks for long
	pruneChunkRows = 10000
)

// row is one topic's row of the topic table. On the way to the store, an empty ip keeps the
// stored rate visitor (see the upsert queries); updated_at is stamped by the store.
type row struct {
	topic        string
	firstSeenAt  int64 // Unix seconds
	subscribedAt int64 // Unix seconds; 0 if never
	userID       string
	ip           string
	updatedAt    int64 // Unix seconds; only on the way back
}

// Store reads and writes the topic table, on SQLite (the user database) or Postgres. Rows are
// stamped (updated_at) with the writing node's clock, like the reader's watermark, so the
// database's clock does not matter. The service's sync worker calls it, and a cold-path
// Refresh.
type Store struct {
	db         *db.DB
	queries    *queries
	writeChunk func(ctx context.Context, rows []row, now int64) error // Dialect specific: unnest on Postgres, a prepared statement per row on SQLite
}

// queries are the dialect's SQL statements
type queries struct {
	selectSince string
	selectOne   string
	upsert      string
	prune       string
}

// write merges the rows into the table (see the upsert queries: first_seen_at keeps the earliest,
// subscribed_at the latest, the rate visitor the last written one, unless the row has none),
// sorted by topic in chunks so two nodes lock overlapping topics in the same order, stamped with
// now (unix seconds). Rows are state, not deltas, so a failed chunk is simply written again.
func (s *Store) write(ctx context.Context, rows []row, now int64) error {
	slices.SortFunc(rows, func(a, b row) int {
		return strings.Compare(a.topic, b.topic)
	})
	for chunk := range slices.Chunk(rows, writeChunkRows) {
		if err := s.writeChunk(ctx, chunk, now); err != nil {
			return err
		}
	}
	return nil
}

// readSince calls fn for every row written at or after the given time (unix seconds), streaming
// them from the primary (a replica may not have this node's own writes yet)
func (s *Store) readSince(ctx context.Context, since int64, fn func(r *row)) error {
	rows, err := s.db.QueryContext(ctx, s.queries.selectSince, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	var r row
	for rows.Next() {
		if err := scanRow(rows, &r); err != nil {
			return err
		}
		fn(&r)
	}
	return rows.Err()
}

// readOne returns the topic's row from the primary, or nil if there is none
func (s *Store) readOne(ctx context.Context, topic string) (*row, error) {
	rows, err := s.db.QueryContext(ctx, s.queries.selectOne, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var r row
	if err := scanRow(rows, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// prune deletes the rows not written since the given time (unix seconds), in chunks, and returns
// how many it deleted
func (s *Store) prune(ctx context.Context, before int64) (int64, error) {
	var deleted int64
	for {
		result, err := s.db.ExecContext(ctx, s.queries.prune, before, pruneChunkRows)
		if err != nil {
			return deleted, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return deleted, err
		}
		deleted += n
		if n < pruneChunkRows {
			return deleted, nil
		}
	}
}

// scanRow reads the current row of a select into r; the selects list the columns in this order
func scanRow(rows *sql.Rows, r *row) error {
	return rows.Scan(&r.topic, &r.firstSeenAt, &r.subscribedAt, &r.userID, &r.ip, &r.updatedAt)
}
