package webpush

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"heckel.io/ntfy/v2/db"
)

const (
	sqliteCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS subscription (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			user_id TEXT NOT NULL,		
			subscriber_ip TEXT NOT NULL,
			updated_at INT NOT NULL,
			warned_at INT NOT NULL DEFAULT 0
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_endpoint ON subscription (endpoint);
		CREATE INDEX IF NOT EXISTS idx_subscriber_ip ON subscription (subscriber_ip);
		CREATE TABLE IF NOT EXISTS subscription_topic (
			subscription_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			PRIMARY KEY (subscription_id, topic),
			FOREIGN KEY (subscription_id) REFERENCES subscription (id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON subscription_topic (topic);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
	`
	sqliteBuiltinStartupQueries = `
		PRAGMA foreign_keys = ON;
	`

	sqliteSelectSubscriptionIDByEndpointQuery        = `SELECT id FROM subscription WHERE endpoint = ?`
	sqliteSelectSubscriptionCountBySubscriberIPQuery = `SELECT COUNT(*) FROM subscription WHERE subscriber_ip = ?`
	sqliteSelectSubscriptionsForTopicQuery           = `
		SELECT id, endpoint, key_auth, key_p256dh, user_id
		FROM subscription_topic st
		JOIN subscription s ON s.id = st.subscription_id
		WHERE st.topic = ?
		ORDER BY endpoint
	`
	sqliteSelectSubscriptionsExpiringSoonQuery = `
		SELECT id, endpoint, key_auth, key_p256dh, user_id 
		FROM subscription 
		WHERE warned_at = 0 AND updated_at <= ?
	`
	sqliteUpsertSubscriptionQuery = `
		INSERT INTO subscription (id, endpoint, key_auth, key_p256dh, user_id, subscriber_ip, updated_at, warned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (endpoint)
		DO UPDATE SET key_auth = excluded.key_auth, key_p256dh = excluded.key_p256dh, user_id = excluded.user_id, subscriber_ip = excluded.subscriber_ip, updated_at = excluded.updated_at, warned_at = excluded.warned_at
		RETURNING id
	`
	sqliteUpdateSubscriptionWarningSentQuery = `UPDATE subscription SET warned_at = ? WHERE id = ?`
	sqliteUpdateSubscriptionUpdatedAtQuery   = `UPDATE subscription SET updated_at = ? WHERE endpoint = ?`
	sqliteDeleteSubscriptionByEndpointQuery  = `DELETE FROM subscription WHERE endpoint = ?`
	sqliteDeleteSubscriptionByUserIDQuery    = `DELETE FROM subscription WHERE user_id = ?`
	sqliteDeleteSubscriptionByAgeQuery       = `DELETE FROM subscription WHERE updated_at <= ?` // Full table scan!

	sqliteInsertSubscriptionTopicQuery                    = `INSERT INTO subscription_topic (subscription_id, topic) VALUES (?, ?)`
	sqliteDeleteSubscriptionTopicAllQuery                 = `DELETE FROM subscription_topic WHERE subscription_id = ?`
	sqliteDeleteSubscriptionTopicWithoutSubscriptionQuery = `DELETE FROM subscription_topic WHERE subscription_id NOT IN (SELECT id FROM subscription)`
)

// SQLite schema management queries
const (
	sqliteCurrentSchemaVersion     = 1
	sqliteInsertSchemaVersionQuery = `INSERT INTO schemaVersion VALUES (1, ?)`
	sqliteSelectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
)

// NewSQLiteStore creates a new SQLite-backed web push store.
func NewSQLiteStore(filename, startupQueries string) (*Store, error) {
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSQLite(d); err != nil {
		return nil, err
	}
	if err := runSQLiteStartupQueries(d, startupQueries); err != nil {
		return nil, err
	}
	return &Store{
		db: db.New(&db.Host{DB: d}, nil),
		queries: queries{
			selectSubscriptionIDByEndpoint:             sqliteSelectSubscriptionIDByEndpointQuery,
			selectSubscriptionCountBySubscriberIP:      sqliteSelectSubscriptionCountBySubscriberIPQuery,
			selectSubscriptionsForTopic:                sqliteSelectSubscriptionsForTopicQuery,
			selectSubscriptionsExpiringSoon:            sqliteSelectSubscriptionsExpiringSoonQuery,
			upsertSubscription:                         sqliteUpsertSubscriptionQuery,
			updateSubscriptionWarningSent:              sqliteUpdateSubscriptionWarningSentQuery,
			updateSubscriptionUpdatedAt:                sqliteUpdateSubscriptionUpdatedAtQuery,
			deleteSubscriptionByEndpoint:               sqliteDeleteSubscriptionByEndpointQuery,
			deleteSubscriptionByUserID:                 sqliteDeleteSubscriptionByUserIDQuery,
			deleteSubscriptionByAge:                    sqliteDeleteSubscriptionByAgeQuery,
			insertSubscriptionTopic:                    sqliteInsertSubscriptionTopicQuery,
			deleteSubscriptionTopicAll:                 sqliteDeleteSubscriptionTopicAllQuery,
			deleteSubscriptionTopicWithoutSubscription: sqliteDeleteSubscriptionTopicWithoutSubscriptionQuery,
		},
	}, nil
}

func setupSQLite(db *sql.DB) error {
	var schemaVersion int
	if err := db.QueryRow(sqliteSelectSchemaVersionQuery).Scan(&schemaVersion); err != nil {
		return setupNewSQLite(db)
	} else if schemaVersion > sqliteCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, sqliteCurrentSchemaVersion)
	}
	return nil
}

func setupNewSQLite(sqlDB *sql.DB) error {
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteCreateTablesQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteInsertSchemaVersionQuery, sqliteCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}

func runSQLiteStartupQueries(db *sql.DB, startupQueries string) error {
	if _, err := db.Exec(startupQueries); err != nil {
		return err
	}
	if _, err := db.Exec(sqliteBuiltinStartupQueries); err != nil {
		return err
	}
	return nil
}
