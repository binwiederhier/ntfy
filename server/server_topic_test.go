package server

import (
	"database/sql"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/limit"
)

// The step 6 tests: the topic table answers what the in-memory topic used to (who a UnifiedPush
// topic bills to, when it was last subscribed to), so topics can be evicted after minutes. See
// plans/261025-shared-rate-limits.md, "UnifiedPush and topic eviction".

func TestServer_TopicEvictedAfterMinutesWhenPersisted(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfig(t, databaseURL))
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "hi", nil).Code)
		s.mu.Lock()
		s.topics["mytopic"].lastAccess = time.Now().Add(-topicExpungeAfterPersisted - time.Minute)
		s.mu.Unlock()
		s.execManager()
		s.mu.RLock()
		_, ok := s.topics["mytopic"]
		s.mu.RUnlock()
		require.False(t, ok) // Not 16 hours: the table remembers what the topic would have
	})
}

func TestServer_UnifiedPush_RateVisitorSurvivesTopicEviction(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
			r.RemoteAddr = "1.2.3.4:1234"
		})
		require.Equal(t, 200, rr.Code)
		waitFor(t, func() bool {
			key, _, err := s.limiter.TopicRateVisitor("upAAAAAAAAAAAA")
			return err == nil && key == "ip:1.2.3.4"
		})

		// The topic goes from memory (or another node takes the publish): the table still says
		// who it bills to, so the publish is accepted and billed to the subscriber
		evictTopic(s, "upAAAAAAAAAAAA")
		rr = request(t, s, "POST", "/upAAAAAAAAAAAA?up=1", "some message", nil)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, int64(1), s.visitor(netip.MustParseAddr("1.2.3.4"), nil).Usage().Count(limit.KindMessages))
		require.Equal(t, int64(0), s.visitor(netip.MustParseAddr("9.9.9.9"), nil).Usage().Count(limit.KindMessages))
	})
}

func TestServer_UnifiedPush_AbandonedTopicAnswers404(t *testing.T) {
	// A UnifiedPush topic nobody subscribed to for upTopicAbandonedAfter tells the publisher to
	// stop: 404, as RFC 8030 prescribes for an expired subscription; before that, 507 (transient)
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		rr := request(t, s, "POST", "/upBBBBBBBBBBBB?up=1", "some message", nil)
		require.Equal(t, 507, rr.Code)
		require.Equal(t, 50701, toHTTPError(t, rr.Body.String()).Code)
		backdateTopic(t, s, "upBBBBBBBBBBBB", upTopicAbandonedAfter-time.Hour)
		evictTopic(s, "upBBBBBBBBBBBB") // Drops the topic's cached read of its row
		rr = request(t, s, "POST", "/upBBBBBBBBBBBB?up=1", "some message", nil)
		require.Equal(t, 507, rr.Code) // A phone offline for days is not gone
		backdateTopic(t, s, "upBBBBBBBBBBBB", upTopicAbandonedAfter+time.Hour)
		evictTopic(s, "upBBBBBBBBBBBB")
		rr = request(t, s, "POST", "/upBBBBBBBBBBBB?up=1", "some message", nil)
		require.Equal(t, 404, rr.Code)
		require.Equal(t, 40402, toHTTPError(t, rr.Body.String()).Code)

		// A plain publish (no up=1) to the same topic is not a UnifiedPush push: unaffected
		require.Equal(t, 200, request(t, s, "POST", "/upBBBBBBBBBBBB", "some message", nil).Code)
	})
}

func TestServer_MatrixGateway_RejectsFromTopicTable(t *testing.T) {
	// The 12 h Matrix rule is answered from the table, so a node that never saw the topic (or
	// evicted it) rejects the pushkey as well
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/upCCCCCCCCCCCC?up=1"}]}}`
		response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
		require.Equal(t, 507, response.Code)
		backdateTopic(t, s, "upCCCCCCCCCCCC", 13*time.Hour)
		evictTopic(s, "upCCCCCCCCCCCC")
		response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
		require.Equal(t, 200, response.Code)
		require.Equal(t, `{"rejected":["http://127.0.0.1:12345/upCCCCCCCCCCCC?up=1"]}`, strings.TrimSpace(response.Body.String()))
	})
}

// evictTopic removes the topic from memory, as the manager does with an idle one
func evictTopic(s *Server, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.topics, id)
}

// backdateTopic moves every recorded time of the topic's row back by age, as if it had been
// created and last subscribed to that long ago
func backdateTopic(t *testing.T, s *Server, id string, age time.Duration) {
	t.Helper()
	waitFor(t, func() bool {
		_, err := s.limiter.Topic(id)
		return err == nil
	})
	then := time.Now().Add(-age).Unix()
	var err error
	if s.db != nil {
		_, err = s.db.Exec(`UPDATE topic SET created_at = $1, last_subscribed_at = LEAST(last_subscribed_at, $1), rate_visitor_seen_at = LEAST(rate_visitor_seen_at, $1) WHERE id = $2`, then, id)
	} else {
		d, openErr := sql.Open("sqlite3", s.config.UsageFile)
		require.Nil(t, openErr)
		defer d.Close()
		_, err = d.Exec(`UPDATE topic SET created_at = ?, last_subscribed_at = MIN(last_subscribed_at, ?), rate_visitor_seen_at = MIN(rate_visitor_seen_at, ?) WHERE id = ?`, then, then, then, id)
	}
	require.Nil(t, err)
}

func TestServer_UnifiedPush_SubscriberAnnouncedToPeers(t *testing.T) {
	// A UnifiedPush topic gaining a rate visitor is announced, so peers drop their cached miss
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	b := &fakeCluster{}
	s.cluster = b
	rr := request(t, s, "GET", "/upDDDDDDDDDDDD/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4:1234"
	})
	require.Equal(t, 200, rr.Code)
	require.Equal(t, []string{"upDDDDDDDDDDDD"}, b.Announced())
}

func TestServer_UnifiedPush_PeerAnnouncementClearsCachedMiss(t *testing.T) {
	// Lab finding (case "up", 2026-10-25): a push before the subscription cached "no row" on this
	// node for 30 s, so a subscription on another node went unseen and pushes got 507
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorSubscriberRateLimiting = true
		s := newTestServer(t, c)
		rr := request(t, s, "POST", "/upEEEEEEEEEEEE?up=1", "m", nil)
		require.Equal(t, 507, rr.Code) // Caches the miss
		other := newOtherNodeLimiter(t, s)
		require.Nil(t, other.SetTopicRateVisitor("upEEEEEEEEEEEE", "ip:1.2.3.4", ""))
		require.Nil(t, other.Close())
		rr = request(t, s, "POST", "/upEEEEEEEEEEEE?up=1", "m", nil)
		require.Equal(t, 507, rr.Code) // Still cached
		s.handlePeerTopicsAdded([]string{"upEEEEEEEEEEEE"})
		rr = request(t, s, "POST", "/upEEEEEEEEEEEE?up=1", "m", nil)
		require.Equal(t, 200, rr.Code)
	})
}
