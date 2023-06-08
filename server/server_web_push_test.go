package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
)

const (
	defaultEndpoint = "https://updates.push.services.mozilla.com/wpush/v1/AAABBCCCDDEEEFFF"
)

func TestServer_WebPush_TopicAdd(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{"test-topic"}, defaultEndpoint), nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	subs, err := s.webPush.SubscriptionsForTopic("test-topic")
	require.Nil(t, err)

	require.Len(t, subs, 1)
	require.Equal(t, subs[0].BrowserSubscription.Endpoint, defaultEndpoint)
	require.Equal(t, subs[0].BrowserSubscription.Keys.P256dh, "p256dh-key")
	require.Equal(t, subs[0].BrowserSubscription.Keys.Auth, "auth-key")
	require.Equal(t, subs[0].UserID, "")
}

func TestServer_WebPush_TopicAdd_InvalidEndpoint(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{"test-topic"}, "https://ddos-target.example.com/webpush"), nil)
	require.Equal(t, 400, response.Code)
	require.Equal(t, `{"code":40039,"http":400,"error":"invalid request: web push endpoint unknown"}`+"\n", response.Body.String())
}

func TestServer_WebPush_TopicAdd_TooManyTopics(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	topicList := make([]string, 51)
	for i := range topicList {
		topicList[i] = util.RandomString(5)
	}

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, topicList, defaultEndpoint), nil)
	require.Equal(t, 400, response.Code)
	require.Equal(t, `{"code":40040,"http":400,"error":"invalid request: too many web push topic subscriptions"}`+"\n", response.Body.String())
}

func TestServer_WebPush_TopicUnsubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	addSubscription(t, s, "test-topic", defaultEndpoint)
	requireSubscriptionCount(t, s, "test-topic", 1)

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{}, defaultEndpoint), nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	requireSubscriptionCount(t, s, "test-topic", 0)
}

func TestServer_WebPush_TopicSubscribeProtected_Allowed(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	config.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, config)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "test-topic", user.PermissionReadWrite))

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{"test-topic"}, defaultEndpoint), map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())

	subs, err := s.webPush.SubscriptionsForTopic("test-topic")
	require.Nil(t, err)
	require.Len(t, subs, 1)
	require.True(t, strings.HasPrefix(subs[0].UserID, "u_"))
}

func TestServer_WebPush_TopicSubscribeProtected_Denied(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	config.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, config)

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{"test-topic"}, defaultEndpoint), nil)
	require.Equal(t, 403, response.Code)

	requireSubscriptionCount(t, s, "test-topic", 0)
}

func TestServer_WebPush_DeleteAccountUnsubscribe(t *testing.T) {
	config := configureAuth(t, newTestConfigWithWebPush(t))
	s := newTestServer(t, config)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "test-topic", user.PermissionReadWrite))

	response := request(t, s, "PUT", "/v1/account/webpush", payloadForTopics(t, []string{"test-topic"}, defaultEndpoint), map[string]string{
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

	pushService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/push-receive", r.URL.Path)
		require.Equal(t, "high", r.Header.Get("Urgency"))
		require.Equal(t, "", r.Header.Get("Topic"))
		received.Store(true)
	}))
	defer pushService.Close()

	addSubscription(t, s, "test-topic", pushService.URL+"/push-receive")

	request(t, s, "PUT", "/test-topic", "web push test", nil)

	waitFor(t, func() bool {
		return received.Load()
	})
}

func TestServer_WebPush_PublishExpire(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	var received atomic.Bool

	pushService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		// Gone
		w.WriteHeader(410)
		received.Store(true)
	}))
	defer pushService.Close()

	addSubscription(t, s, "test-topic", pushService.URL+"/push-receive")
	addSubscription(t, s, "test-topic-abc", pushService.URL+"/push-receive")

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

func TestServer_WebPush_Expiry(t *testing.T) {
	s := newTestServer(t, newTestConfigWithWebPush(t))

	var received atomic.Bool

	pushService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		w.WriteHeader(200)
		w.Write([]byte(``))
		received.Store(true)
	}))
	defer pushService.Close()

	addSubscription(t, s, "test-topic", pushService.URL+"/push-receive")
	requireSubscriptionCount(t, s, "test-topic", 1)

	_, err := s.webPush.db.Exec("UPDATE subscriptions SET updated_at = datetime('now', '-7 days')")
	require.Nil(t, err)

	s.expireOrNotifyOldSubscriptions()
	requireSubscriptionCount(t, s, "test-topic", 1)

	waitFor(t, func() bool {
		return received.Load()
	})

	_, err = s.webPush.db.Exec("UPDATE subscriptions SET updated_at = datetime('now', '-8 days')")
	require.Nil(t, err)

	s.expireOrNotifyOldSubscriptions()
	requireSubscriptionCount(t, s, "test-topic", 0)
}

func payloadForTopics(t *testing.T, topics []string, endpoint string) string {
	topicsJSON, err := json.Marshal(topics)
	require.Nil(t, err)

	return fmt.Sprintf(`{
		"topics": %s,
		"browser_subscription":{
			"endpoint": "%s",
			"keys": {
				"p256dh": "p256dh-key",
				"auth": "auth-key"
			}
		}
	}`, topicsJSON, endpoint)
}

func addSubscription(t *testing.T, s *Server, topic string, url string) {
	err := s.webPush.AddSubscription(topic, "", webpush.Subscription{
		Endpoint: url,
		Keys: webpush.Keys{
			// connected to a local test VAPID key, not a leak!
			Auth:   "kSC3T8aN1JCQxxPdrFLrZg",
			P256dh: "BMKKbxdUU_xLS7G1Wh5AN8PvWOjCzkCuKZYb8apcqYrDxjOF_2piggBnoJLQYx9IeSD70fNuwawI3e9Y8m3S3PE",
		},
	})
	require.Nil(t, err)
}

func requireSubscriptionCount(t *testing.T, s *Server, topic string, expectedLength int) {
	subs, err := s.webPush.SubscriptionsForTopic("test-topic")
	require.Nil(t, err)

	require.Len(t, subs, expectedLength)
}
