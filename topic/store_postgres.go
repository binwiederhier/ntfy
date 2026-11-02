package topic

import (
	"context"

	"heckel.io/ntfy/v2/db"
)

// The upsert is one fixed statement per chunk (unnest), because SQL built per call bloats pgx's
// statement cache. A row without a rate visitor (no ip) keeps the stored one.
const (
	postgresSelectSinceQuery = `SELECT topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at FROM topic WHERE updated_at >= $1`
	postgresSelectOneQuery   = `SELECT topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at FROM topic WHERE topic = $1`
	postgresUpsertQuery      = `
		INSERT INTO topic AS t (topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at)
		SELECT topic, f, s, u, i, $6::BIGINT
		FROM unnest($1::TEXT[], $2::BIGINT[], $3::BIGINT[], $4::TEXT[], $5::TEXT[]) AS d(topic, f, s, u, i)
		ON CONFLICT (topic) DO UPDATE SET
			first_seen_at = LEAST(t.first_seen_at, EXCLUDED.first_seen_at),
			subscribed_at = GREATEST(t.subscribed_at, EXCLUDED.subscribed_at),
			rate_visitor_user_id = CASE WHEN EXCLUDED.rate_visitor_ip = '' THEN t.rate_visitor_user_id ELSE EXCLUDED.rate_visitor_user_id END,
			rate_visitor_ip = CASE WHEN EXCLUDED.rate_visitor_ip = '' THEN t.rate_visitor_ip ELSE EXCLUDED.rate_visitor_ip END,
			updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM topic WHERE updated_at < $1 AND topic IN (SELECT topic FROM topic WHERE updated_at < $1 LIMIT $2)` // The outer age check is rechecked on a row updated while the delete waits for it
)

// NewPostgresStore returns the store on the shared pool; package user creates and migrates the table
func NewPostgresStore(d *db.DB) *Store {
	s := &Store{
		db: d,
		queries: &queries{
			selectSince: postgresSelectSinceQuery,
			selectOne:   postgresSelectOneQuery,
			prune:       postgresPruneQuery,
		},
	}
	s.writeChunk = s.writePostgres
	return s
}

// writePostgres writes the rows in one statement, as arrays
func (s *Store) writePostgres(ctx context.Context, rows []*State, now int64) error {
	topics, userIDs, ips := make([]string, len(rows)), make([]string, len(rows)), make([]string, len(rows))
	firstSeenAts, subscribedAts := make([]int64, len(rows)), make([]int64, len(rows))
	for i, r := range rows {
		topics[i], firstSeenAts[i], subscribedAts[i], userIDs[i], ips[i] = r.Topic, r.FirstSeenAt, r.SubscribedAt, r.RateVisitorUserID, r.RateVisitorIP
	}
	_, err := s.db.ExecContext(ctx, postgresUpsertQuery, topics, firstSeenAts, subscribedAts, userIDs, ips, now)
	return err
}
