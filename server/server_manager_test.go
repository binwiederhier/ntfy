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
	topics[0].Subscribe(func(_ *visitor, _ *model.Message) error { return nil }, "", func() {}) // Not stale
	topics[0].mu.Lock()
	topics[0].delivered["old"] = time.Now().Add(-topicDeliveredRetain - time.Second)
	topics[0].mu.Unlock()
	s.execManager()
	_, ok := topics[0].DeliveredAt("old")
	require.False(t, ok)
}
