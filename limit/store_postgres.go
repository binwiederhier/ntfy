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
	postgresSelectSinceQuery = `SELECT key, kind, count FROM usage WHERE day = $1 AND updated_at >= $2`
	postgresSelectKeysQuery  = `SELECT key, kind, count FROM usage WHERE day = $1 AND key = ANY($2)`
	postgresUpsertQuery      = `
		INSERT INTO usage (key, day, kind, count, updated_at)
		SELECT k, $1, kd, c, EXTRACT(EPOCH FROM now())::BIGINT FROM unnest($2::TEXT[], $3::TEXT[], $4::BIGINT[]) AS t(k, kd, c)
		ON CONFLICT (key, day, kind) DO UPDATE SET count = usage.count + EXCLUDED.count, updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM usage WHERE day < $1`

	postgresUpsertTopicSubscriberQuery = `
		INSERT INTO topic (id, created_at, last_subscribed_at, last_subscriber_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			last_subscribed_at = EXCLUDED.last_subscribed_at,
			last_subscriber_key = EXCLUDED.last_subscriber_key,
			rate_visitor_seen_at = CASE WHEN topic.rate_visitor_key = EXCLUDED.last_subscriber_key THEN EXCLUDED.last_subscribed_at ELSE topic.rate_visitor_seen_at END
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

// The topic table: who a topic bills to (the UnifiedPush rate visitor), when it was created,
// last subscribed to and last published on (see topics.go)
const (
	postgresCreateTopicTableQuery = `
		CREATE TABLE IF NOT EXISTS topic (
			id TEXT PRIMARY KEY,
			created_at BIGINT NOT NULL,
			last_subscribed_at BIGINT NOT NULL DEFAULT 0,
			last_subscriber_key TEXT NOT NULL DEFAULT '',
			last_published_at BIGINT NOT NULL DEFAULT 0,
			last_publisher_key TEXT NOT NULL DEFAULT '',
			rate_visitor_key TEXT NOT NULL DEFAULT '',
			rate_visitor_uid TEXT NOT NULL DEFAULT '',
			rate_visitor_seen_at BIGINT NOT NULL DEFAULT 0
		);
	`
)

// Schema
var (
	postgresCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS usage (
			key TEXT NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			count BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (key, day, kind)
		);
		CREATE INDEX IF NOT EXISTS idx_usage_day_updated_at ON usage (day, updated_at);
	` + postgresCreateTopicTableQuery)
	postgresMigrations = schema.Migrations{
		{From: 1, To: 2}: schema.Query(postgresCreateTopicTableQuery), // The topic table (who a topic bills to, when it was last subscribed to)
	}
)

// NewPostgresStore sets up the usage table on the shared pool
func NewPostgresStore(d *db.DB) (*Store, error) {
	if err := schema.Migrate(d.Primary(), schema.Postgres, schemaStore, postgresCreateTables, postgresMigrations); err != nil {
		return nil, err
	}
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
	}, nil
}
