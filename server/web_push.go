package server

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	createWebPushSubscriptionsTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			user_id TEXT,
			endpoint TEXT NOT NULL,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			warning_sent BOOLEAN DEFAULT FALSE
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON subscriptions (topic);
		CREATE INDEX IF NOT EXISTS idx_endpoint ON subscriptions (endpoint);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_topic_endpoint ON subscriptions (topic, endpoint);
		COMMIT;
	`
	insertWebPushSubscriptionQuery = `
		INSERT OR REPLACE INTO subscriptions (topic, user_id, endpoint, key_auth, key_p256dh)
		VALUES (?, ?, ?, ?, ?)
	`
	deleteWebPushSubscriptionByEndpointQuery = `DELETE FROM subscriptions WHERE endpoint = ?`
	deleteWebPushSubscriptionByUserIDQuery   = `DELETE FROM subscriptions WHERE user_id = ?`
	deleteWebPushSubscriptionsByAgeQuery     = `DELETE FROM subscriptions WHERE warning_sent = 1 AND updated_at <= datetime('now', ?)`

	selectWebPushSubscriptionsForTopicQuery     = `SELECT endpoint, key_auth, key_p256dh, user_id FROM subscriptions WHERE topic = ?`
	selectWebPushSubscriptionsExpiringSoonQuery = `SELECT DISTINCT endpoint, key_auth, key_p256dh FROM subscriptions WHERE warning_sent = 0 AND updated_at <= datetime('now', ?)`

	updateWarningSentQuery = `UPDATE subscriptions SET warning_sent = true WHERE warning_sent = 0 AND updated_at <= datetime('now', ?)`

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
	rows, err := db.Query(selectWebPushSubscriptionsCountQuery)
	if err != nil {
		return setupNewSubscriptionsDB(db)
	}
	return rows.Close()
}

func setupNewSubscriptionsDB(db *sql.DB) error {
	if _, err := db.Exec(createWebPushSubscriptionsTableQuery); err != nil {
		return err
	}
	return nil
}

func (c *webPushStore) UpdateSubscriptions(topics []string, userID string, subscription webpush.Subscription) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = c.RemoveByEndpoint(subscription.Endpoint); err != nil {
		return err
	}
	for _, topic := range topics {
		if err := c.AddSubscription(topic, userID, subscription); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *webPushStore) AddSubscription(topic string, userID string, subscription webpush.Subscription) error {
	_, err := c.db.Exec(
		insertWebPushSubscriptionQuery,
		topic,
		userID,
		subscription.Endpoint,
		subscription.Keys.Auth,
		subscription.Keys.P256dh,
	)
	return err
}

func (c *webPushStore) SubscriptionsForTopic(topic string) (subscriptions []*webPushSubscription, err error) {
	rows, err := c.db.Query(selectWebPushSubscriptionsForTopicQuery, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []*webPushSubscription
	for rows.Next() {
		var userID, endpoint, auth, p256dh string
		if err = rows.Scan(&endpoint, &auth, &p256dh, &userID); err != nil {
			return nil, err
		}
		data = append(data, &webPushSubscription{
			UserID: userID,
			BrowserSubscription: webpush.Subscription{
				Endpoint: endpoint,
				Keys: webpush.Keys{
					Auth:   auth,
					P256dh: p256dh,
				},
			},
		})
	}
	return data, nil
}

func (c *webPushStore) ExpireAndGetExpiringSubscriptions(warningDuration time.Duration, expiryDuration time.Duration) (subscriptions []webPushSubscription, err error) {
	// TODO this should be two functions
	tx, err := c.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(deleteWebPushSubscriptionsByAgeQuery, fmt.Sprintf("-%.2f seconds", expiryDuration.Seconds()))
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(selectWebPushSubscriptionsExpiringSoonQuery, fmt.Sprintf("-%.2f seconds", warningDuration.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []webPushSubscription
	for rows.Next() {
		i := webPushSubscription{}
		err = rows.Scan(&i.BrowserSubscription.Endpoint, &i.BrowserSubscription.Keys.Auth, &i.BrowserSubscription.Keys.P256dh)
		fmt.Printf("%v+", i)
		if err != nil {
			return nil, err
		}
		data = append(data, i)
	}

	// also set warning as sent
	_, err = tx.Exec(updateWarningSentQuery, fmt.Sprintf("-%.2f seconds", warningDuration.Seconds()))
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return data, nil
}

func (c *webPushStore) RemoveByEndpoint(endpoint string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByEndpointQuery,
		endpoint,
	)
	return err
}

func (c *webPushStore) RemoveByUserID(userID string) error {
	_, err := c.db.Exec(
		deleteWebPushSubscriptionByUserIDQuery,
		userID,
	)
	return err
}

func (c *webPushStore) Close() error {
	return c.db.Close()
}
