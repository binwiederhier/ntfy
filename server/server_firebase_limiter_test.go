//go:build !nofirebase

package server

import (
	"fmt"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	vis "heckel.io/ntfy/v2/visitor"
)

func TestFirebaseLimiter_VisitorBurstThenOnePerMinute(t *testing.T) {
	l := newFirebaseLimiter(DefaultFirebaseLimits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)

	// The first few go out, on any topic, then the visitor is out of tokens
	for i := 0; i < DefaultFirebaseLimits.VisitorBurst; i++ {
		require.True(t, l.allowKeys("1.2.3.4", fmt.Sprintf("topic%d", i), start))
	}
	require.False(t, l.allowKeys("1.2.3.4", "another", start))
	require.False(t, l.allowKeys("1.2.3.4", "another", start.Add(30*time.Second)))

	// One more every minute
	require.True(t, l.allowKeys("1.2.3.4", "another", start.Add(time.Minute)))
	require.False(t, l.allowKeys("1.2.3.4", "another", start.Add(time.Minute)))

	// Other visitors are independent
	require.True(t, l.allowKeys("5.6.7.8", "another", start.Add(time.Minute)))
}

func TestFirebaseLimiter_TopicBurstThenTenPerHour(t *testing.T) {
	l := newFirebaseLimiter(DefaultFirebaseLimits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)

	// The first few go out, from any visitor, then the topic is out of tokens
	for i := 0; i < DefaultFirebaseLimits.TopicBurst; i++ {
		require.True(t, l.allowKeys(fmt.Sprintf("10.0.0.%d", i), "mytopic", start))
	}
	require.False(t, l.allowKeys("10.0.1.1", "mytopic", start))
	require.False(t, l.allowKeys("10.0.1.2", "mytopic", start.Add(5*time.Minute)))

	// One more every six minutes
	require.True(t, l.allowKeys("10.0.1.3", "mytopic", start.Add(6*time.Minute)))
	require.False(t, l.allowKeys("10.0.1.4", "mytopic", start.Add(6*time.Minute)))

	// Other topics are independent
	require.True(t, l.allowKeys("10.0.1.5", "othertopic", start.Add(6*time.Minute)))
}

func TestFirebaseLimiter_DroppedMessagesUseNoTokens(t *testing.T) {
	l := newFirebaseLimiter(DefaultFirebaseLimits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)

	// Drain the topic, then a visitor keeps publishing to it: its own tokens are not used up
	for i := 0; i < DefaultFirebaseLimits.TopicBurst; i++ {
		require.True(t, l.allowKeys(fmt.Sprintf("10.0.0.%d", i), "busy", start))
	}
	for i := 0; i < 20; i++ {
		require.False(t, l.allowKeys("1.2.3.4", "busy", start))
	}
	require.True(t, l.allowKeys("1.2.3.4", "quiet", start))
}

func TestFirebaseLimiter_VisitorAcrossTopics(t *testing.T) {
	l := newFirebaseLimiter(DefaultFirebaseLimits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)
	for i := 0; i < DefaultFirebaseLimits.VisitorBurst; i++ {
		require.True(t, l.allowKeys("1.2.3.4", fmt.Sprintf("topic%d", i), start)) // As a peer would
	}
	require.False(t, l.allowKeys("1.2.3.4", "another", start))
}

func TestFirebaseLimiter_ForgetsFullBuckets(t *testing.T) {
	l := newFirebaseLimiter(DefaultFirebaseLimits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)
	require.True(t, l.allowKeys("1.2.3.4", "mytopic", start))
	require.Len(t, l.visitors, 1)
	require.Len(t, l.topics, 1)

	// An hour later, both buckets are full again and are forgotten
	require.True(t, l.allowKeys("5.6.7.8", "othertopic", start.Add(time.Hour)))
	require.Len(t, l.visitors, 1)
	require.Len(t, l.topics, 1)
}

func TestFirebaseLimiter_ConfiguredLimits(t *testing.T) {
	limits := &FirebaseLimits{
		VisitorBurst:  2,
		VisitorRefill: 30 * time.Second,
		TopicBurst:    100,
		TopicRefill:   time.Second,
	}
	l := newFirebaseLimiter(limits, newTestFirebaseLimiterConfig())
	start := time.Date(2026, 11, 25, 20, 30, 0, 0, time.UTC)
	require.True(t, l.allowKeys("1.2.3.4", "mytopic", start))
	require.True(t, l.allowKeys("1.2.3.4", "mytopic", start))
	require.False(t, l.allowKeys("1.2.3.4", "mytopic", start))
	require.True(t, l.allowKeys("1.2.3.4", "mytopic", start.Add(30*time.Second)))
}

func TestParseFirebaseLimits(t *testing.T) {
	// Empty disables the limit
	limits, err := ParseFirebaseLimits("")
	require.Nil(t, err)
	require.Nil(t, limits)

	// All values
	limits, err = ParseFirebaseLimits("visitor-burst=3, visitor-refill=2m, topic-burst=20, topic-refill=3m")
	require.Nil(t, err)
	require.Equal(t, &FirebaseLimits{
		VisitorBurst:  3,
		VisitorRefill: 2 * time.Minute,
		TopicBurst:    20,
		TopicRefill:   3 * time.Minute,
	}, limits)

	// Missing values are the defaults
	limits, err = ParseFirebaseLimits("topic-burst=20")
	require.Nil(t, err)
	require.Equal(t, 20, limits.TopicBurst)
	require.Equal(t, DefaultFirebaseLimits.VisitorBurst, limits.VisitorBurst)
	require.Equal(t, DefaultFirebaseLimits.VisitorRefill, limits.VisitorRefill)
	require.Equal(t, DefaultFirebaseLimits.TopicRefill, limits.TopicRefill)
	limits, err = ParseFirebaseLimits("defaults")
	require.Nil(t, err)
	require.Equal(t, DefaultFirebaseLimits, limits)

	// Invalid
	for _, s := range []string{"visitor-burst", "visitor-burst=0", "visitor-burst=x", "topic-refill=0", "topic-refill=soon", "unknown=1"} {
		_, err := ParseFirebaseLimits(s)
		require.Error(t, err, s)
	}
}

func TestServer_FirebaseLimit_Visitor(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		sender := newTestFirebaseSender(100)
		conf := newTestConfig(t, databaseURL)
		conf.FirebaseLimit = DefaultFirebaseLimits
		s := newTestServer(t, conf)
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true}, newFirebaseLimiterFromConfig(conf))

		// The visitor's first few messages go to Firebase, the rest are not sent
		for i := 0; i < DefaultFirebaseLimits.VisitorBurst+2; i++ {
			response := request(t, s, "PUT", fmt.Sprintf("/topic%d", i), "hi", nil)
			require.Equal(t, 200, response.Code)
		}

		// Another visitor is not affected
		response := request(t, s, "PUT", "/othertopic", "hi", nil, func(r *http.Request) {
			r.RemoteAddr = "5.6.7.8:1234"
		})
		require.Equal(t, 200, response.Code)
		requireFirebaseMessages(t, sender, DefaultFirebaseLimits.VisitorBurst+1)
	})
}

func TestServer_FirebaseLimit_PayingUserIsNotLimited(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		sender := newTestFirebaseSender(100)
		conf := newTestConfigWithAuthFile(t, databaseURL)
		conf.FirebaseLimit = DefaultFirebaseLimits
		s := newTestServer(t, conf)
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true}, newFirebaseLimiterFromConfig(conf))
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "pro",
			MessageLimit: 1000,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

		// More than both bursts, all on one topic
		count := DefaultFirebaseLimits.TopicBurst + DefaultFirebaseLimits.VisitorBurst
		for i := 0; i < count; i++ {
			response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, response.Code)
		}
		requireFirebaseMessages(t, sender, count)
	})
}

func TestServer_FirebaseLimit_CountsPeerMessages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		sender := newTestFirebaseSender(100)
		conf := newTestConfig(t, databaseURL)
		conf.FirebaseLimit = DefaultFirebaseLimits
		s := newTestServer(t, conf)
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true}, newFirebaseLimiterFromConfig(conf))

		// Messages published on a peer use up the topic's tokens, even with no local subscriber
		for i := 0; i < DefaultFirebaseLimits.TopicBurst; i++ {
			m := model.NewDefaultMessage("mytopic", fmt.Sprintf("peer %d", i))
			m.Sender = netip.MustParseAddr(fmt.Sprintf("10.0.0.%d", i))
			s.deliverFromBus(m)
		}
		response := request(t, s, "PUT", "/mytopic", "local", nil)
		require.Equal(t, 200, response.Code)
		response = request(t, s, "PUT", "/othertopic", "local", nil)
		require.Equal(t, 200, response.Code)
		requireFirebaseMessages(t, sender, 1)
		require.Equal(t, "othertopic", sender.Messages()[0].Data["topic"])
	})
}

func TestServer_FirebaseLimit_DisabledByDefault(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		sender := newTestFirebaseSender(100)
		conf := newTestConfig(t, databaseURL)
		s := newTestServer(t, conf)
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true}, newFirebaseLimiterFromConfig(conf))

		// More than both bursts, all on one topic
		count := DefaultFirebaseLimits.TopicBurst + DefaultFirebaseLimits.VisitorBurst
		for i := 0; i < count; i++ {
			response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
			require.Equal(t, 200, response.Code)
		}
		requireFirebaseMessages(t, sender, count)
	})
}

func newTestFirebaseLimiterConfig() *vis.Config {
	return &vis.Config{
		PrefixBitsIPv4: 32,
		PrefixBitsIPv6: 64,
	}
}

func requireFirebaseMessages(t *testing.T, sender *testFirebaseSender, count int) {
	require.Eventually(t, func() bool {
		return len(sender.Messages()) == count
	}, 2*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // Nothing else arrives
	require.Len(t, sender.Messages(), count)
}
