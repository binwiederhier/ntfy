package topic

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"heckel.io/ntfy/v2/db"
)

const (
	writeChunkRows = 1000  // Rows per write statement (Postgres) or transaction (SQLite)
	pruneChunkRows = 10000 // Rows per prune statement, so none holds locks for long
)

// Store reads and writes the topic table, on SQLite (the user database) or Postgres. Rows are
// stamped with the writing node's clock, like the reader's watermark, so the database's does not matter.
type Store struct {
	db         *db.DB
	queries    *queries
	writeChunk func(ctx context.Context, rows []State, now int64) error // Dialect specific: unnest on Postgres, a transaction on SQLite
}

// queries are the dialect's SQL statements
type queries struct {
	selectSince string
	selectOne   string
	prune       string
}

// write merges the rows into the table (see the upsert queries), stamped with now. Rows are
// sorted by topic, so two nodes lock overlapping topics in the same order.
func (s *Store) write(ctx context.Context, rows []State, now int64) error {
	slices.SortFunc(rows, func(a, b State) int {
		return strings.Compare(a.Topic, b.Topic)
	})
	for chunk := range slices.Chunk(rows, writeChunkRows) {
		if err := s.writeChunk(ctx, chunk, now); err != nil {
			return err
		}
	}
	return nil
}

// readSince calls fn for every row written at or after the given time, from the primary (a
// replica may not have this node's own writes yet)
func (s *Store) readSince(ctx context.Context, since int64, fn func(r *State)) error {
	rows, err := s.db.QueryContext(ctx, s.queries.selectSince, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	var r State
	for rows.Next() {
		if err := scanRow(rows, &r); err != nil {
			return err
		}
		fn(&r)
	}
	return rows.Err()
}

// readOne returns the topic's row from the primary, or nil if there is none
func (s *Store) readOne(ctx context.Context, topic string) (*State, error) {
	rows, err := s.db.QueryContext(ctx, s.queries.selectOne, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var r State
	if err := scanRow(rows, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// prune deletes the rows not written since the given time, in chunks, and returns how many it deleted
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
func scanRow(rows *sql.Rows, r *State) error {
	return rows.Scan(&r.Topic, &r.FirstSeenAt, &r.SubscribedAt, &r.RateVisitorUserID, &r.RateVisitorIP, &r.UpdatedAt)
}
