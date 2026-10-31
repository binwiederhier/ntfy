package limit

import (
	"context"
	"errors"
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

var (
	// errMaybeWritten wraps a chunk's error when the chunk may have been written anyway (a timeout
	// after the statement was sent), so the caller counts it as written rather than writing it twice
	errMaybeWritten = errors.New("chunk may have been written")
)

// usageRow is one key's daily counts in one period: a delta on the way to the store, the row's
// totals on the way back
type usageRow struct {
	key         string
	periodStart int64 // Unix seconds (see periodStart)
	counts      [numDaily]int64
}

// rateVisitorRow is one topic's row of the topic table. On the way to the store, an empty key
// keeps the stored rate visitor and only moves seen_at forward (a touch).
type rateVisitorRow struct {
	topic  string
	key    string
	userID string
	seenAt int64 // Unix seconds
}

// Store reads and writes the usage table, on SQLite (the user database) or Postgres. Rows are
// stamped (updated_at) with the writing node's clock, like the reader's watermark, so the
// database's clock does not matter. Only the service's sync worker calls it.
type Store struct {
	db                    *db.DB
	queries               *queries
	writeChunk            func(ctx context.Context, rows []usageRow, now int64) error       // Dialect specific: unnest on Postgres, a prepared statement per row on SQLite
	writeRateVisitorChunk func(ctx context.Context, rows []rateVisitorRow, now int64) error // Dialect specific, as writeChunk
}

// queries are the dialect's SQL statements
type queries struct {
	selectSince             string
	upsert                  string
	prune                   string
	selectRateVisitorsSince string
	upsertRateVisitor       string
	pruneRateVisitors       string
}

// write adds the rows' counts to their keys' rows if the periods match, or replaces them if the
// row's period is older. Rows of an older period than the stored one are added to the stored
// one (a node's leftover of a finished period, see the usage queries). Rows are written sorted
// by key in chunks, so two nodes lock overlapping keys in the same order; it returns how many
// rows (in sorted order) are committed: on an error, the chunks before it are, plus the failed
// chunk if it may have been written anyway (errMaybeWritten). The rows' updated_at is now (unix
// seconds).
func (s *Store) write(ctx context.Context, rows []usageRow, now int64) (int, error) {
	slices.SortFunc(rows, func(a, b usageRow) int {
		return strings.Compare(a.key, b.key)
	})
	written := 0
	for chunk := range slices.Chunk(rows, writeChunkRows) {
		if err := s.writeChunk(ctx, chunk, now); err != nil {
			if errors.Is(err, errMaybeWritten) {
				written += len(chunk)
			}
			return written, err
		}
		written += len(chunk)
	}
	return written, nil
}

// readSince calls fn for every row written at or after the given time (unix seconds), streaming them
// from the primary (a replica may not have this node's own writes yet)
func (s *Store) readSince(ctx context.Context, since int64, fn func(r *usageRow)) error {
	rows, err := s.db.QueryContext(ctx, s.queries.selectSince, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	var r usageRow
	for rows.Next() {
		if err := rows.Scan(&r.key, &r.periodStart, &r.counts[0], &r.counts[1], &r.counts[2]); err != nil {
			return err
		}
		fn(&r)
	}
	return rows.Err()
}

// prune deletes the rows not written since the given time (unix seconds), in chunks, and returns how
// many it deleted
func (s *Store) prune(ctx context.Context, before int64) (int64, error) {
	return s.pruneChunked(ctx, s.queries.prune, before)
}

// pruneChunked runs the prune query (age, chunk size) until a chunk deletes fewer rows than the
// chunk size, and returns how many rows it deleted
func (s *Store) pruneChunked(ctx context.Context, query string, before int64) (int64, error) {
	var deleted int64
	for {
		result, err := s.db.ExecContext(ctx, query, before, pruneChunkRows)
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

// writeRateVisitors writes the rate visitor rows, sorted by topic in chunks (see write), stamped with now
// (unix seconds). A set replaces the stored rate visitor and seen_at; a touch keeps the rate visitor and moves
// seen_at forward. Rows are state, not deltas, so a failed write is simply written again.
func (s *Store) writeRateVisitors(ctx context.Context, rows []rateVisitorRow, now int64) error {
	slices.SortFunc(rows, func(a, b rateVisitorRow) int {
		return strings.Compare(a.topic, b.topic)
	})
	for chunk := range slices.Chunk(rows, writeChunkRows) {
		if err := s.writeRateVisitorChunk(ctx, chunk, now); err != nil {
			return err
		}
	}
	return nil
}

// readRateVisitorsSince calls fn for every rate visitor row written at or after the given time (unix seconds),
// streaming them from the primary
func (s *Store) readRateVisitorsSince(ctx context.Context, since int64, fn func(r *rateVisitorRow)) error {
	rows, err := s.db.QueryContext(ctx, s.queries.selectRateVisitorsSince, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	var r rateVisitorRow
	for rows.Next() {
		if err := rows.Scan(&r.topic, &r.key, &r.userID, &r.seenAt); err != nil {
			return err
		}
		fn(&r)
	}
	return rows.Err()
}

// pruneRateVisitors deletes the rate visitor rows not written since the given time (unix seconds), in chunks,
// and returns how many it deleted
func (s *Store) pruneRateVisitors(ctx context.Context, before int64) (int64, error) {
	return s.pruneChunked(ctx, s.queries.pruneRateVisitors, before)
}
