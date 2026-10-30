package server

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTopicPayers_KeptForExemptSubscriber(t *testing.T) {
	// A subscriber from a request-limit-exempt prefix spends nothing before it becomes a topic's
	// payer; the topic must still keep it. Fails if a new payer counts as stale.
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	c.VisitorRequestExemptPrefixes = []netip.Prefix{netip.MustParsePrefix("9.9.9.0/24")}
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	topicIDs, err := s.payers.Subscribe(v, []string{"upAAAAAAAAAAAA"})
	require.Nil(t, err)
	require.Equal(t, []string{"upAAAAAAAAAAAA"}, topicIDs)
	require.True(t, s.payers.Live("upAAAAAAAAAAAA"))
}

func TestTopicPayers_LivenessIsTheSubscribers(t *testing.T) {
	// A topic keeps its payer for as long as the subscriber is around (its keepalive), not for as
	// long as its limiter key is in use: the limiter forgetting the key must not drop it, and
	// publishers charging the key must not keep it
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	_, err := s.payers.Subscribe(v, []string{"upAAAAAAAAAAAA"})
	require.Nil(t, err)
	s.limiter.Prune(0) // The key is gone, the subscriber is not
	payer, err := s.payers.Resolve("upAAAAAAAAAAAA")
	require.Nil(t, err)
	require.NotNil(t, payer)
	agePayer(s, "upAAAAAAAAAAAA", 2*visitorExpungeAfter)           // The subscriber is gone
	require.True(t, s.limiter.Allow(v.key, v.messageRule(), 1).OK) // A publisher charges the key
	payer, err = s.payers.Resolve("upAAAAAAAAAAAA")
	require.Nil(t, err)
	require.Nil(t, payer)
}

func TestTopicPayers_OnlyUnifiedPushTopics(t *testing.T) {
	// Only topics shaped like UnifiedPush topics get a payer, and only if the feature is on
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	topicIDs, err := s.payers.Subscribe(v, []string{"mytopic", "upAAAAAAAAAAAA", "upAAAAAAAAAAAAA"})
	require.Nil(t, err)
	require.Equal(t, []string{"upAAAAAAAAAAAA"}, topicIDs)
	c.VisitorSubscriberRateLimiting = false
	topicIDs, err = s.payers.Subscribe(v, []string{"upBBBBBBBBBBBB"})
	require.Nil(t, err)
	require.Empty(t, topicIDs)
	require.False(t, s.payers.Live("upBBBBBBBBBBBB"))
}

func TestTopicPayers_PruneForgetsStalePayers(t *testing.T) {
	// The manager prunes payers whose subscriber is gone, and keeps the others
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	_, err := s.payers.Subscribe(v, []string{"upAAAAAAAAAAAA", "upBBBBBBBBBBBB"})
	require.Nil(t, err)
	agePayer(s, "upAAAAAAAAAAAA", 2*visitorExpungeAfter)
	require.Equal(t, 1, s.payers.Prune())
	require.Equal(t, "", payerIP(s, "upAAAAAAAAAAAA"))
	require.Equal(t, "9.9.9.9", payerIP(s, "upBBBBBBBBBBBB"))
}

func TestTopicPayers_ManagerKeepsTopicWithPayer(t *testing.T) {
	// A topic whose payer is alive stays in memory, so the Matrix gateway can tell how long it had no access
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	_, err := s.topicsFromIDs(v, "upAAAAAAAAAAAA", "upBBBBBBBBBBBB")
	require.Nil(t, err)
	_, err = s.payers.Subscribe(v, []string{"upAAAAAAAAAAAA"})
	require.Nil(t, err)
	for _, topicID := range []string{"upAAAAAAAAAAAA", "upBBBBBBBBBBBB"} {
		to := lockedTopic(s, topicID)
		to.mu.Lock()
		to.lastAccess = time.Now().Add(-2 * topicExpungeAfter)
		to.mu.Unlock()
	}
	s.execManager()
	require.NotNil(t, lockedTopic(s, "upAAAAAAAAAAAA"))
	require.Nil(t, lockedTopic(s, "upBBBBBBBBBBBB"))
}

// agePayer makes the topic's payer look as if it was last seen d ago
func agePayer(s *Server, topicID string, d time.Duration) {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	py := s.payers.payers[topicID]
	py.seen = time.Now().Add(-d)
	s.payers.payers[topicID] = py
}

// payerSeen returns when the topic's payer was last seen
func payerSeen(s *Server, topicID string) time.Time {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	return s.payers.payers[topicID].seen
}

// payerIP returns the IP address of the topic's payer, or "" if it has none
func payerIP(s *Server, topicID string) string {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	py, ok := s.payers.payers[topicID]
	if !ok {
		return ""
	}
	return py.ip.String()
}
