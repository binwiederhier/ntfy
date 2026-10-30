package limit

import (
	"context"

	"heckel.io/ntfy/v2/db"
)

// Timestamps come from the database clock, never from a node's (see Store). The upsert is one
// fixed statement per chunk (unnest), because SQL built per call bloats pgx's statement cache.
// Same period: add; a newer one: replace; an older one (a leftover): add to the stored period.
const (
	postgresSelectClockQuery = `SELECT EXTRACT(EPOCH FROM now())::BIGINT`
	postgresSelectSinceQuery = `SELECT key, period_start, messages, emails, calls FROM usage WHERE updated_at >= $1`
	postgresUpsertQuery      = `
		INSERT INTO usage AS u (key, period_start, messages, emails, calls, updated_at)
		SELECT key, ps, m, e, c, EXTRACT(EPOCH FROM now())::BIGINT
		FROM unnest($1::TEXT[], $2::BIGINT[], $3::BIGINT[], $4::BIGINT[], $5::BIGINT[]) AS d(key, ps, m, e, c)
		ON CONFLICT (key) DO UPDATE SET
			messages = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.messages + EXCLUDED.messages ELSE EXCLUDED.messages END,
			emails = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.emails + EXCLUDED.emails ELSE EXCLUDED.emails END,
			calls = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.calls + EXCLUDED.calls ELSE EXCLUDED.calls END,
			period_start = GREATEST(u.period_start, EXCLUDED.period_start),
			updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM usage WHERE key IN (SELECT key FROM usage WHERE updated_at < $1 LIMIT $2)`
)

// NewPostgresStore returns the store on the shared pool; the usage table is the user schema's
// (see package user, which creates and migrates it)
func NewPostgresStore(d *db.DB) *Store {
	s := &Store{
		db: d,
		queries: &queries{
			selectClock: postgresSelectClockQuery,
			selectSince: postgresSelectSinceQuery,
			upsert:      postgresUpsertQuery,
			prune:       postgresPruneQuery,
		},
	}
	s.writeChunk = s.writePostgres
	return s
}

// writePostgres writes the rows in one statement, as arrays
func (s *Store) writePostgres(ctx context.Context, rows []usageRow) error {
	keys := make([]string, len(rows))
	periodStarts := make([]int64, len(rows))
	var counts [numDaily][]int64
	for i := range counts {
		counts[i] = make([]int64, len(rows))
	}
	for i, r := range rows {
		keys[i], periodStarts[i] = r.key, r.periodStart
		for j := range counts {
			counts[j][i] = r.counts[j]
		}
	}
	_, err := s.db.ExecContext(ctx, s.queries.upsert, keys, periodStarts, counts[0], counts[1], counts[2])
	return err
}
