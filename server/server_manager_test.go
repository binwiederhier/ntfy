package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestServer_Manager_Prune_Messages_Without_Attachments_DoesNotPanic(t *testing.T) {
	// Tests that the manager runs without attachment-cache-dir set, see #617
	c := newTestConfig(t)
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
	require.Equal(t, errMessageNotFound, err)
}
