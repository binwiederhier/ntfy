package webpush_test

import (
	"database/sql"
	"fmt"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/webpush"
)

const testWebPushEndpoint = "https://updates.push.services.mozilla.com/wpush/v1/AAABBCCCDDEEEFFF"

// Schema layout as written by ntfy releases before the db/schema framework; used to verify
// that existing databases open cleanly without an adoption step
const (
	testPreFrameworkSQLiteSchema = `
		CREATE TABLE subscription (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at INT NOT NULL,
			warned_at INT NOT NULL DEFAULT 0
		);
		CREATE UNIQUE INDEX idx_endpoint ON subscription (endpoint);
		CREATE INDEX idx_subscriber_ip ON subscription (subscriber_ip);
		CREATE TABLE subscription_topic (
			subscription_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			PRIMARY KEY (subscription_id, topic),
			FOREIGN KEY (subscription_id) REFERENCES subscription (id) ON DELETE CASCADE
		);
		CREATE INDEX idx_topic ON subscription_topic (topic);
		CREATE TABLE schemaVersion (id INT PRIMARY KEY, version INT NOT NULL);
		INSERT INTO schemaVersion VALUES (1, 1);
	`
	testPreFrameworkPostgresSchema = `
		CREATE TABLE webpush_subscription (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL UNIQUE,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			warned_at BIGINT NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_webpush_subscriber_ip ON webpush_subscription (subscriber_ip);
		CREATE INDEX idx_webpush_updated_at ON webpush_subscription (updated_at);
		CREATE INDEX idx_webpush_user_id ON webpush_subscription (user_id);
		CREATE TABLE webpush_subscription_topic (
			subscription_id TEXT NOT NULL REFERENCES webpush_subscription (id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			PRIMARY KEY (subscription_id, topic)
		);
		CREATE INDEX idx_webpush_topic ON webpush_subscription_topic (topic);
		CREATE TABLE schema_version (store TEXT PRIMARY KEY, version INT NOT NULL);
		INSERT INTO schema_version (store, version) VALUES ('webpush', 1);
	`
)

// TestStoreSchemaEquivalence verifies that a database adopted from the pre-framework layout is
// structurally identical to a freshly created one: same tables, columns, indexes and keys.
func TestStoreSchemaEquivalence(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		freshFile := filepath.Join(t.TempDir(), "fresh.db")
		fresh, err := webpush.NewSQLiteStore(freshFile, "")
		require.Nil(t, err)
		defer fresh.Close()
		migratedFile := filepath.Join(t.TempDir(), "migrated.db")
		d, err := sql.Open("sqlite3", migratedFile)
		require.Nil(t, err)
		_, err = d.Exec(testPreFrameworkSQLiteSchema)
		require.Nil(t, err)
		require.Nil(t, d.Close())
		migrated, err := webpush.NewSQLiteStore(migratedFile, "")
		require.Nil(t, err)
		defer migrated.Close()
		freshDB, err := sql.Open("sqlite3", freshFile)
		require.Nil(t, err)
		defer freshDB.Close()
		migratedDB, err := sql.Open("sqlite3", migratedFile)
		require.Nil(t, err)
		defer migratedDB.Close()
		require.Equal(t, dbtest.SQLiteSchema(t, freshDB), dbtest.SQLiteSchema(t, migratedDB))
	})
	t.Run("postgres", func(t *testing.T) {
		freshDB := dbtest.CreateTestPostgres(t)
		_, err := webpush.NewPostgresStore(freshDB)
		require.Nil(t, err)
		migratedDB := dbtest.CreateTestPostgres(t)
		_, err = migratedDB.Exec(testPreFrameworkPostgresSchema)
		require.Nil(t, err)
		_, err = webpush.NewPostgresStore(migratedDB)
		require.Nil(t, err)
		require.Equal(t, dbtest.PostgresSchema(t, freshDB), dbtest.PostgresSchema(t, migratedDB))
	})
}

func TestStoreSQLiteOpensExistingDatabase(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "webpush.db")
	d, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)
	_, err = d.Exec(testPreFrameworkSQLiteSchema)
	require.Nil(t, err)
	require.Nil(t, d.Close())
	store, err := webpush.NewSQLiteStore(filename, "")
	require.Nil(t, err)
	defer store.Close()
	requireStoreUsable(t, store)
}

func TestStorePostgresOpensExistingDatabase(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t)
	_, err := testDB.Exec(testPreFrameworkPostgresSchema)
	require.Nil(t, err)
	store, err := webpush.NewPostgresStore(testDB)
	require.Nil(t, err)
	requireStoreUsable(t, store)
}

func requireStoreUsable(t *testing.T, store *webpush.Store) {
	t.Helper()
	err := store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"mytopic"})
	require.Nil(t, err)
	subs, err := store.SubscriptionsForTopic("mytopic")
	require.Nil(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, testWebPushEndpoint, subs[0].Endpoint)
}

func forEachBackend(t *testing.T, f func(t *testing.T, store *webpush.Store)) {
	t.Run("sqlite", func(t *testing.T) {
		store, err := webpush.NewSQLiteStore(filepath.Join(t.TempDir(), "webpush.db"), "")
		require.Nil(t, err)
		t.Cleanup(func() { store.Close() })
		f(t, store)
	})
	t.Run("postgres", func(t *testing.T) {
		testDB := dbtest.CreateTestPostgres(t)
		store, err := webpush.NewPostgresStore(testDB)
		require.Nil(t, err)
		f(t, store)
	})
}

func TestStoreUpsertSubscriptionSubscriptionsForTopic(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

		subs, err := store.SubscriptionsForTopic("test-topic")
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, subs[0].Endpoint, testWebPushEndpoint)
		require.Equal(t, subs[0].P256dh, "p256dh-key")
		require.Equal(t, subs[0].Auth, "auth-key")
		require.Equal(t, subs[0].UserID, "u_1234")

		subs2, err := store.SubscriptionsForTopic("mytopic")
		require.Nil(t, err)
		require.Len(t, subs2, 1)
		require.Equal(t, subs[0].Endpoint, subs2[0].Endpoint)
	})
}

func TestStoreUpsertSubscriptionSubscriberIPLimitReached(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert 10 subscriptions with the same IP address
		for i := 0; i < 10; i++ {
			endpoint := fmt.Sprintf(testWebPushEndpoint+"%d", i)
			require.Nil(t, store.UpsertSubscription(endpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))
		}

		// Another one for the same endpoint should be fine
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

		// But with a different endpoint it should fail
		require.Equal(t, webpush.ErrWebPushTooManySubscriptions, store.UpsertSubscription(testWebPushEndpoint+"11", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

		// But with a different IP address it should be fine again
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"99", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("9.9.9.9"), []string{"test-topic", "mytopic"}))
	})
}

func TestStoreUpsertSubscriptionUpdateTopics(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics, and another with one topic
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"1", "auth-key", "p256dh-key", "", netip.MustParseAddr("9.9.9.9"), []string{"topic1"}))

		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 2)
		require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)
		require.Equal(t, testWebPushEndpoint+"1", subs[1].Endpoint)

		subs, err = store.SubscriptionsForTopic("topic2")
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)

		// Update the first subscription to have only one topic
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))

		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 2)
		require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)

		subs, err = store.SubscriptionsForTopic("topic2")
		require.Nil(t, err)
		require.Len(t, subs, 0)
	})
}

func TestStoreUpsertSubscriptionUpdateFields(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert a subscription
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))

		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, "auth-key", subs[0].Auth)
		require.Equal(t, "p256dh-key", subs[0].P256dh)
		require.Equal(t, "u_1234", subs[0].UserID)

		// Re-upsert the same endpoint with different auth, p256dh, and userID
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "new-auth", "new-p256dh", "u_5678", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))

		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, testWebPushEndpoint, subs[0].Endpoint)
		require.Equal(t, "new-auth", subs[0].Auth)
		require.Equal(t, "new-p256dh", subs[0].P256dh)
		require.Equal(t, "u_5678", subs[0].UserID)
	})
}

func TestStoreRemoveByUserIDMultiple(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert two subscriptions for u_1234 and one for u_5678
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"1", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint+"2", "auth-key", "p256dh-key", "u_5678", netip.MustParseAddr("9.9.9.9"), []string{"topic1"}))

		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 3)

		// Remove all subscriptions for u_1234
		require.Nil(t, store.RemoveSubscriptionsByUserID("u_1234"))

		// Only u_5678's subscription should remain
		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, testWebPushEndpoint+"2", subs[0].Endpoint)
		require.Equal(t, "u_5678", subs[0].UserID)
	})
}

func TestStoreRemoveByEndpoint(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)

		// And remove it again
		require.Nil(t, store.RemoveSubscriptionsByEndpoint(testWebPushEndpoint))
		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 0)
	})
}

func TestStoreRemoveByUserID(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)

		// And remove it again
		require.Nil(t, store.RemoveSubscriptionsByUserID("u_1234"))
		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 0)
	})
}

func TestStoreRemoveByUserIDEmpty(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		require.Equal(t, webpush.ErrWebPushUserIDCannotBeEmpty, store.RemoveSubscriptionsByUserID(""))
	})
}

func TestStoreExpiryWarningSent(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))

		// Set updated_at to the past so it shows up as expiring
		require.Nil(t, store.SetSubscriptionUpdatedAt(testWebPushEndpoint, time.Now().Add(-8*24*time.Hour).Unix()))

		// Verify subscription appears in expiring list (warned_at == 0)
		subs, err := store.SubscriptionsExpiring(7 * 24 * time.Hour)
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, testWebPushEndpoint, subs[0].Endpoint)

		// Mark them as warning sent
		require.Nil(t, store.MarkExpiryWarningSent(subs))

		// Verify subscription no longer appears in expiring list (warned_at > 0)
		subs, err = store.SubscriptionsExpiring(7 * 24 * time.Hour)
		require.Nil(t, err)
		require.Len(t, subs, 0)
	})
}

func TestStoreExpiring(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)

		// Fake-mark them as soon-to-expire
		require.Nil(t, store.SetSubscriptionUpdatedAt(testWebPushEndpoint, time.Now().Add(-8*24*time.Hour).Unix()))

		// Should not be cleaned up yet
		require.Nil(t, store.RemoveExpiredSubscriptions(9*24*time.Hour))

		// Run expiration
		subs, err = store.SubscriptionsExpiring(7 * 24 * time.Hour)
		require.Nil(t, err)
		require.Len(t, subs, 1)
		require.Equal(t, testWebPushEndpoint, subs[0].Endpoint)
	})
}

func TestStoreRemoveExpired(t *testing.T) {
	forEachBackend(t, func(t *testing.T, store *webpush.Store) {
		// Insert subscription with two topics
		require.Nil(t, store.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
		subs, err := store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 1)

		// Fake-mark them as expired
		require.Nil(t, store.SetSubscriptionUpdatedAt(testWebPushEndpoint, time.Now().Add(-10*24*time.Hour).Unix()))

		// Run expiration
		require.Nil(t, store.RemoveExpiredSubscriptions(9*24*time.Hour))

		// List again, should be 0
		subs, err = store.SubscriptionsForTopic("topic1")
		require.Nil(t, err)
		require.Len(t, subs, 0)
	})
}
