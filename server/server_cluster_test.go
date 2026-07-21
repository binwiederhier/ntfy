package server

import (
	"net/http"
	"net/netip"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// fakeBroadcaster records broadcast messages so tests can assert that every publish path passes
// through the cluster broadcaster exactly once.
type fakeBroadcaster struct {
	mu       sync.Mutex
	messages []*model.Message
}

func (b *fakeBroadcaster) Broadcast(m *model.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, m)
	return nil
}

func (b *fakeBroadcaster) ServeFanout(_ http.ResponseWriter, _ *http.Request) {}

func (b *fakeBroadcaster) IsLeader() bool { return true }

func (b *fakeBroadcaster) Close() error { return nil }

func (b *fakeBroadcaster) Messages() []*model.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*model.Message{}, b.messages...)
}

func TestServer_Cluster_PublishBroadcastsOnce(t *testing.T) {
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeBroadcaster{}
	s.broadcaster = b
	response := request(t, s, "PUT", "/mytopic", "hi there", nil)
	require.Equal(t, 200, response.Code)
	messages := b.Messages()
	require.Len(t, messages, 1)
	require.Equal(t, "mytopic", messages[0].Topic)
	require.Equal(t, "hi there", messages[0].Message)
}

func TestServer_Cluster_SyncEventBroadcasts(t *testing.T) {
	// Account sync events are delivered via the user's st_... sync topic; without broadcasting
	// them, cross-device account sync silently breaks when a user's devices land on different
	// cluster nodes.
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeBroadcaster{}
	s.broadcaster = b
	u := &user.User{ID: "u_abc", Name: "phil", SyncTopic: "st_1234"}
	v := s.visitor(netip.MustParseAddr("1.2.3.4"), nil)
	require.Nil(t, s.publishSyncEventForUser(v, u))
	messages := b.Messages()
	require.Len(t, messages, 1)
	require.Equal(t, "st_1234", messages[0].Topic)
}

func TestServer_Cluster_DeliverFromBus(t *testing.T) {
	// deliverFromBus is the receive side of the broadcaster: a message that originated on a peer
	// node must reach this node's local subscribers, but must NOT be re-broadcast (loop) nor
	// re-trigger origin-only side effects.
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeBroadcaster{}
	s.broadcaster = b
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	var mu sync.Mutex
	var received []*model.Message
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m)
		return nil
	}, "", func() {})
	m := model.NewDefaultMessage("mytopic", "from peer")
	m.Sender = netip.MustParseAddr("5.6.7.8")
	s.deliverFromBus(m)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	})
	require.Empty(t, b.Messages()) // Peer messages are never re-broadcast
}
