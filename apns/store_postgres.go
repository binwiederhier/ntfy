package apns

import (
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	postgresUpsertSubscriptionQuery = `
		INSERT INTO apns_subscription (token, topic, user_id, subscriber_ip, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (token, topic)
		DO UPDATE SET user_id = EXCLUDED.user_id, subscriber_ip = EXCLUDED.subscriber_ip, updated_at = EXCLUDED.updated_at
	`
	postgresDeleteSubscriptionQuery = `
		DELETE FROM apns_subscription WHERE token = $1 AND topic = $2
	`
	postgresDeleteSubscriptionByTokenQuery = `
		DELETE FROM apns_subscription WHERE token = $1
	`
	postgresSelectSubscriptionsForTopicQuery = `
		SELECT token FROM apns_subscription WHERE topic = $1
	`
)

const (
	postgresCurrentSchemaVersion = 1
)

var (
	postgresCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS apns_subscription (
			token TEXT NOT NULL,
			topic TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (token, topic)
		);
		CREATE INDEX IF NOT EXISTS idx_apns_topic ON apns_subscription (topic);
	`)
)

// NewPostgresStore creates a new PostgreSQL-backed APNs store using an existing connection pool.
func NewPostgresStore(d *db.DB) (*Store, error) {
	if err := schema.Migrate(d.Primary(), schema.Postgres, schemaStore, postgresCurrentSchemaVersion, postgresCreateTables, nil); err != nil {
		return nil, err
	}
	return &Store{
		db: d,
		queries: queries{
			upsertSubscription:          postgresUpsertSubscriptionQuery,
			deleteSubscription:          postgresDeleteSubscriptionQuery,
			deleteSubscriptionByToken:   postgresDeleteSubscriptionByTokenQuery,
			selectSubscriptionsForTopic: postgresSelectSubscriptionsForTopicQuery,
		},
	}, nil
}
