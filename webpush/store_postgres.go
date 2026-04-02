package webpush

import (
	"database/sql"
	"fmt"

	"heckel.io/ntfy/v2/db"
)

const (
	postgresCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS webpush_subscription (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL UNIQUE,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			warned_at BIGINT NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_webpush_subscriber_ip ON webpush_subscription (subscriber_ip);
		CREATE INDEX IF NOT EXISTS idx_webpush_updated_at ON webpush_subscription (updated_at);
		CREATE INDEX IF NOT EXISTS idx_webpush_user_id ON webpush_subscription (user_id);
		CREATE TABLE IF NOT EXISTS webpush_subscription_topic (
			subscription_id TEXT NOT NULL REFERENCES webpush_subscription (id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			PRIMARY KEY (subscription_id, topic)
		);
		CREATE INDEX IF NOT EXISTS idx_webpush_topic ON webpush_subscription_topic (topic);
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
	`

	postgresSelectSubscriptionIDByEndpointQuery        = `SELECT id FROM webpush_subscription WHERE endpoint = $1`
	postgresSelectSubscriptionCountBySubscriberIPQuery = `SELECT COUNT(*) FROM webpush_subscription WHERE subscriber_ip = $1`
	postgresSelectSubscriptionsForTopicQuery           = `
		SELECT s.id, s.endpoint, s.key_auth, s.key_p256dh, s.user_id
		FROM webpush_subscription_topic st
		JOIN webpush_subscription s ON s.id = st.subscription_id
		WHERE st.topic = $1
		ORDER BY s.endpoint
	`
	postgresSelectSubscriptionsExpiringSoonQuery = `
		SELECT id, endpoint, key_auth, key_p256dh, user_id
		FROM webpush_subscription
		WHERE warned_at = 0 AND updated_at <= $1
	`
	postgresUpsertSubscriptionQuery = `
		INSERT INTO webpush_subscription (id, endpoint, key_auth, key_p256dh, user_id, subscriber_ip, updated_at, warned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (endpoint)
		DO UPDATE SET key_auth = excluded.key_auth, key_p256dh = excluded.key_p256dh, user_id = excluded.user_id, subscriber_ip = excluded.subscriber_ip, updated_at = excluded.updated_at, warned_at = excluded.warned_at
		RETURNING id
	`
	postgresUpdateSubscriptionWarningSentQuery = `UPDATE webpush_subscription SET warned_at = $1 WHERE id = $2`
	postgresUpdateSubscriptionUpdatedAtQuery   = `UPDATE webpush_subscription SET updated_at = $1 WHERE endpoint = $2`
	postgresDeleteSubscriptionByEndpointQuery  = `DELETE FROM webpush_subscription WHERE endpoint = $1`
	postgresDeleteSubscriptionByUserIDQuery    = `DELETE FROM webpush_subscription WHERE user_id = $1`
	postgresDeleteSubscriptionByAgeQuery       = `DELETE FROM webpush_subscription WHERE updated_at <= $1`

	postgresInsertSubscriptionTopicQuery                    = `INSERT INTO webpush_subscription_topic (subscription_id, topic) VALUES ($1, $2)`
	postgresDeleteSubscriptionTopicAllQuery                 = `DELETE FROM webpush_subscription_topic WHERE subscription_id = $1`
	postgresDeleteSubscriptionTopicWithoutSubscriptionQuery = `DELETE FROM webpush_subscription_topic WHERE subscription_id NOT IN (SELECT id FROM webpush_subscription)`
)

// PostgreSQL schema management queries
const (
	pgCurrentSchemaVersion           = 1
	postgresInsertSchemaVersionQuery = `INSERT INTO schema_version (store, version) VALUES ('webpush', $1)`
	postgresSelectSchemaVersionQuery = `SELECT version FROM schema_version WHERE store = 'webpush'`
)

// NewPostgresStore creates a new PostgreSQL-backed web push store using an existing database connection pool.
func NewPostgresStore(d *db.DB) (*Store, error) {
	if err := setupPostgres(d.Primary()); err != nil {
		return nil, err
	}
	return &Store{
		db: d,
		queries: queries{
			selectSubscriptionIDByEndpoint:             postgresSelectSubscriptionIDByEndpointQuery,
			selectSubscriptionCountBySubscriberIP:      postgresSelectSubscriptionCountBySubscriberIPQuery,
			selectSubscriptionsForTopic:                postgresSelectSubscriptionsForTopicQuery,
			selectSubscriptionsExpiringSoon:            postgresSelectSubscriptionsExpiringSoonQuery,
			upsertSubscription:                         postgresUpsertSubscriptionQuery,
			updateSubscriptionWarningSent:              postgresUpdateSubscriptionWarningSentQuery,
			updateSubscriptionUpdatedAt:                postgresUpdateSubscriptionUpdatedAtQuery,
			deleteSubscriptionByEndpoint:               postgresDeleteSubscriptionByEndpointQuery,
			deleteSubscriptionByUserID:                 postgresDeleteSubscriptionByUserIDQuery,
			deleteSubscriptionByAge:                    postgresDeleteSubscriptionByAgeQuery,
			insertSubscriptionTopic:                    postgresInsertSubscriptionTopicQuery,
			deleteSubscriptionTopicAll:                 postgresDeleteSubscriptionTopicAllQuery,
			deleteSubscriptionTopicWithoutSubscription: postgresDeleteSubscriptionTopicWithoutSubscriptionQuery,
		},
	}, nil
}

func setupPostgres(d *sql.DB) error {
	var schemaVersion int
	err := d.QueryRow(postgresSelectSchemaVersionQuery).Scan(&schemaVersion)
	if err != nil {
		return setupNewPostgres(d)
	}
	if schemaVersion > pgCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, pgCurrentSchemaVersion)
	}
	return nil
}

func setupNewPostgres(d *sql.DB) error {
	return db.ExecTx(d, func(tx *sql.Tx) error {
		if _, err := tx.Exec(postgresCreateTablesQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(postgresInsertSchemaVersionQuery, pgCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}
