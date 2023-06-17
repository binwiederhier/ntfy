package server

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestWebPushStore_UpsertSubscription_SubscriptionsForTopic(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

	subs, err := webPush.SubscriptionsForTopic("test-topic")
	require.Nil(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, subs[0].Endpoint, testWebPushEndpoint)
	require.Equal(t, subs[0].P256dh, "p256dh-key")
	require.Equal(t, subs[0].Auth, "auth-key")
	require.Equal(t, subs[0].UserID, "u_1234")

	subs2, err := webPush.SubscriptionsForTopic("mytopic")
	require.Nil(t, err)
	require.Len(t, subs2, 1)
	require.Equal(t, subs[0].Endpoint, subs2[0].Endpoint)
}

func TestWebPushStore_UpsertSubscription_SubscriberIPLimitReached(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert 10 subscriptions with the same IP address
	for i := 0; i < 10; i++ {
		endpoint := fmt.Sprintf(testWebPushEndpoint+"%d", i)
		require.Nil(t, webPush.UpsertSubscription(endpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))
	}

	// Another one for the same endpoint should be fine
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

	// But with a different endpoint it should fail
	require.Equal(t, errWebPushTooManySubscriptions, webPush.UpsertSubscription(testWebPushEndpoint+"11", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"test-topic", "mytopic"}))

	// But with a different IP address it should be fine again
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint+"99", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("9.9.9.9"), []string{"test-topic", "mytopic"}))
}

func TestWebPushStore_UpsertSubscription_UpdateTopics(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics, and another with one topic
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint+"1", "auth-key", "p256dh-key", "", netip.MustParseAddr("9.9.9.9"), []string{"topic1"}))

	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 2)
	require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)
	require.Equal(t, testWebPushEndpoint+"1", subs[1].Endpoint)

	subs, err = webPush.SubscriptionsForTopic("topic2")
	require.Nil(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)

	// Update the first subscription to have only one topic
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint+"0", "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1"}))

	subs, err = webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 2)
	require.Equal(t, testWebPushEndpoint+"0", subs[0].Endpoint)

	subs, err = webPush.SubscriptionsForTopic("topic2")
	require.Nil(t, err)
	require.Len(t, subs, 0)
}

func TestWebPushStore_RemoveSubscriptionsByEndpoint(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 1)

	// And remove it again
	require.Nil(t, webPush.RemoveSubscriptionsByEndpoint(testWebPushEndpoint))
	subs, err = webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 0)
}

func TestWebPushStore_RemoveSubscriptionsByUserID(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 1)

	// And remove it again
	require.Nil(t, webPush.RemoveSubscriptionsByUserID("u_1234"))
	subs, err = webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 0)
}

func TestWebPushStore_RemoveSubscriptionsByUserID_Empty(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()
	require.Equal(t, errWebPushUserIDCannotBeEmpty, webPush.RemoveSubscriptionsByUserID(""))
}

func TestWebPushStore_MarkExpiryWarningSent(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 1)

	// Mark them as warning sent
	require.Nil(t, webPush.MarkExpiryWarningSent(subs))

	rows, err := webPush.db.Query("SELECT endpoint FROM subscription WHERE warned_at > 0")
	require.Nil(t, err)
	defer rows.Close()
	var endpoint string
	require.True(t, rows.Next())
	require.Nil(t, rows.Scan(&endpoint))
	require.Nil(t, err)
	require.Equal(t, testWebPushEndpoint, endpoint)
	require.False(t, rows.Next())
}

func TestWebPushStore_SubscriptionsExpiring(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 1)

	// Fake-mark them as soon-to-expire
	_, err = webPush.db.Exec("UPDATE subscription SET updated_at = ? WHERE endpoint = ?", time.Now().Add(-8*24*time.Hour).Unix(), testWebPushEndpoint)
	require.Nil(t, err)

	// Should not be cleaned up yet
	require.Nil(t, webPush.RemoveExpiredSubscriptions(9*24*time.Hour))

	// Run expiration
	subs, err = webPush.SubscriptionsExpiring(7 * 24 * time.Hour)
	require.Nil(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, testWebPushEndpoint, subs[0].Endpoint)
}

func TestWebPushStore_RemoveExpiredSubscriptions(t *testing.T) {
	webPush := newTestWebPushStore(t)
	defer webPush.Close()

	// Insert subscription with two topics
	require.Nil(t, webPush.UpsertSubscription(testWebPushEndpoint, "auth-key", "p256dh-key", "u_1234", netip.MustParseAddr("1.2.3.4"), []string{"topic1", "topic2"}))
	subs, err := webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 1)

	// Fake-mark them as expired
	_, err = webPush.db.Exec("UPDATE subscription SET updated_at = ? WHERE endpoint = ?", time.Now().Add(-10*24*time.Hour).Unix(), testWebPushEndpoint)
	require.Nil(t, err)

	// Run expiration
	require.Nil(t, webPush.RemoveExpiredSubscriptions(9*24*time.Hour))

	// List again, should be 0
	subs, err = webPush.SubscriptionsForTopic("topic1")
	require.Nil(t, err)
	require.Len(t, subs, 0)
}

func newTestWebPushStore(t *testing.T) *webPushStore {
	webPush, err := newWebPushStore(filepath.Join(t.TempDir(), "webpush.db"))
	require.Nil(t, err)
	return webPush
}
