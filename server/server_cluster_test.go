package server

import (
	"net"
	"net/http"
	"net/netip"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// fakeCluster records broadcast messages so tests can assert that every publish path passes
// through the cluster broadcaster exactly once.
type fakeCluster struct {
	mu       sync.Mutex
	messages []*model.Message
}

func (b *fakeCluster) Broadcast(m *model.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, m)
	return nil
}

func (b *fakeCluster) ServeFanout(_ http.ResponseWriter, _ *http.Request) {}

func (b *fakeCluster) IsLeader() bool { return true }

func (b *fakeCluster) Close() error { return nil }

func (b *fakeCluster) Messages() []*model.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*model.Message{}, b.messages...)
}

func TestServer_Cluster_PublishBroadcastsOnce(t *testing.T) {
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeCluster{}
	s.cluster = b
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
	b := &fakeCluster{}
	s.cluster = b
	u := &user.User{ID: "u_abc", Name: "phil", SyncTopic: "st_1234"}
	v := s.visitor(netip.MustParseAddr("1.2.3.4"), nil)
	require.Nil(t, s.publishSyncEventForUser(v, u))
	messages := b.Messages()
	require.Len(t, messages, 1)
	require.Equal(t, "st_1234", messages[0].Topic)
}

func TestServer_Cluster_EndToEnd(t *testing.T) {
	// Two full servers sharing one Postgres schema: a message published to node A over HTTP must
	// reach a subscriber connected to node B, via the node registry and the fan-out endpoint.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	// Node B: create the listener first so its advertise URL is known before the server exists
	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	confB := newTestConfig(t, schemaDSN)
	confB.ClusterMode = true
	confB.ClusterNodeID = "node-b"
	confB.ClusterSecret = "s3cret"
	confB.ClusterAdvertiseURL = "http://" + listenerB.Addr().String()
	sB := newTestServer(t, confB)
	srvB := &http.Server{Handler: http.HandlerFunc(sB.handle)}
	go srvB.Serve(listenerB)
	defer srvB.Close()
	// Node A: publish-only in this test, so its advertise URL is never called
	confA := newTestConfig(t, schemaDSN)
	confA.ClusterMode = true
	confA.ClusterNodeID = "node-a"
	confA.ClusterSecret = "s3cret"
	confA.ClusterAdvertiseURL = "http://127.0.0.1:1"
	sA := newTestServer(t, confA)
	// Subscribe on node B
	topics, err := sB.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	var mu sync.Mutex
	var received []*model.Message
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m)
		return nil
	}, "", func() {})
	// Publish on node A
	response := request(t, sA, "PUT", "/mytopic", "hello cluster", nil)
	require.Equal(t, 200, response.Code)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "hello cluster", received[0].Message)
}

func TestServer_Cluster_DeliverFromBus(t *testing.T) {
	// deliverFromBus is the receive side of the broadcaster: a message that originated on a peer
	// node must reach this node's local subscribers, but must NOT be re-broadcast (loop) nor
	// re-trigger origin-only side effects.
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeCluster{}
	s.cluster = b
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
