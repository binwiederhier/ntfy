package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
)

var (
	webPushSubscribePayloadExample = `{
		"browser_subscription":{
			"endpoint": "https://example.com/webpush",
			"keys": {
				"p256dh": "p256dh-key",
				"auth": "auth-key"
			}
		}
	}`
)

func TestServer_WebPush_GetConfig(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	response := request(t, s, "GET", "/v1/web-push-config", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, fmt.Sprintf(`{"public_key":"%s"}`, s.config.WebPushPublicKey)+"\n", response.Body.String())
}

func TestServer_WebPush_TopicSubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	response := request(t, s, "POST", "/test-topic/web-push/subscribe", webPushSubscribePayloadExample, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	subs, err := s.webPush.GetSubscriptionsForTopic("test-topic")
	if err != nil {
		t.Fatal(err)
	}

	require.Len(t, subs, 1)
	require.Equal(t, subs[0].BrowserSubscription.Endpoint, "https://example.com/webpush")
	require.Equal(t, subs[0].BrowserSubscription.Keys.P256dh, "p256dh-key")
	require.Equal(t, subs[0].BrowserSubscription.Keys.Auth, "auth-key")
	require.Equal(t, subs[0].Username, "")
}

func TestServer_WebPush_TopicSubscribeProtected_Allowed(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	config.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, config)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "test-topic", user.PermissionReadWrite))

	response := request(t, s, "POST", "/test-topic/web-push/subscribe", webPushSubscribePayloadExample, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})

	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	subs, err := s.webPush.GetSubscriptionsForTopic("test-topic")
	if err != nil {
		t.Fatal(err)
	}

	require.Len(t, subs, 1)
	require.Equal(t, subs[0].Username, "ben")
}

func TestServer_WebPush_TopicSubscribeProtected_Denied(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	config.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, config)

	response := request(t, s, "POST", "/test-topic/web-push/subscribe", webPushSubscribePayloadExample, nil)
	require.Equal(t, 403, response.Code)

	requireSubscriptionCount(t, s, "test-topic", 0)
}

func TestServer_WebPush_TopicUnsubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	response := request(t, s, "POST", "/test-topic/web-push/subscribe", webPushSubscribePayloadExample, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	requireSubscriptionCount(t, s, "test-topic", 1)

	unsubscribe := `{"endpoint":"https://example.com/webpush"}`
	response = request(t, s, "POST", "/test-topic/web-push/unsubscribe", unsubscribe, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	requireSubscriptionCount(t, s, "test-topic", 0)
}

func TestServer_WebPush_DeleteAccountUnsubscribe(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	config.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, config)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "test-topic", user.PermissionReadWrite))

	response := request(t, s, "POST", "/test-topic/web-push/subscribe", webPushSubscribePayloadExample, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})

	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	requireSubscriptionCount(t, s, "test-topic", 1)

	request(t, s, "DELETE", "/v1/account", `{"password":"ben"}`, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	// should've been deleted with the account
	requireSubscriptionCount(t, s, "test-topic", 0)
}

func TestServer_WebPush_Publish(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	var received atomic.Bool

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/push-receive", r.URL.Path)
		require.Equal(t, "high", r.Header.Get("Urgency"))
		require.Equal(t, "", r.Header.Get("Topic"))
		received.Store(true)
	}))
	defer upstreamServer.Close()

	addSubscription(t, s, "test-topic", upstreamServer.URL+"/push-receive")

	request(t, s, "PUT", "/test-topic", "web push test", nil)

	waitFor(t, func() bool {
		return received.Load()
	})
}

func TestServer_WebPush_PublishExpire(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	var received atomic.Bool

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		// Gone
		w.WriteHeader(410)
		w.Write([]byte(``))
		received.Store(true)
	}))
	defer upstreamServer.Close()

	addSubscription(t, s, "test-topic", upstreamServer.URL+"/push-receive")
	addSubscription(t, s, "test-topic-abc", upstreamServer.URL+"/push-receive")

	requireSubscriptionCount(t, s, "test-topic", 1)
	requireSubscriptionCount(t, s, "test-topic-abc", 1)

	request(t, s, "PUT", "/test-topic", "web push test", nil)

	waitFor(t, func() bool {
		return received.Load()
	})

	// Receiving the 410 should've caused the publisher to expire all subscriptions on the endpoint

	requireSubscriptionCount(t, s, "test-topic", 0)
	requireSubscriptionCount(t, s, "test-topic-abc", 0)
}

func addSubscription(t *testing.T, s *Server, topic string, url string) {
	err := s.webPush.AddSubscription("test-topic", "", webPushSubscribePayload{
		BrowserSubscription: webpush.Subscription{
			Endpoint: url,
			Keys: webpush.Keys{
				// connected to a local test VAPID key, not a leak!
				Auth:   "kSC3T8aN1JCQxxPdrFLrZg",
				P256dh: "BMKKbxdUU_xLS7G1Wh5AN8PvWOjCzkCuKZYb8apcqYrDxjOF_2piggBnoJLQYx9IeSD70fNuwawI3e9Y8m3S3PE",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requireSubscriptionCount(t *testing.T, s *Server, topic string, expectedLength int) {
	subs, err := s.webPush.GetSubscriptionsForTopic("test-topic")
	if err != nil {
		t.Fatal(err)
	}

	require.Len(t, subs, expectedLength)
}
