package server

import (
	"database/sql"
	"errors"
	"heckel.io/ntfy/util"
	"net/netip"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	subscriptionIDPrefix                     = "wps_"
	subscriptionIDLength                     = 10
	subscriptionEndpointLimitPerSubscriberIP = 10
)

var (
	errWebPushNoRows               = errors.New("no rows found")
	errWebPushTooManySubscriptions = errors.New("too many subscriptions")
	errWebPushUserIDCannotBeEmpty  = errors.New("user ID cannot be empty")
)

const (
	createWebPushSubscriptionsTableQuery = `
		BEGIN;
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
		COMMIT;
	`
	builtinStartupQueries = `
		PRAGMA foreign_keys = ON;
	`

	selectWebPushSubscriptionIDByEndpoint        = `SELECT id FROM subscription WHERE endpoint = ?`
	selectWebPushSubscriptionCountBySubscriberIP = `SELECT COUNT(*) FROM subscription WHERE subscriber_ip = ?`
	selectWebPushSubscriptionsForTopicQuery      = `
		SELECT id, endpoint, key_auth, key_p256dh, user_id
		FROM subscription_topic st
		JOIN subscription s ON s.id = st.subscription_id
		WHERE st.topic = ?
		ORDER BY endpoint
	`
	selectWebPushSubscriptionsExpiringSoonQuery = `
		SELECT id, endpoint, key_auth, key_p256dh, user_id 
		FROM subscription 
		WHERE warned_at = 0 AND updated_at <= ?
	`
	insertWebPushSubscriptionQuery = `
		INSERT INTO subscription (id, endpoint, key_auth, key_p256dh, user_id, subscriber_ip, updated_at, warned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) 
		DO UPDATE SET key_auth = excluded.key_auth, key_p256dh = excluded.key_p256dh, user_id = excluded.user_id, subscriber_ip = excluded.subscriber_ip, updated_at = excluded.updated_at, warned_at = excluded.warned_at
	`
	updateWebPushSubscriptionWarningSentQuery = `UPDATE subscription SET warned_at = ? WHERE id = ?`
	deleteWebPushSubscriptionByEndpointQuery  = `DELETE FROM subscription WHERE endpoint = ?`
	deleteWebPushSubscriptionByUserIDQuery    = `DELETE FROM subscription WHERE user_id = ?`
	deleteWebPushSubscriptionByAgeQuery       = `DELETE FROM subscription WHERE updated_at <= ?` // Full table scan!

	insertWebPushSubscriptionTopicQuery    = `INSERT INTO subscription_topic (subscription_id, topic) VALUES (?, ?)`
	deleteWebPushSubscriptionTopicAllQuery = `DELETE FROM subscription_topic WHERE subscription_id = ?`
)

// Schema management queries
const (
	currentWebPushSchemaVersion     = 1
	insertWebPushSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	selectWebPushSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
)

type webPushStore struct {
	db *sql.DB
}

func newWebPushStore(filename, startupQueries string) (*webPushStore, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupWebPushDB(db); err != nil {
		return nil, err
	}
	if err := runWebPushStartupQueries(db, startupQueries); err != nil {
		return nil, err
	}
	return &webPushStore{
		db: db,
	}, nil
}

func setupWebPushDB(db *sql.DB) error {
	// If 'schemaVersion' table does not exist, this must be a new database
	rows, err := db.Query(selectWebPushSchemaVersionQuery)
	if err != nil {
		return setupNewWebPushDB(db)
	}
	return rows.Close()
}

func setupNewWebPushDB(db *sql.DB) error {
	if _, err := db.Exec(createWebPushSubscriptionsTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(insertWebPushSchemaVersion, currentWebPushSchemaVersion); err != nil {
		return err
	}
	return nil
}

func runWebPushStartupQueries(db *sql.DB, startupQueries string) error {
	if _, err := db.Exec(startupQueries); err != nil {
		return err
	}
	if _, err := db.Exec(builtinStartupQueries); err != nil {
		return err
	}
	return nil
}

// UpsertSubscription adds or updates Web Push subscriptions for the given topics and user ID. It always first deletes all
// existing entries for a given endpoint.
func (c *webPushStore) UpsertSubscription(endpoint string, auth, p256dh, userID string, subscriberIP netip.Addr, topics []string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Read number of subscriptions for subscriber IP address
	rowsCount, err := tx.Query(selectWebPushSubscriptionCountBySubscriberIP, subscriberIP.String())
	if err != nil {
		return err
	}
	defer rowsCount.Close()
	var subscriptionCount int
	if !rowsCount.Next() {
		return errWebPushNoRows
	}
	if err := rowsCount.Scan(&subscriptionCount); err != nil {
		return err
	}
	if err := rowsCount.Close(); err != nil {
		return err
	}
	// Read existing subscription ID for endpoint (or create new ID)
	rows, err := tx.Query(selectWebPushSubscriptionIDByEndpoint, endpoint)
	if err != nil {
		return err
	}
	defer rows.Close()
	var subscriptionID string
	if rows.Next() {
		if err := rows.Scan(&subscriptionID); err != nil {
			return err
		}
	} else {
		if subscriptionCount >= subscriptionEndpointLimitPerSubscriberIP {
			return errWebPushTooManySubscriptions
		}
		subscriptionID = util.RandomStringPrefix(subscriptionIDPrefix, subscriptionIDLength)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	// Insert or update subscription
	updatedAt, warnedAt := time.Now().Unix(), 0
	if _, err = tx.Exec(insertWebPushSubscriptionQuery, subscriptionID, endpoint, auth, p256dh, userID, subscriberIP.String(), updatedAt, warnedAt); err != nil {
		return err
	}
	// Replace all subscription topics
	if _, err := tx.Exec(deleteWebPushSubscriptionTopicAllQuery, subscriptionID); err != nil {
		return err
	}
	for _, topic := range topics {
		if _, err = tx.Exec(insertWebPushSubscriptionTopicQuery, subscriptionID, topic); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SubscriptionsForTopic returns all subscriptions for the given topic
func (c *webPushStore) SubscriptionsForTopic(topic string) ([]*webPushSubscription, error) {
	rows, err := c.db.Query(selectWebPushSubscriptionsForTopicQuery, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return c.subscriptionsFromRows(rows)
}

// SubscriptionsExpiring returns all subscriptions that have not been updated for a given time period
func (c *webPushStore) SubscriptionsExpiring(warnAfter time.Duration) ([]*webPushSubscription, error) {
	rows, err := c.db.Query(selectWebPushSubscriptionsExpiringSoonQuery, time.Now().Add(-warnAfter).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return c.subscriptionsFromRows(rows)
}

// MarkExpiryWarningSent marks the given subscriptions as having received a warning about expiring soon
func (c *webPushStore) MarkExpiryWarningSent(subscriptions []*webPushSubscription) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, subscription := range subscriptions {
		if _, err := tx.Exec(updateWebPushSubscriptionWarningSentQuery, time.Now().Unix(), subscription.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *webPushStore) subscriptionsFromRows(rows *sql.Rows) ([]*webPushSubscription, error) {
	subscriptions := make([]*webPushSubscription, 0)
	for rows.Next() {
		var id, endpoint, auth, p256dh, userID string
		if err := rows.Scan(&id, &endpoint, &auth, &p256dh, &userID); err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, &webPushSubscription{
			ID:       id,
			Endpoint: endpoint,
			Auth:     auth,
			P256dh:   p256dh,
			UserID:   userID,
		})
	}
	return subscriptions, nil
}

// RemoveSubscriptionsByEndpoint removes the subscription for the given endpoint
func (c *webPushStore) RemoveSubscriptionsByEndpoint(endpoint string) error {
	_, err := c.db.Exec(deleteWebPushSubscriptionByEndpointQuery, endpoint)
	return err
}

// RemoveSubscriptionsByUserID removes all subscriptions for the given user ID
func (c *webPushStore) RemoveSubscriptionsByUserID(userID string) error {
	if userID == "" {
		return errWebPushUserIDCannotBeEmpty
	}
	_, err := c.db.Exec(deleteWebPushSubscriptionByUserIDQuery, userID)
	return err
}

// RemoveExpiredSubscriptions removes all subscriptions that have not been updated for a given time period
func (c *webPushStore) RemoveExpiredSubscriptions(expireAfter time.Duration) error {
	_, err := c.db.Exec(deleteWebPushSubscriptionByAgeQuery, time.Now().Add(-expireAfter).Unix())
	return err
}

// Close closes the underlying database connection
func (c *webPushStore) Close() error {
	return c.db.Close()
}
