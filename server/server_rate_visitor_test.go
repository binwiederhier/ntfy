package server

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	tp "heckel.io/ntfy/v2/topic"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestServer_UnifiedPush_RateVisitorOnOtherNode(t *testing.T) {
	// Two nodes on one database: a push on the other node than the subscriber's is billed to the
	// subscriber once the nodes have synced, not answered with a 507
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s1 := newTestServer(t, c)
		s2 := newTestServer(t, c)
		rr := request(t, s1, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return s2.topicService.State("upAAAAAAAAAAAA") != nil
		})
		response := request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, response.Code)
		require.Equal(t, int64(1), s2.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
		require.Equal(t, int64(0), s2.visitor(netip.MustParseAddr("9.9.9.9"), nil).Stats().Messages)
	})
}

func TestServer_UnifiedPush_RateVisitorOnOtherNodeUsesTier(t *testing.T) {
	// The other node knows the subscriber's user ID from the row, and looks up its tier on the
	// push, like an authenticated publish loads its user
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s1 := newTestServer(t, c)
		s2 := newTestServer(t, c)
		require.Nil(t, s1.userManager.AddTier(&user.Tier{
			Code:         "pro",
			MessageLimit: 10,
		}))
		require.Nil(t, s1.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s1.userManager.ChangeTier("phil", "pro"))
		phil, err := s1.userManager.User("phil")
		require.Nil(t, err)
		rr := request(t, s1, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		}, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "user:"+phil.ID, rateVisitorKey(s1, "upAAAAAAAAAAAA"))
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return s2.topicService.State("upAAAAAAAAAAAA") != nil
		})
		require.Equal(t, "user:"+phil.ID, rateVisitorKey(s2, "upAAAAAAAAAAAA"))
	})
}

func TestServer_UnifiedPush_StreamOnOtherNodeKeepsRateVisitor(t *testing.T) {
	// A stream open on one node keeps the topic's rate visitor live on the other nodes too: the
	// manager records it as subscriber activity, so it does not expire a day after the subscribe
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s1 := newTestServer(t, c)
		s2 := newTestServer(t, c)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			request(t, s1, "GET", "/upAAAAAAAAAAAA/json", "", nil, func(r *http.Request) {
				r.RemoteAddr = "1.2.3.4:1234"
				*r = *r.WithContext(ctx)
			})
		}()
		waitFor(t, func() bool {
			to := lockedTopic(s1, "upAAAAAAAAAAAA")
			return to != nil && to.SubscribersCount() == 1
		})
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return s2.topicService.State("upAAAAAAAAAAAA") != nil
		})

		// A day later, the stream is still open on s1
		ageTopic(t, "upAAAAAAAAAAAA", 25*time.Hour, s1, s2)
		require.Equal(t, 507, request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		s1.execManager()
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return time.Since(time.Unix(s2.topicService.State("upAAAAAAAAAAAA").SubscribedAt, 0)) < time.Hour
		})
		response := request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers410(t *testing.T) {
	// A UnifiedPush push to a topic without subscriber activity for unifiedPushAbandonedAfter is
	// told to go away (410); before that, it may be retried (507)
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		push := func() *errHTTP {
			response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
			if response.Code == 200 {
				return nil
			}
			return toHTTPError(t, response.Body.String())
		}

		// Never subscribed: 507 for 4 days from the first push, then 410
		require.Equal(t, 50701, push().Code)
		ageTopic(t, "upAAAAAAAAAAAA", 4*oneDay-time.Hour, s)
		require.Equal(t, 50701, push().Code)
		ageTopic(t, "upAAAAAAAAAAAA", 2*time.Hour, s)
		require.Equal(t, 41002, push().Code)
		require.Equal(t, 410, push().HTTPCode)

		// A subscriber comes (back): it pays again; 4 days after it is gone, 410
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		require.Nil(t, push())
		ageTopic(t, "upAAAAAAAAAAAA", 2*oneDay, s)
		require.Equal(t, 50701, push().Code)
		ageTopic(t, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter, s)
		require.Equal(t, 41002, push().Code)

		// An open stream keeps the topic, however long ago it was seen otherwise
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			request(t, s, "GET", "/upAAAAAAAAAAAA/json", "", nil, func(r *http.Request) {
				r.RemoteAddr = "1.2.3.4:1234"
				*r = *r.WithContext(ctx)
			})
		}()
		waitFor(t, func() bool {
			return lockedTopic(s, "upAAAAAAAAAAAA").SubscribersCount() == 1
		})
		ageTopic(t, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour, s)
		require.Nil(t, push())
	})
}

func TestServer_UnifiedPush_AbandonedTopicReadsRowBeforeAnswering410(t *testing.T) {
	// Before a 410, the node reads the topic's row: a subscriber may have come back on another
	// node, and the row may be fresher than what this node has read back so far
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		ageTopic(t, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour, s)
		subscribed := time.Now().Add(-2 * oneDay).Unix()
		execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET subscribed_at = %d, rate_visitor_ip = '1.2.3.4', updated_at = 1`, subscribed)) // Not read back: written "long ago"
		response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 507, response.Code)
		require.Equal(t, subscribed, s.topicService.State("upAAAAAAAAAAAA").SubscribedAt)

		// Known as abandoned, and so is the row: 410
		ageTopic(t, "upAAAAAAAAAAAA", 3*oneDay, s)
		response = request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 410, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers507IfRowReadFails(t *testing.T) {
	// If the row cannot be read, the push is not told to go away: 507, and the app server retries
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		ageTopic(t, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour, s)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil, func(r *http.Request) {
			*r = *r.WithContext(ctx)
		})
		require.Equal(t, 507, response.Code)
		response = request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 410, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicOnOtherNode(t *testing.T) {
	// The first refused push starts the countdown for every node: a node started 4 days later
	// answers 410
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s1 := newTestServer(t, c)
		response := request(t, s1, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 507, response.Code)
		s1.Stop() // Writes the topic
		execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET first_seen_at = first_seen_at - %d`, int64((unifiedPushAbandonedAfter+time.Hour)/time.Second)))
		s2 := newTestServer(t, c)
		response = request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 410, response.Code)
		require.Equal(t, 41002, toHTTPError(t, response.Body.String()).Code)
	})
}

func TestServer_UnifiedPush_RefusedPushDoesNotReviveRateVisitor(t *testing.T) {
	// A refused push to a topic whose rate visitor expired is not subscriber activity: the rate
	// visitor stays expired, here and in the row
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		ageTopic(t, "upAAAAAAAAAAAA", 25*time.Hour, s)
		subscribed := s.topicService.State("upAAAAAAAAAAAA").SubscribedAt
		for i := 0; i < 3; i++ {
			require.Equal(t, 507, request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		}
		require.Equal(t, subscribed, s.topicService.State("upAAAAAAAAAAAA").SubscribedAt)
		s.Stop() // Writes the topic
		var subscribedAt int64
		queryUserDB(t, c, `SELECT subscribed_at FROM topic WHERE topic = 'upAAAAAAAAAAAA'`, &subscribedAt)
		require.Equal(t, subscribed, subscribedAt)
	})
}

func TestServer_UnifiedPush_NonUnifiedPushTopicIsNotAbandoned(t *testing.T) {
	// A topic that cannot have a rate visitor (not an "up..." topic) keeps answering 507
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic?up=1", "some message", nil)
	require.Equal(t, 507, response.Code)
	require.Nil(t, s.topicService.State("mytopic"))
}

func TestServer_MatrixGateway_AbandonedTopicRejectsPushKey(t *testing.T) {
	// The Matrix gateway answers an abandoned topic with a rejected pushkey, not with the 410; a
	// topic without a subscriber for less than that is a 507, however long the node has known it
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	lockedTopic(s, "upAAAAAAAAAAAA").lastAccess = time.Now().Add(-13 * time.Hour)
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	ageTopic(t, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour, s)
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":["http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"]}`, strings.TrimSpace(response.Body.String()))
}

func TestServer_UnifiedPush_RateVisitorLivenessIsTheSubscribers(t *testing.T) {
	// A topic keeps its rate visitor for as long as the subscriber is around, not for as long as its
	// limiter key is in use: the limiter forgetting the key must not drop it, and publishers
	// charging the key must not keep it
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4:1234"
	})
	require.Equal(t, 200, rr.Code)
	s.limiter.Prune(0) // The key is gone, the subscriber is not
	rateVisitor := rateVisitorOf(t, s, "upAAAAAAAAAAAA")
	require.NotNil(t, rateVisitor)
	ageTopic(t, "upAAAAAAAAAAAA", 2*rateVisitorLiveFor, s) // The subscriber is gone
	require.True(t, rateVisitor.MessageAllowed().OK)       // A publisher charges the key
	require.Nil(t, rateVisitorOf(t, s, "upAAAAAAAAAAAA"))
}

func TestServer_UnifiedPush_TierChangeAppliesRightAway(t *testing.T) {
	// Pushes are billed with the subscriber's tier as of the push: the user is looked up on every
	// push, so a tier change applies to the next one
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "small",
			MessageLimit: 1,
		}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "large",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "small"))
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
		push := func() int {
			return request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code
		}
		require.Equal(t, 200, push())
		require.Equal(t, 429, push())

		// The new tier applies to the next push
		require.Nil(t, s.userManager.ChangeTier("phil", "large"))
		require.Equal(t, 200, push())
	})
}

func TestServer_UnifiedPush_TierDowngradeChargesIP(t *testing.T) {
	// A subscriber that loses its tier is charged on its IP like an anonymous one, with the
	// anonymous limits, from the next push; the user key stops being charged
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "pro",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
		phil, err := s.userManager.User("phil")
		require.Nil(t, err)
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		}, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		require.Equal(t, 200, request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("9.9.9.9"), phil).Stats().Messages)
		require.Equal(t, int64(0), s.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
		require.Nil(t, s.userManager.ResetTier("phil"))
		require.Equal(t, "ip:1.2.3.4", rateVisitorKey(s, "upAAAAAAAAAAAA"))
		require.Equal(t, 200, request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("9.9.9.9"), phil).Stats().Messages)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
	})
}

func TestServer_UnifiedPush_DeletedSubscriberChargesIP(t *testing.T) {
	// A deleted subscriber's account pays for nothing: its IP pays, like an anonymous subscriber's
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "pro",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("alice", "alice", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("bob", "bob", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("alice", "pro"))
		require.Nil(t, s.userManager.ChangeTier("bob", "pro"))
		alice, err := s.userManager.User("alice")
		require.Nil(t, err)
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("alice", "alice"),
		}, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "GET", "/upBBBBBBBBBBBB/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("bob", "bob"),
		}, func(r *http.Request) {
			r.RemoteAddr = "5.6.7.8:1234"
		})
		require.Equal(t, 200, rr.Code)

		// Alice deletes her account (soft delete), bob is removed by an admin (hard delete)
		require.Nil(t, s.userManager.MarkUserRemoved(alice))
		require.Nil(t, s.userManager.RemoveUser("bob"))
		require.Equal(t, "ip:1.2.3.4", rateVisitorKey(s, "upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", rateVisitorKey(s, "upBBBBBBBBBBBB"))
		for _, topic := range []string{"upAAAAAAAAAAAA", "upBBBBBBBBBBBB"} {
			require.Equal(t, 200, request(t, s, "PUT", "/"+topic+"?up=1", "some message", nil).Code, topic)
		}
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("5.6.7.8"), nil).Stats().Messages)
	})
}

// syncWait is how long a topic takes at most to reach another node in healthy operation: one
// sync round to be written, one to be read
const syncWait = 10 * time.Second

// rateVisitorKey returns the limiter key the topic's pushes are charged to, or "" if the topic
// has no live rate visitor
func rateVisitorKey(s *Server, topic string) string {
	to, err := s.topicFromID(nil, topic)
	if err != nil {
		return ""
	}
	if v, err := s.rateVisitor(to); err == nil && v != nil {
		return v.key
	}
	return ""
}

// ageTopic moves the topic's first observation and subscriber activity d back, as the servers see
// it after loading the aged row: it writes their topics, ages the row (an unknown topic becomes
// one first seen d ago), and restarts their topic services on it
func ageTopic(t *testing.T, topic string, d time.Duration, servers ...*Server) {
	t.Helper()
	for _, s := range servers {
		s.topicService.Close()
	}
	c := servers[0].config
	now, secs := time.Now().Unix(), int64(d/time.Second)
	execUserDB(t, c, fmt.Sprintf(`INSERT INTO topic (topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at) VALUES ('%s', %d, 0, '', '', %d) ON CONFLICT (topic) DO NOTHING`, topic, now, now))
	execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET first_seen_at = first_seen_at - %d, subscribed_at = CASE WHEN subscribed_at = 0 THEN 0 ELSE subscribed_at - %d END, updated_at = %d WHERE topic = '%s'`, secs, secs, now, topic))
	for _, s := range servers {
		userDB := openUserDB(t, c)
		store := tp.NewSQLiteStore(userDB)
		if c.DatabaseURL != "" {
			store = tp.NewPostgresStore(userDB)
		}
		topicService := tp.New(&tp.Config{Store: store})
		require.Nil(t, topicService.Start(context.Background()))
		s.topicService = topicService
		t.Cleanup(func() {
			topicService.Close() // Before the database closes; the server's Stop closes it again, a no-op
			userDB.Close()
		})
	}
}

// execUserDB runs the query on the config's user database: the auth-file, or the Postgres schema
func execUserDB(t *testing.T, c *Config, query string) {
	t.Helper()
	d := openUserDB(t, c)
	defer d.Close()
	_, err := d.Exec(query)
	require.Nil(t, err)
}

// queryUserDB runs the single-row query on the config's user database, scanning into dest
func queryUserDB(t *testing.T, c *Config, query string, dest ...any) {
	t.Helper()
	d := openUserDB(t, c)
	defer d.Close()
	require.Nil(t, d.QueryRow(query).Scan(dest...))
}

func openUserDB(t *testing.T, c *Config) *db.DB {
	t.Helper()
	if c.DatabaseURL != "" {
		host, err := pg.Open(c.DatabaseURL)
		require.Nil(t, err)
		return db.New(host, nil)
	}
	d, err := user.OpenSQLite(c.AuthFile)
	require.Nil(t, err)
	return d
}

// rateVisitorOf returns the visitor the topic's pushes are charged to, or nil if none
func rateVisitorOf(t *testing.T, s *Server, topic string) *visitor {
	v, err := s.rateVisitor(lockedTopic(s, topic))
	require.Nil(t, err)
	return v
}
