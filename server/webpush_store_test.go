package server

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"net/netip"
	"path/filepath"
	"testing"
)

func newTestWebPushStore(t *testing.T, filename string) *webPushStore {
	webPush, err := newWebPushStore(filename)
	require.Nil(t, err)
	return webPush
}

func TestWebPushStore_UpsertSubscription_SubscriptionsForTopic(t *testing.T) {
	webPush := newTestWebPushStore(t, filepath.Join(t.TempDir(), "webpush.db"))
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
	webPush := newTestWebPushStore(t, filepath.Join(t.TempDir(), "webpush.db"))
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
