package server

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	createWebPushSubscriptionsTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			username TEXT,
			endpoint TEXT NOT NULL,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON subscriptions (topic);
		CREATE INDEX IF NOT EXISTS idx_endpoint ON subscriptions (endpoint);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_endpoint ON subscriptions (topic, endpoint);
		COMMIT;
	`
	insertWebPushSubscriptionQuery = `
		INSERT OR REPLACE INTO subscriptions (topic, username, endpoint, key_auth, key_p256dh)
		VALUES (?, ?, ?, ?, ?)
	`
	deleteWebPushSubscriptionByEndpointQuery         = `DELETE FROM subscriptions WHERE endpoint = ?`
	deleteWebPushSubscriptionByUsernameQuery         = `DELETE FROM subscriptions WHERE username = ?`
	deleteWebPushSubscriptionByTopicAndEndpointQuery = `DELETE FROM subscriptions WHERE topic = ? AND endpoint = ?`

	selectWebPushSubscriptionsForTopicQuery = `SELECT endpoint, key_auth, key_p256dh, username FROM subscriptions WHERE topic = ?`

	selectWebPushSubscriptionsCountQuery = `SELECT COUNT(*) FROM subscriptions`
)

type webPushStore struct {
	db *sql.DB
}

func newWebPushStore(filename string) (*webPushStore, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSubscriptionsDB(db); err != nil {
		return nil, err
	}
	return &webPushStore{
		db: db,
	}, nil
}

func setupSubscriptionsDB(db *sql.DB) error {
	// If 'subscriptions' table does not exist, this must be a new database
	rowsMC, err := db.Query(selectWebPushSubscriptionsCountQuery)
	if err != nil {
		return setupNewSubscriptionsDB(db)
	}
	return rowsMC.Close()
}

func setupNewSubscriptionsDB(db *sql.DB) error {
	if _, err := db.Exec(createWebPushSubscriptionsTableQuery); err != nil {
		return err
	}
	return nil
}

func (c *webPushStore) AddSubscription(topic string, username string, subscription webPushSubscribePayload) error {
	_, err := c.db.Exec(
		insertWebPushSubscriptionQuery,
		topic,
		username,
		subscription.BrowserSubscription.Endpoint,
		subscription.BrowserSubscription.Keys.Auth,
		subscription.BrowserSubscription.Keys.P256dh,
	)
	return err
}

func (c *webPushStore) RemoveSubscription(topic string, endpoint string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByTopicAndEndpointQuery,
		topic,
		endpoint,
	)
	return err
}

func (c *webPushStore) GetSubscriptionsForTopic(topic string) (subscriptions []webPushSubscription, err error) {
	rows, err := c.db.Query(selectWebPushSubscriptionsForTopicQuery, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []webPushSubscription
	for rows.Next() {
		i := webPushSubscription{}
		err = rows.Scan(&i.BrowserSubscription.Endpoint, &i.BrowserSubscription.Keys.Auth, &i.BrowserSubscription.Keys.P256dh, &i.Username)
		if err != nil {
			return nil, err
		}
		data = append(data, i)
	}
	return data, nil
}

func (c *webPushStore) ExpireWebPushEndpoint(endpoint string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByEndpointQuery,
		endpoint,
	)
	return err
}

func (c *webPushStore) ExpireWebPushForUser(username string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByUsernameQuery,
		username,
	)
	return err
}
func (c *webPushStore) Close() error {
	return c.db.Close()
}
