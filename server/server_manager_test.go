package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
	"testing"
	"time"
)

func TestServer_Manager_Prune_Messages_Without_Attachments_DoesNotPanic(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// Tests that the manager runs without attachment-cache-dir set, see #617
		c := newTestConfig(t, databaseURL)
		c.AttachmentCacheDir = ""
		s := newTestServer(t, c)

		// Publish a message
		rr := request(t, s, "POST", "/mytopic", "hi", nil)
		require.Equal(t, 200, rr.Code)
		m := toMessage(t, rr.Body.String())

		// Expire message
		require.Nil(t, s.messageCache.ExpireMessages("mytopic"))

		// Does not panic
		s.pruneMessages()

		// Actually deleted
		_, err := s.messageCache.Message(m.ID)
		require.Equal(t, model.ErrMessageNotFound, err)
	})
}

func TestServer_Manager_ExpiresDeliveries(t *testing.T) {
	s := newTestServer(t, newTestConfig(t, ""))
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	topics[0].Subscribe(func(_ *visitor, _ *model.Message) error { return nil }, nil, func() {}) // Not stale
	topics[0].mu.Lock()
	topics[0].delivered["old"] = time.Now().Add(-topicDeliveredRetain - time.Second)
	topics[0].mu.Unlock()
	s.execManager()
	_, ok := topics[0].DeliveredAt("old")
	require.False(t, ok)
}

func TestServer_Manager_SharedJobsRunOnLeaderOnly(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// A passive (non-leader) node must not prune the shared database. It does add its own
		// publishes to the stored message total, as a delta, so another node's count survives
		s := newTestServer(t, newTestConfig(t, databaseURL))
		b := &fakeCluster{}
		b.setLeader(false)
		s.cluster = b

		// Publish a message and expire it, and store a total as another node would have
		rr := request(t, s, "POST", "/mytopic", "hi", nil)
		require.Equal(t, 200, rr.Code)
		m := toMessage(t, rr.Body.String())
		require.Nil(t, s.messageCache.ExpireMessages("mytopic"))
		require.Nil(t, s.messageCache.AddStats(500))

		// Not the leader: nothing shared is touched
		s.execManager()
		_, err := s.messageCache.Message(m.ID)
		require.Nil(t, err)
		messages, err := s.messageCache.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(501), messages)

		// Leader: expired messages are pruned, and the total is not counted twice
		b.setLeader(true)
		s.execManager()
		_, err = s.messageCache.Message(m.ID)
		require.Equal(t, model.ErrMessageNotFound, err)
		messages, err = s.messageCache.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(501), messages)
		require.Equal(t, int64(501), s.messages)
	})
}

func TestServer_Manager_MessagesTotalSumsAcrossNodes(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// Two nodes on one database: each adds only its own publishes to the stored total, and
		// a node that loaded the total at startup does not add it again on its first flush
		c := newTestConfig(t, databaseURL)
		s1 := newTestServer(t, c)
		require.Equal(t, 200, request(t, s1, "POST", "/mytopic", "one", nil).Code)
		require.Equal(t, 200, request(t, s1, "POST", "/mytopic", "two", nil).Code)
		s1.execManager()

		s2 := newTestServer(t, c)
		require.Equal(t, int64(2), s2.messages)
		require.Equal(t, 200, request(t, s2, "POST", "/mytopic", "three", nil).Code)
		s2.execManager()
		s1.execManager()

		messages, err := s1.messageCache.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(3), messages)
		require.Equal(t, int64(3), s1.messages)
		require.Equal(t, int64(3), s2.messages)
	})
}
