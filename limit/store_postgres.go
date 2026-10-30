package limit

import (
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

// Timestamps come from the database clock, never from a node's: the read-back watermark compares
// them, and node clocks neither agree with each other nor with the database. The upsert is one
// statement for a whole chunk (unnest), because per-call SQL bloats pgx's statement cache.
const (
	postgresSelectClockQuery = `SELECT EXTRACT(EPOCH FROM now())::BIGINT`
	postgresSelectSinceQuery = `SELECT key, period, kind, count FROM usage WHERE updated_at >= $1`
	postgresSelectKeysQuery  = `SELECT key, period, kind, count FROM usage WHERE key = ANY($1)`
	postgresUpsertQuery      = `
		INSERT INTO usage (key, period, kind, count, updated_at)
		SELECT k, p, kd, c, EXTRACT(EPOCH FROM now())::BIGINT FROM unnest($1::TEXT[], $2::BIGINT[], $3::TEXT[], $4::BIGINT[]) AS t(k, p, kd, c)
		ON CONFLICT (key, period, kind) DO UPDATE SET count = usage.count + EXCLUDED.count, updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM usage WHERE updated_at < EXTRACT(EPOCH FROM now())::BIGINT - $1`

	postgresUpsertTopicSubscriberQuery = `
		INSERT INTO topic (id, created_at, last_subscribed_at, last_subscriber_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			last_subscribed_at = EXCLUDED.last_subscribed_at,
			last_subscriber_key = EXCLUDED.last_subscriber_key
	`
	postgresUpsertTopicPublishQuery = `
		INSERT INTO topic (id, created_at, last_published_at, last_publisher_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET last_published_at = EXCLUDED.last_published_at, last_publisher_key = EXCLUDED.last_publisher_key
	`
	postgresUpsertTopicRateVisitorQuery = `
		INSERT INTO topic (id, created_at, rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET rate_visitor_key = EXCLUDED.rate_visitor_key, rate_visitor_uid = EXCLUDED.rate_visitor_uid, rate_visitor_seen_at = EXCLUDED.rate_visitor_seen_at
	`
	postgresInsertTopicQuery = `INSERT INTO topic (id, created_at) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`
	postgresSelectTopicQuery = `
		SELECT id, created_at, last_subscribed_at, last_subscriber_key, last_published_at, last_publisher_key, rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at
		FROM topic WHERE id = $1
	`
	postgresPruneTopicsQuery = `DELETE FROM topic WHERE GREATEST(created_at, last_subscribed_at, last_published_at, rate_visitor_seen_at) < $1`
)

// NewPostgresStore returns the store on the shared pool; the usage table is the user schema's (see
// package user, which creates and migrates it)
func NewPostgresStore(d *db.DB) *Store {
	return &Store{
		db:      d,
		dialect: schema.Postgres,
		queries: queries{
			selectClock: postgresSelectClockQuery,
			selectSince: postgresSelectSinceQuery,
			selectKeys:  postgresSelectKeysQuery,
			upsert:      postgresUpsertQuery,
			prune:       postgresPruneQuery,
			topics: topicQueries{
				upsertSubscriber:  postgresUpsertTopicSubscriberQuery,
				upsertPublish:     postgresUpsertTopicPublishQuery,
				upsertRateVisitor: postgresUpsertTopicRateVisitorQuery,
				insert:            postgresInsertTopicQuery,
				selectTopic:       postgresSelectTopicQuery,
				prune:             postgresPruneTopicsQuery,
			},
		},
	}
}
