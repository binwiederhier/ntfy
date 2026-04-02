package webpush_test

import (
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
