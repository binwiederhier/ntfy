package server

import (
	"context"
	"errors"
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

func TestServer_UnifiedPush_RateVisitorOnOtherNodeUsesTierAfterRefresh(t *testing.T) {
	// The other node knows the subscriber's user ID from the row, but its tier only once the
	// background refresh has looked it up: until then, the subscriber's IP is charged
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
		require.Equal(t, "user:"+phil.ID, rateVisitorKey(s1, "upAAAAAAAAAAAA")) // The subscribe primes the cache
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return s2.topicService.State("upAAAAAAAAAAAA") != nil
		})
		require.Equal(t, "ip:1.2.3.4", rateVisitorKey(s2, "upAAAAAAAAAAAA"))
		s2.execManager() // Refreshes the rate visitors' users
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
		ageTopic(s1, "upAAAAAAAAAAAA", 25*time.Hour)
		ageTopic(s2, "upAAAAAAAAAAAA", 25*time.Hour)
		require.Equal(t, 507, request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		s1.execManager()
		waitForWithMaxWait(t, 3*syncWait, func() bool {
			return time.Since(s2.topicService.State("upAAAAAAAAAAAA").SubscribedAt) < time.Hour
		})
		response := request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers404(t *testing.T) {
	// A UnifiedPush push to a topic without subscriber activity for unifiedPushAbandonedAfter is
	// told to go away (404, RFC 8030 section 7.3); before that, it may be retried (507)
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

		// Never subscribed: 507 for a week from the first push, then 404
		require.Equal(t, 50701, push().Code)
		ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter-time.Hour)
		require.Equal(t, 50701, push().Code)
		ageTopic(s, "upAAAAAAAAAAAA", 2*time.Hour)
		require.Equal(t, 40402, push().Code)
		require.Equal(t, 404, push().HTTPCode)

		// A subscriber comes (back): it pays again; a week after it is gone, 404
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		require.Nil(t, push())
		ageTopic(s, "upAAAAAAAAAAAA", 2*oneDay)
		require.Equal(t, 50701, push().Code)
		ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter)
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
		ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
		require.Nil(t, push())
	})
}

func TestServer_UnifiedPush_AbandonedTopicReadsRowBeforeAnswering404(t *testing.T) {
	// Before a 404, the node reads the topic's row: a subscriber may have come back on another
	// node, and the row may be fresher than what this node has read back so far
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
		now := time.Now().Unix()
		execUserDB(t, c, fmt.Sprintf(`INSERT INTO topic (topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at) VALUES ('upAAAAAAAAAAAA', %d, %d, '', '1.2.3.4', 1)`, now-2*int64(oneDay/time.Second), now-2*int64(oneDay/time.Second)))
		response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 507, response.Code)
		require.WithinDuration(t, time.Now().Add(-2*oneDay), s.topicService.State("upAAAAAAAAAAAA").SubscribedAt, time.Minute)

		// Known as abandoned, and so is the row: 404
		ageTopic(s, "upAAAAAAAAAAAA", 6*oneDay)
		execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET subscribed_at = %d, first_seen_at = %d`, now-8*int64(oneDay/time.Second), now-8*int64(oneDay/time.Second)))
		response = request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 404, response.Code)
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers507IfRowReadFails(t *testing.T) {
	// If the row cannot be read, the push is not told to go away: 507, and the app server retries
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil, func(r *http.Request) {
			*r = *r.WithContext(ctx)
		})
		require.Equal(t, 507, response.Code)
		response = request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 404, response.Code)
	})
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
		execUserDB(t, c, fmt.Sprintf(`UPDATE topic SET first_seen_at = first_seen_at - %d`, int64((unifiedPushAbandonedAfter+time.Hour)/time.Second)))
		s2 := newTestServer(t, c)
		response = request(t, s2, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 404, response.Code)
		require.Equal(t, 40402, toHTTPError(t, response.Body.String()).Code)
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
		ageTopic(s, "upAAAAAAAAAAAA", 25*time.Hour)
		subscribed := s.topicService.State("upAAAAAAAAAAAA").SubscribedAt
		for i := 0; i < 3; i++ {
			require.Equal(t, 507, request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code)
		}
		require.Equal(t, subscribed, s.topicService.State("upAAAAAAAAAAAA").SubscribedAt)
		s.Stop() // Writes the topic
		var subscribedAt int64
		queryUserDB(t, c, `SELECT subscribed_at FROM topic WHERE topic = 'upAAAAAAAAAAAA'`, &subscribedAt)
		require.Equal(t, subscribed.Unix(), subscribedAt)
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
	// The Matrix gateway answers an abandoned topic with a rejected pushkey, not with the 404; a
	// topic without a subscriber for less than that is a 507, however long the node has known it
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	lockedTopic(s, "upAAAAAAAAAAAA").lastAccess = time.Now().Add(-13 * time.Hour)
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	ageTopic(s, "upAAAAAAAAAAAA", unifiedPushAbandonedAfter+time.Hour)
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":["http://127.0.0.1:12345/upAAAAAAAAAAAA?up=1"]}`, strings.TrimSpace(response.Body.String()))
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
	rateVisitor := s.rateVisitor(lockedTopic(s, "upAAAAAAAAAAAA"))
	require.NotNil(t, rateVisitor)
	ageTopic(s, "upAAAAAAAAAAAA", 2*rateVisitorLiveFor) // The subscriber is gone
	require.True(t, rateVisitor.MessageAllowed().OK)    // A publisher charges the key
	require.Nil(t, s.rateVisitor(lockedTopic(s, "upAAAAAAAAAAAA")))
}

func TestServer_UnifiedPush_TierChangeAppliesAfterRefresh(t *testing.T) {
	// Pushes are billed with the subscriber's current tier, as of the last background refresh of
	// the rate visitors' users; a push never looks a user up
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
		lookups := 0
		s.rateVisitorUsers.lookup = func(ids []string) ([]*user.User, error) {
			lookups++
			return s.userManager.UsersByIDs(ids)
		}
		push := func() int {
			return request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil).Code
		}
		require.Equal(t, 200, push())
		require.Equal(t, 429, push())

		// The new tier applies once the users were refreshed
		require.Nil(t, s.userManager.ChangeTier("phil", "large"))
		require.Equal(t, 429, push())
		require.Equal(t, 0, lookups)
		s.execManager()
		require.Equal(t, 1, lookups)
		require.Equal(t, 200, push())
		require.Equal(t, 1, lookups)
	})
}

func TestServer_UnifiedPush_TierDowngradeChargesIP(t *testing.T) {
	// A subscriber that loses its tier is charged on its IP like an anonymous one, with the
	// anonymous limits, from the next refresh; the user key stops being charged
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
		s.execManager()
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
		s.execManager()
		require.Equal(t, "ip:1.2.3.4", rateVisitorKey(s, "upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", rateVisitorKey(s, "upBBBBBBBBBBBB"))
		for _, topic := range []string{"upAAAAAAAAAAAA", "upBBBBBBBBBBBB"} {
			require.Equal(t, 200, request(t, s, "PUT", "/"+topic+"?up=1", "some message", nil).Code, topic)
		}
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("1.2.3.4"), nil).Stats().Messages)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("5.6.7.8"), nil).Stats().Messages)
	})
}

func TestServer_UnifiedPush_RateVisitorUserRefreshFailureKeepsCachedUsers(t *testing.T) {
	// If the users cannot be refreshed, pushes keep using what was cached, rather than failing or
	// falling back to the IP
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
	}))
	require.Nil(t, s.userManager.AddUser("alice", "alice", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("alice", "pro"))
	alice, err := s.userManager.User("alice")
	require.Nil(t, err)
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("alice", "alice"),
	})
	require.Equal(t, 200, rr.Code)
	s.rateVisitorUsers.lookup = func(ids []string) ([]*user.User, error) {
		return nil, errors.New("database is down")
	}
	s.execManager()
	require.Equal(t, "user:"+alice.ID, rateVisitorKey(s, "upAAAAAAAAAAAA"))
	response := request(t, s, "PUT", "/upAAAAAAAAAAAA?up=1", "some message", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("9.9.9.9"), alice).Stats().Messages)
}

func TestServer_UnifiedPush_RateVisitorUsersAreOnlyTheLiveOnes(t *testing.T) {
	// The refresh looks up the users of the live rate visitors only, and drops the others
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("alice", "alice", user.RoleUser, false))
	require.Nil(t, s.userManager.AddUser("bob", "bob", user.RoleUser, false))
	alice, err := s.userManager.User("alice")
	require.Nil(t, err)
	bob, err := s.userManager.User("bob")
	require.Nil(t, err)
	for name, topic := range map[string]string{"alice": "upAAAAAAAAAAAA", "bob": "upBBBBBBBBBBBB"} {
		rr := request(t, s, "GET", "/"+topic+"/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth(name, name),
		})
		require.Equal(t, 200, rr.Code)
	}
	ageTopic(s, "upBBBBBBBBBBBB", 2*rateVisitorLiveFor)
	var looked []string
	s.rateVisitorUsers.lookup = func(ids []string) ([]*user.User, error) {
		looked = ids
		return s.userManager.UsersByIDs(ids)
	}
	s.execManager()
	require.Equal(t, []string{alice.ID}, looked)
	require.NotNil(t, s.rateVisitorUsers.User(alice.ID))
	require.Nil(t, s.rateVisitorUsers.User(bob.ID))
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
	if v := s.rateVisitor(to); v != nil {
		return v.key
	}
	return ""
}

// ageTopic makes the topic look as if its subscriber activity and first observation were d
// earlier than they are; an unknown topic becomes one first seen d ago
func ageTopic(s *Server, topic string, d time.Duration) {
	t := s.topicService.State(topic)
	if t == nil {
		t = &tp.State{FirstSeenAt: time.Now()}
	}
	t.FirstSeenAt = t.FirstSeenAt.Add(-d)
	if !t.SubscribedAt.IsZero() {
		t.SubscribedAt = t.SubscribedAt.Add(-d)
	}
	s.topicService.SetForTest(topic, t)
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
