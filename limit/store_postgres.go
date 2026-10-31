package limit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"heckel.io/ntfy/v2/db"
)

// The upsert is one fixed statement per chunk (unnest), because SQL built per call bloats pgx's statement cache.
// Same period: add; a newer one: replace; an older one (a leftover): add to the stored period.
const (
	postgresSelectSinceQuery = `SELECT key, period_start, messages, emails, calls FROM usage WHERE updated_at >= $1`
	postgresUpsertQuery      = `
		INSERT INTO usage AS u (key, period_start, messages, emails, calls, updated_at)
		SELECT key, ps, m, e, c, $6::BIGINT
		FROM unnest($1::TEXT[], $2::BIGINT[], $3::BIGINT[], $4::BIGINT[], $5::BIGINT[]) AS d(key, ps, m, e, c)
		ON CONFLICT (key) DO UPDATE SET
			messages = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.messages + EXCLUDED.messages ELSE EXCLUDED.messages END,
			emails = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.emails + EXCLUDED.emails ELSE EXCLUDED.emails END,
			calls = CASE WHEN u.period_start >= EXCLUDED.period_start THEN u.calls + EXCLUDED.calls ELSE EXCLUDED.calls END,
			period_start = GREATEST(u.period_start, EXCLUDED.period_start),
			updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM usage WHERE updated_at < $1 AND key IN (SELECT key FROM usage WHERE updated_at < $1 LIMIT $2)` // The outer age check is rechecked on a row updated while the delete waits for it
)

// The payer upsert, one fixed statement per chunk like the usage upsert. A set (a payer key)
// replaces the payer and seen_at; a touch (no payer key) keeps the payer and moves seen_at forward.
const (
	postgresSelectPayersSinceQuery = `SELECT topic, payer_key, payer_user_id, seen_at FROM topic_payer WHERE updated_at >= $1`
	postgresUpsertPayerQuery       = `
		INSERT INTO topic_payer AS p (topic, payer_key, payer_user_id, seen_at, updated_at)
		SELECT t, k, u, s, $5::BIGINT
		FROM unnest($1::TEXT[], $2::TEXT[], $3::TEXT[], $4::BIGINT[]) AS d(t, k, u, s)
		ON CONFLICT (topic) DO UPDATE SET
			payer_key = CASE WHEN EXCLUDED.payer_key = '' THEN p.payer_key ELSE EXCLUDED.payer_key END,
			payer_user_id = CASE WHEN EXCLUDED.payer_key = '' THEN p.payer_user_id ELSE EXCLUDED.payer_user_id END,
			seen_at = CASE WHEN EXCLUDED.payer_key = '' THEN GREATEST(p.seen_at, EXCLUDED.seen_at) ELSE EXCLUDED.seen_at END,
			updated_at = EXCLUDED.updated_at
	`
	postgresPrunePayersQuery = `DELETE FROM topic_payer WHERE updated_at < $1 AND topic IN (SELECT topic FROM topic_payer WHERE updated_at < $1 LIMIT $2)` // As the usage prune
)

// NewPostgresStore returns the store on the shared pool; the usage table is the user schema's
// (see package user, which creates and migrates it)
func NewPostgresStore(d *db.DB) *Store {
	s := &Store{
		db: d,
		queries: &queries{
			selectSince:       postgresSelectSinceQuery,
			upsert:            postgresUpsertQuery,
			prune:             postgresPruneQuery,
			selectPayersSince: postgresSelectPayersSinceQuery,
			upsertPayer:       postgresUpsertPayerQuery,
			prunePayers:       postgresPrunePayersQuery,
		},
	}
	s.writeChunk = s.writePostgres
	s.writePayerChunk = s.writePayersPostgres
	return s
}

// writePostgres writes the rows in one statement, as arrays. A timeout after the statement was
// sent is wrapped in errMaybeWritten, since the commit may have been acknowledged after the deadline.
func (s *Store) writePostgres(ctx context.Context, rows []usageRow, now int64) error {
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
	_, err := s.db.ExecContext(ctx, s.queries.upsert, keys, periodStarts, counts[0], counts[1], counts[2], now)
	if err != nil && mayHaveCommittedPostgres(err) {
		return fmt.Errorf("%w: %w", errMaybeWritten, err)
	}
	return err
}

// writePayersPostgres writes the payer rows in one statement, as arrays
func (s *Store) writePayersPostgres(ctx context.Context, rows []payerRow, now int64) error {
	topics, keys, userIDs := make([]string, len(rows)), make([]string, len(rows)), make([]string, len(rows))
	seenAts := make([]int64, len(rows))
	for i, r := range rows {
		topics[i], keys[i], userIDs[i], seenAts[i] = r.topic, r.key, r.userID, r.seenAt
	}
	_, err := s.db.ExecContext(ctx, s.queries.upsertPayer, topics, keys, userIDs, seenAts, now)
	return err
}

// mayHaveCommittedPostgres reports whether a failed write may have committed anyway: a timeout
// after the statement was sent, e.g. a commit acknowledged after the deadline. An error from the
// server, or one before the statement was sent, means nothing was written.
func mayHaveCommittedPostgres(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) || pgconn.SafeToRetry(err) {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err)
}
