package apns

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	sqliteBuiltinStartupQueries = `
		PRAGMA foreign_keys = ON;
	`

	sqliteUpsertSubscriptionQuery = `
		INSERT INTO apns_subscription (token, topic, user_id, subscriber_ip, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (token, topic)
		DO UPDATE SET user_id = excluded.user_id, subscriber_ip = excluded.subscriber_ip, updated_at = excluded.updated_at
	`
	sqliteDeleteSubscriptionQuery = `
		DELETE FROM apns_subscription WHERE token = ? AND topic = ?
	`
	sqliteDeleteSubscriptionByTokenQuery = `
		DELETE FROM apns_subscription WHERE token = ?
	`
	sqliteSelectSubscriptionsForTopicQuery = `
		SELECT token FROM apns_subscription WHERE topic = ?
	`
)

const (
	sqliteCurrentSchemaVersion = 1
)

var (
	sqliteCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS apns_subscription (
			token TEXT NOT NULL,
			topic TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at INT NOT NULL,
			PRIMARY KEY (token, topic)
		);
		CREATE INDEX IF NOT EXISTS idx_apns_topic ON apns_subscription (topic);
	`)
)

// NewSQLiteStore creates a new SQLite-backed APNs store.
func NewSQLiteStore(filename, startupQueries string) (*Store, error) {
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := schema.Migrate(d, schema.SQLite, schemaStore, sqliteCurrentSchemaVersion, sqliteCreateTables, nil); err != nil {
		return nil, err
	}
	if err := runSQLiteStartupQueries(d, startupQueries); err != nil {
		return nil, err
	}
	return &Store{
		db: db.New(&db.Host{DB: d}, nil),
		queries: queries{
			upsertSubscription:          sqliteUpsertSubscriptionQuery,
			deleteSubscription:          sqliteDeleteSubscriptionQuery,
			deleteSubscriptionByToken:   sqliteDeleteSubscriptionByTokenQuery,
			selectSubscriptionsForTopic: sqliteSelectSubscriptionsForTopicQuery,
		},
	}, nil
}

func runSQLiteStartupQueries(db *sql.DB, startupQueries string) error {
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}
	if _, err := db.Exec(sqliteBuiltinStartupQueries); err != nil {
		return err
	}
	return nil
}
