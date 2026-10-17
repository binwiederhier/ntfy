package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"sync/atomic"
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
	topics[0].Subscribe(func(_ *visitor, _ *model.Message) error { return nil }, "", func() {}) // Not stale
	topics[0].mu.Lock()
	topics[0].delivered["old"] = time.Now().Add(-topicDeliveredRetain - time.Second)
	topics[0].mu.Unlock()
	s.execManager()
	_, ok := topics[0].DeliveredAt("old")
	require.False(t, ok)
}

func TestServer_Manager_SharedJobsRunOnLeaderOnly(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// A passive (non-leader) node must not prune the shared database, nor overwrite the
		// stored message total with its own count, which only covers its own publishes
		s := newTestServer(t, newTestConfig(t, databaseURL))
		b := &fakeCluster{}
		b.setLeader(false)
		s.cluster = b

		// Publish a message and expire it, and store a total as another node would have
		rr := request(t, s, "POST", "/mytopic", "hi", nil)
		require.Equal(t, 200, rr.Code)
		m := toMessage(t, rr.Body.String())
		require.Nil(t, s.messageCache.ExpireMessages("mytopic"))
		require.Nil(t, s.messageCache.UpdateStats(500))

		// Not the leader: nothing shared is touched
		s.execManager()
		_, err := s.messageCache.Message(m.ID)
		require.Nil(t, err)
		messages, err := s.messageCache.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(500), messages)

		// Leader: expired messages are pruned and the total is written
		b.setLeader(true)
		s.execManager()
		_, err = s.messageCache.Message(m.ID)
		require.Equal(t, model.ErrMessageNotFound, err)
		messages, err = s.messageCache.Stats()
		require.Nil(t, err)
		require.Equal(t, s.messages, messages)
	})
}

func TestServer_Manager_ReauthorizesSubscribers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// A relayed revocation can be lost, and an ACL change made elsewhere (another node, the
		// `ntfy access` CLI) never cancels anything here. A live stream must not keep receiving
		// forever, so the manager re-checks that live subscribers may still read their topics.
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddUser("alice", "password", user.RoleUser, false))
		require.Nil(t, s.userManager.AllowAccess("alice", "private", user.PermissionRead))
		alice, err := s.userManager.User("alice")
		require.Nil(t, err)
		topics, err := s.topicsFromIDs(nil, "private")
		require.Nil(t, err)
		canceled := atomic.Bool{}
		topics[0].Subscribe(func(*visitor, *model.Message) error { return nil }, alice.ID, func() { canceled.Store(true) })

		// Still allowed to read: nothing happens
		s.execManager()
		require.False(t, canceled.Load())

		// Revoked behind the server's back: the next manager run cancels the subscriber
		require.Nil(t, s.userManager.ResetAccess("alice", "private"))
		s.execManager()
		require.True(t, canceled.Load())
	})
}
