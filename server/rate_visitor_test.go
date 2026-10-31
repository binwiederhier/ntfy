package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/limit"
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
			return rateVisitorKey(s2, "upAAAAAAAAAAAA") == "ip:1.2.3.4"
		})
		response := request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, response.Code)
		require.Equal(t, int64(1), s2.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
		require.Equal(t, int64(0), s2.visitor(netip.MustParseAddr("9.9.9.9"), nil).Stats().Messages)
	})
}

func TestServer_UnifiedPush_StreamOnOtherNodeKeepsRateVisitor(t *testing.T) {
	// A stream open on one node keeps the topic's rate visitor live on the other nodes too: the manager
	// touches it, so it does not expire a day after the subscribe
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

		// A day later, the stream is still open on s1
		ageRateVisitor(s1, "upAAAAAAAAAAAA", 25*time.Hour)
		s1.execManager()
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			p := s2.limiter.RateVisitor("upAAAAAAAAAAAA")
			return p != nil && p.Key == "ip:1.2.3.4" && time.Since(p.SeenAt) < time.Hour
		})
		response := request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers404(t *testing.T) {
	// A UnifiedPush push to a topic without subscriber activity for unifiedPushAbandonedAfter is
	// told to go away (404, RFC 8030 section 7.3); before that, it may be retried (507)
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	push := func() *errHTTP {
		response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		if response.Code == 200 {
			return nil
		}
		return toHTTPError(t, response.Body.String())
	}

	// Never subscribed: 507 for a week from the first push, then 404
	require.Equal(t, 50701, push().Code)
	ageRateVisitor(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter-time.Hour)
	require.Equal(t, 50701, push().Code)
	ageRateVisitor(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
	require.Equal(t, 40402, push().Code)
	require.Equal(t, 404, push().HTTPCode)

	// A subscriber comes (back): it pays again; a week after it is gone, 404
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4:1234"
	})
	require.Equal(t, 200, rr.Code)
	require.Nil(t, push())
	ageRateVisitor(s, "upAAAAAAAAAAAA", 2*oneDay)
	require.Equal(t, 50701, push().Code)
	ageRateVisitor(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
	require.Equal(t, 40402, push().Code)

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
	ageRateVisitor(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
	require.Nil(t, push())
}

func TestServer_UnifiedPush_AbandonedTopicOnOtherNode(t *testing.T) {
	// The first refused push starts the countdown for every node: a node started a week later
	// answers 404
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s1 := newTestServer(t, c)
		response := request(t, s1, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 507, response.Code)
		s1.Stop() // Writes the topic
		execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET seen_at = seen_at - %d`, int64((unifiedPushAbandonedAfter+time.Hour)/time.Second)))
		s2 := newTestServer(t, c)
		response = request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 404, response.Code)
		require.Equal(t, 40402, toHTTPError(t, response.Body.String()).Code)
	})
}

func TestServer_UnifiedPush_NonUnifiedPushTopicIsNotAbandoned(t *testing.T) {
	// A topic that cannot have a rate visitor (not an "up..." topic) keeps answering 507
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic?up=1", "some message", nil)
	require.Equal(t, 507, response.Code)
	require.Nil(t, s.limiter.RateVisitor("mytopic"))
}

func TestServer_MatrixGateway_AbandonedTopicRejectsPushKey(t *testing.T) {
	// The Matrix gateway answers an abandoned topic with a rejected pushkey, not with the 404
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	ageRateVisitor(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":["http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"]}`+"\n", response.Body.String())
}

func TestServer_UnifiedPush_RateVisitorLivenessIsTheSubscribers(t *testing.T) {
	// A topic keeps its rate visitor for as long as the subscriber is around, not for as long as its
	// limiter key is in use: the limiter forgetting the key must not drop it, and publishers
	// charging the key must not keep it
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4:1234"
	})
	require.Equal(t, 200, rr.Code)
	s.limiter.Prune(0) // The key is gone, the subscriber is not
	rateVisitor, err := s.rateVisitor(lockedTopic(s, "upAAAAAAAAAAAA"))
	require.Nil(t, err)
	require.NotNil(t, rateVisitor)
	ageRateVisitor(s, "upAAAAAAAAAAAA", 2*visitorExpungeAfter) // The subscriber is gone
	require.True(t, rateVisitor.MessageAllowed())              // A publisher charges the key
	rateVisitor, err = s.rateVisitor(lockedTopic(s, "upAAAAAAAAAAAA"))
	require.Nil(t, err)
	require.Nil(t, rateVisitor)
}

func TestServer_UnifiedPush_TierChangeAppliesAfterUserCache(t *testing.T) {
	// Pushes are billed with the subscriber's current tier, as of the cached user lookup
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

		// The new tier applies once the cached user expires
		require.Nil(t, s.userManager.ChangeTier("phil", "large"))
		require.Equal(t, 429, push())
		expireRateVisitorUsers(s)
		require.Equal(t, 200, push())
	})
}

func TestServer_UnifiedPush_RateVisitorUserIsCached(t *testing.T) {
	// Pushes do not read the subscriber's user every time
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	lookups := 0
	s.rateVisitorUsers.lookup = func(id string) (*user.User, error) {
		lookups++
		return s.userManager.UserByID(id)
	}
	for i := 0; i < 3; i++ {
		require.Equal(t, 200, request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
	}
	require.Equal(t, 1, lookups)
}

func TestServer_UnifiedPush_DeletedSubscriberIsNoRateVisitor(t *testing.T) {
	// A deleted subscriber pays for nothing, and its IP does not pay in its place: the push gets
	// the 507 of a topic without subscriber
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddUser("alice", "alice", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("bob", "bob", user.RoleUser, false))
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("alice", "alice"),
		})
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "GET", "/upBBBBBBBBBBBB/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("bob", "bob"),
		})
		require.Equal(t, 200, rr.Code)

		// Alice deletes her account (soft delete), bob is removed by an admin (hard delete)
		alice, err := s.userManager.User("alice")
		require.Nil(t, err)
		require.Nil(t, s.userManager.MarkUserRemoved(alice))
		require.Nil(t, s.userManager.RemoveUser("bob"))
		for _, topic := range []string{"upAAAAAAAAAAAA", "upBBBBBBBBBBBB"} {
			response := request(t, s, "PUT", "/"+topic+"?up=1", "some message", nil)
			require.Equal(t, 507, response.Code, topic)
			require.Equal(t, 50701, toHTTPError(t, response.Body.String()).Code)
		}
	})
}

func TestServer_UnifiedPush_RateVisitorUserLookupErrorIsRetryable(t *testing.T) {
	// If the subscriber cannot be looked up, the push fails with a 5xx the publisher retries: it is
	// not a topic without subscriber, which would make the Matrix gateway reject the pushkey
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("alice", "alice", user.RoleUser, false))
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("alice", "alice"),
	})
	require.Equal(t, 200, rr.Code)
	s.rateVisitorUsers.lookup = func(id string) (*user.User, error) {
		return nil, errors.New("database is down")
	}
	response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
	require.Equal(t, 500, response.Code)
}

// syncWait is how long a rate visitor takes at most to reach another node in healthy operation: one
// sync round to be written, one to be read
const syncWait = 10 * time.Second

// rateVisitorKey returns the key of the topic's rate visitor, or "" if it has none
func rateVisitorKey(s *Server, topic string) string {
	if p := s.limiter.RateVisitor(topic); p != nil {
		return p.Key
	}
	return ""
}

// ageRateVisitor makes the topic's rate visitor (or the topic, if it has none) look as if it was last seen d ago
func ageRateVisitor(s *Server, topic string, d time.Duration) {
	p := s.limiter.RateVisitor(topic)
	if p == nil {
		p = &limit.RateVisitor{} // Unknown: make it known, so the caller's assertion fails, not the helper
	}
	p.SeenAt = time.Now().Add(-d)
	s.limiter.SetRateVisitor(topic, p)
}

// expireRateVisitorUsers makes the cached users of the rate visitors look older than rateVisitorUserCacheTTL
func expireRateVisitorUsers(s *Server) {
	s.rateVisitorUsers.mu.Lock()
	defer s.rateVisitorUsers.mu.Unlock()
	for _, e := range s.rateVisitorUsers.entries {
		e.fetched = e.fetched.Add(-2 * rateVisitorUserCacheTTL)
	}
}

// execUserDB runs the query on the config's user database: the auth-file, or the Postgres schema
func execUserDB(t *testing.T, c *Config, query string) {
	t.Helper()
	var d *db.DB
	if c.DatabaseURL != "" {
		host, err := pg.Open(c.DatabaseURL)
		require.Nil(t, err)
		d = db.New(host, nil)
	} else {
		var err error
		d, err = user.OpenSQLite(c.AuthFile)
		require.Nil(t, err)
	}
	defer d.Close()
	_, err := d.Exec(query)
	require.Nil(t, err)
}
