package server

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Messages cache
const (
	createWebPushSubscriptionsTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS web_push_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			username TEXT,
			endpoint TEXT NOT NULL,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON web_push_subscriptions (topic);
		CREATE INDEX IF NOT EXISTS idx_endpoint ON web_push_subscriptions (endpoint);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_endpoint ON web_push_subscriptions (topic, endpoint);
		COMMIT;
	`
	insertWebPushSubscriptionQuery = `
		INSERT OR REPLACE INTO web_push_subscriptions (topic, username, endpoint, key_auth, key_p256dh)
		VALUES (?, ?, ?, ?, ?);
	`
	deleteWebPushSubscriptionByEndpointQuery         = `DELETE FROM web_push_subscriptions WHERE endpoint = ?`
	deleteWebPushSubscriptionByUsernameQuery         = `DELETE FROM web_push_subscriptions WHERE username = ?`
	deleteWebPushSubscriptionByTopicAndEndpointQuery = `DELETE FROM web_push_subscriptions WHERE topic = ? AND endpoint = ?`

	selectWebPushSubscriptionsForTopicQuery = `SELECT endpoint, key_auth, key_p256dh, username FROM web_push_subscriptions WHERE topic = ?`

	selectWebPushSubscriptionsCountQuery = `SELECT COUNT(*) FROM web_push_subscriptions`
)

type webPushSubscriptionStore struct {
	db *sql.DB
}

func newWebPushSubscriptionStore(filename string) (*webPushSubscriptionStore, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSubscriptionDb(db); err != nil {
		return nil, err
	}
	webPushSubscriptionStore := &webPushSubscriptionStore{
		db: db,
	}
	return webPushSubscriptionStore, nil
}

func setupSubscriptionDb(db *sql.DB) error {
	// If 'messages' table does not exist, this must be a new database
	rowsMC, err := db.Query(selectWebPushSubscriptionsCountQuery)
	if err != nil {
		return setupNewSubscriptionDb(db)
	}
	rowsMC.Close()
	return nil
}

func setupNewSubscriptionDb(db *sql.DB) error {
	if _, err := db.Exec(createWebPushSubscriptionsTableQuery); err != nil {
		return err
	}
	return nil
}

func (c *webPushSubscriptionStore) AddSubscription(topic string, username string, subscription webPushSubscribePayload) error {
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

func (c *webPushSubscriptionStore) RemoveSubscription(topic string, endpoint string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByTopicAndEndpointQuery,
		topic,
		endpoint,
	)
	return err
}

func (c *webPushSubscriptionStore) GetSubscriptionsForTopic(topic string) (subscriptions []webPushSubscription, err error) {
	rows, err := c.db.Query(selectWebPushSubscriptionsForTopicQuery, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := []webPushSubscription{}
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

func (c *webPushSubscriptionStore) ExpireWebPushEndpoint(endpoint string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByEndpointQuery,
		endpoint,
	)
	return err
}

func (c *webPushSubscriptionStore) ExpireWebPushForUser(username string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByUsernameQuery,
		username,
	)
	return err
}
func (c *webPushSubscriptionStore) Close() error {
	return c.db.Close()
}
