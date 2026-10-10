package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
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

func TestServer_ManagerPrunesTopicWindows(t *testing.T) {
	// A topic prunes its recent window when a message arrives, so a topic that goes quiet would
	// hold its last messages until it is expunged, which on a busy server is hundreds of
	// thousands of topics worth. The manager sweeps them.
	conf := newTestConfig(t, "")
	conf.CacheBatchTimeout = 10 * time.Millisecond
	conf.ClusterBatchLinger = 0
	s := newTestServer(t, conf) // Window: 10ms + 0 + the 1s margin
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "remembered", nil).Code)
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	require.Len(t, recentAfter(topics[0].Recent(), "", 0), 1)

	time.Sleep(2100 * time.Millisecond) // The window, plus the second that whole-second message times can add
	s.execManager()
	require.Empty(t, recentAfter(topics[0].Recent(), "", 0))
}
