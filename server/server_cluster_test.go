package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// fakeCluster records relayed messages and topic announcements so tests can assert that every
// publish path passes through the cluster exactly once, and that subscription hooks fire.
type fakeCluster struct {
	mu            sync.Mutex
	messages      []*model.Message
	announced     []string
	deletedUsers  []string
	accessChanged int
	notLeader     bool
	notHealthy    bool
	notReady      bool
	role          cluster.Role // Empty means active
	draining      bool
	peers         cluster.Peers
}

func (b *fakeCluster) ForwardMessage(m *model.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, m)
	return nil
}

func (b *fakeCluster) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func (b *fakeCluster) BroadcastState(state *cluster.State) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.announced = append(b.announced, state.AddedTopics...)
	b.deletedUsers = append(b.deletedUsers, state.DeletedUserIDs...)
	if state.AccessChanged {
		b.accessChanged++
	}
}

func (b *fakeCluster) BroadcastBuckets(_ *limit.Buckets) {}

func (b *fakeCluster) PeerBuckets(_ context.Context) map[cluster.NodeID]*limit.Buckets {
	return nil
}

func (b *fakeCluster) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.notReady && !b.draining // Like the mesh: a draining node acts as passive
}

func (b *fakeCluster) setReady(ready bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notReady = !ready
}

func (b *fakeCluster) Role() cluster.Role {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.draining {
		return cluster.RolePassive // Like the mesh: the role it acts in
	} else if b.role == "" {
		return cluster.RoleActive
	}
	return b.role
}

func (b *fakeCluster) Healthy() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.notHealthy
}

func (b *fakeCluster) setHealthy(healthy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notHealthy = !healthy
}

func (b *fakeCluster) IsLeader() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.notLeader
}

func (b *fakeCluster) setLeader(leader bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notLeader = !leader
}

func (b *fakeCluster) SetDraining(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.draining = on
}

func (b *fakeCluster) Draining() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.draining
}

func (b *fakeCluster) Peers() cluster.Peers {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peers
}

func (b *fakeCluster) Members() []cluster.Member {
	return nil // No test needs members
}

func (b *fakeCluster) Close() error { return nil }

func (b *fakeCluster) Messages() []*model.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*model.Message{}, b.messages...)
}

func (b *fakeCluster) AccessChanged() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.accessChanged
}

func (b *fakeCluster) DeletedUsers() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.deletedUsers
}

func (b *fakeCluster) Announced() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.announced...)
}

func TestServer_Cluster_PublishForwardsOnce(t *testing.T) {
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

func TestServer_Cluster_SyncEventForwards(t *testing.T) {
	// Account sync events are delivered via the user's st_... sync topic; without relaying
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

func TestServer_Cluster_DeliverNotOnPublicHandler(t *testing.T) {
	// The fan-out endpoint lives only on the dedicated cluster listener; the public handler must
	// not serve it, even with cluster mode on and a valid secret.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	s := newTestServer(t, conf)
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	var mu sync.Mutex
	var received []*model.Message
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m)
		return nil
	}, nil, func() {})
	// A valid fan-out request against the PUBLIC handler must not deliver
	response := request(t, s, "POST", "/v1/cluster/message",
		`{"message":{"id":"x1","time":1,"event":"message","topic":"mytopic","message":"sneaky"}}`,
		map[string]string{"X-Cluster-Secret": "s3cret", "X-Cluster-Origin": "node-b"})
	require.Equal(t, 404, response.Code)
	time.Sleep(250 * time.Millisecond) // Delivery is async; give a wrong implementation time to fail
	mu.Lock()
	require.Empty(t, received)
	mu.Unlock()
	// The same request against the cluster listener handler DOES deliver
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/cluster/message",
		strings.NewReader(`{"message":{"id":"x2","time":1,"event":"message","topic":"mytopic","message":"legit"}}`))
	require.Nil(t, err)
	req.Header.Set("X-Cluster-Secret", "s3cret")
	req.Header.Set("X-Cluster-Origin", "node-b")
	s.cluster.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	})
}

func TestServer_Cluster_Role(t *testing.T) {
	// The configured role reaches the cluster, which reports it to the LB agents
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	conf.ClusterRole = cluster.RolePassive
	s := newTestServer(t, conf)
	members := s.cluster.Members()
	require.Len(t, members, 1)
	require.Equal(t, cluster.RolePassive, members[0].Role)
}

func TestServer_Cluster_StopClosesCluster(t *testing.T) {
	// Stopping the server must stop the cluster: it deregisters the node, so a node starting
	// after it no longer sees it, and its heartbeat does not outlive the server's database
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		return newTestServer(t, conf)
	}
	sA := newNode("node-a")
	require.Len(t, sA.cluster.Members(), 1)
	sA.Stop()
	sB := newNode("node-b")
	members := sB.cluster.Members()
	require.Len(t, members, 1)
	require.Equal(t, cluster.NodeID("node-b"), members[0].NodeID)
}

func TestServer_Cluster_ActiveWithPassivePeer(t *testing.T) {
	// The setup for trying clustering beside production: the production server is the only
	// active node, a second one runs passive. The active one leads right away, both deliver
	// each other's messages, and only the active one leads.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string, role cluster.Role) *Server {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.Nil(t, err)
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = listener.Addr().String()
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://" + listener.Addr().String()
		conf.ClusterRole = role
		s := newTestServer(t, conf)
		srv := &http.Server{Handler: s.cluster}
		go srv.Serve(listener)
		t.Cleanup(func() { srv.Close() })
		return s
	}
	passive := newNode("node-a", cluster.RolePassive) // Lower id, but passive
	started := time.Now()
	active := newNode("node-b", cluster.RoleActive)
	waitFor(t, func() bool { return active.cluster.IsLeader() })
	require.Less(t, time.Since(started), 5*time.Second) // Not after a one-minute hold-off
	require.False(t, passive.cluster.IsLeader())

	// Both nodes know each other before publishing, or the fan-out has no peer to go to
	waitFor(t, func() bool { return len(active.cluster.Members()) == 2 && len(passive.cluster.Members()) == 2 })
	receive := func(s *Server, topic string) func() []string {
		topics, err := s.topicsFromIDs(nil, topic)
		require.Nil(t, err)
		var mu sync.Mutex
		received := make([]string, 0)
		topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, m.Message)
			return nil
		}, nil, func() {})
		return func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string{}, received...)
		}
	}
	onPassive := receive(passive, "toward-passive")
	onActive := receive(active, "toward-active")
	require.Equal(t, 200, request(t, active, "PUT", "/toward-passive", "from active", nil).Code)
	require.Equal(t, 200, request(t, passive, "PUT", "/toward-active", "from passive", nil).Code)
	waitFor(t, func() bool { return len(onPassive()) == 1 && len(onActive()) == 1 })
	require.Equal(t, []string{"from active"}, onPassive())
	require.Equal(t, []string{"from passive"}, onActive())
	require.Equal(t, 200, request(t, active, "GET", "/v1/health", "", nil).Code)
}

func TestServer_Cluster_EndToEnd(t *testing.T) {
	// Two full servers sharing one Postgres schema: a message published to node A over HTTP must
	// reach a subscriber connected to node B, via the node registry and the fan-out endpoint.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	// Node B: create the listener first so its advertise URL is known before the server exists
	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	confB := newTestConfig(t, schemaDSN)
	confB.ClusterNodeID = "node-b"
	confB.ClusterListen = listenerB.Addr().String() // Enables clustering; the test serves it below
	confB.ClusterSecret = "s3cret"
	confB.ClusterAdvertiseURL = "http://" + listenerB.Addr().String()
	sB := newTestServer(t, confB)
	srvB := &http.Server{Handler: sB.cluster}
	go srvB.Serve(listenerB)
	defer srvB.Close()
	// Node A: publish-only in this test, so its advertise URL is never called
	confA := newTestConfig(t, schemaDSN)
	confA.ClusterNodeID = "node-a"
	confA.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
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
	}, nil, func() {})
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

func TestServer_Cluster_RequestLimitAcrossNodes(t *testing.T) {
	// A client spread over two nodes gets its request limit once, not once per node: the requests
	// node A admits for its visitor key are charged into node B's bucket for the same key
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	sB := newTestClusterNode(t, schemaDSN, "node-b", "").s
	sA := newTestClusterNode(t, schemaDSN, "node-a", "").s

	// One request on node B, nine left there; then the whole burst on node A
	require.Equal(t, 200, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Code)
	for i := 0; i < 10; i++ {
		require.Equal(t, 200, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Code)
	}
	require.Equal(t, 429, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Code)

	// Node A tells node B, which then has no tokens left either (watched without spending any)
	sA.gossipBuckets()
	waitFor(t, func() bool {
		return sB.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule) == 0
	})
	require.Equal(t, 429, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Code)
}

func TestServer_Cluster_ModerateClientAcrossNodes(t *testing.T) {
	// A client that alternates between two nodes never drains either node's bucket below half on
	// its own; it still gets its burst once, not once per node
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	sB := newTestClusterNode(t, schemaDSN, "node-b", "").s
	sA := newTestClusterNode(t, schemaDSN, "node-a", "").s
	waitFor(t, func() bool {
		return len(sB.cluster.Peers()) == 1 // B registered first and sees A from its next heartbeat
	})
	for i := 0; i < 5; i++ {
		require.Equal(t, 200, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Code)
		require.Equal(t, 200, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Code)
	}
	sA.gossipBuckets() // The gossip loops may have sent some rounds already; these send the rest
	sB.gossipBuckets()
	waitFor(t, func() bool {
		return sA.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule) == 0 && sB.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule) == 0
	})
	require.Equal(t, 429, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Code)
	require.Equal(t, 429, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Code)
}

func TestServer_Cluster_RollingRestartKeepsBuckets(t *testing.T) {
	// A node that restarts takes the buckets over from its peers before it serves, so a rolling
	// restart forgives nothing as long as one node is up: node A's spent burst survives A's
	// restart (from B), and then B's (from the new A)
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	nodeB := newTestClusterNode(t, schemaDSN, "node-b", "")
	nodeA := newTestClusterNode(t, schemaDSN, "node-a", "")
	for i := 0; i < 10; i++ {
		require.Equal(t, 200, request(t, nodeA.s, "GET", "/mytopic/json?poll=1", "", nil).Code)
	}
	nodeA.s.gossipBuckets()
	waitFor(t, func() bool {
		return nodeB.s.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule) == 0
	})

	nodeA.stop()
	nodeA = newTestClusterNode(t, schemaDSN, "node-a", nodeA.listen)
	require.Equal(t, 429, request(t, nodeA.s, "GET", "/mytopic/json?poll=1", "", nil).Code)

	nodeB.stop()
	nodeB = newTestClusterNode(t, schemaDSN, "node-b", nodeB.listen)
	require.Equal(t, 429, request(t, nodeB.s, "GET", "/mytopic/json?poll=1", "", nil).Code)
}

func TestServer_Cluster_StopWithGossipRunning(t *testing.T) {
	// A node that stops while its gossip loop holds a round it has not sent yet still gets that
	// round to its peers: the loop is stopped (and finishes its round) before the last round and
	// before the cluster closes
	entered, release := make(chan struct{}, 1), make(chan struct{})
	testHookBucketGossip = func(s *Server, round *limit.Buckets) {
		if s.config.ClusterNodeID == "node-a" && len(round.Usages) > 0 {
			entered <- struct{}{}
			<-release
		}
	}
	t.Cleanup(func() {
		testHookBucketGossip = nil // Registered before the nodes, so it runs after they stopped
	})
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	nodeB := newTestClusterNode(t, schemaDSN, "node-b", "")
	nodeA := newTestClusterNode(t, schemaDSN, "node-a", "")
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release) // Runs before the nodes stop, so a failed test does not leave a loop blocked
		}
	})
	for i := 0; i < 10; i++ {
		require.Equal(t, 200, request(t, nodeA.s, "GET", "/mytopic/json?poll=1", "", nil).Code)
	}
	select {
	case <-entered: // The loop took the round, and has not sent it
	case <-time.After(10 * time.Second):
		t.Fatal("the gossip loop did not take a round")
	}
	stopped := make(chan struct{})
	go func() {
		nodeA.stop()
		close(stopped)
	}()
	waitFor(t, func() bool {
		return nodeA.s.stopped.Load() // Stop is waiting for the loop
	})
	close(release)
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("stop did not finish")
	}
	require.Equal(t, int64(0), nodeB.s.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule))
}

func TestServer_Cluster_StopSendsLastBuckets(t *testing.T) {
	// A node that stops sends what it used since its last gossip round before it goes, so an
	// orderly restart loses nothing: the burst node A just spent is on node B once A has stopped
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	nodeB := newTestClusterNode(t, schemaDSN, "node-b", "")
	nodeA := newTestClusterNode(t, schemaDSN, "node-a", "")
	for i := 0; i < 10; i++ {
		require.Equal(t, 200, request(t, nodeA.s, "GET", "/mytopic/json?poll=1", "", nil).Code)
	}
	nodeA.stop()
	require.Equal(t, int64(0), nodeB.s.limiter.Tokens("ip:9.9.9.9", testClusterRequestRule))
}

// testClusterRequestRule is the request bucket of the nodes newTestClusterNode starts
var testClusterRequestRule = limit.Rule{
	Kind:   limit.KindRequests,
	Burst:  10,
	Refill: time.Hour,
}

// testClusterNode is a cluster node of a test, running (Run) with its peer API on listen
type testClusterNode struct {
	s      *Server
	listen string
}

// newTestClusterNode runs a cluster node with a request burst of 10 (testClusterRequestRule) whose
// peer API listens on listen, or on a free port if empty, and waits until the peer API answers
func newTestClusterNode(t *testing.T, schemaDSN, nodeID, listen string) *testClusterNode {
	t.Helper()
	if listen == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.Nil(t, err)
		listen = listener.Addr().String()
		require.Nil(t, listener.Close())
	}
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = nodeID
	conf.ClusterListen = listen
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://" + listen
	conf.VisitorRequestLimitBurst = int(testClusterRequestRule.Burst)
	conf.VisitorRequestLimitReplenish = testClusterRequestRule.Refill
	node := &testClusterNode{
		s:      newTestServer(t, conf),
		listen: listen,
	}
	runTestServer(t, node.s)
	waitFor(t, func() bool {
		conn, err := net.Dial("tcp", listen)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	})
	return node
}

// stop stops the node, freeing its address for a restart
func (n *testClusterNode) stop() {
	n.s.Stop()
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
	}, nil, func() {})
	m := model.NewDefaultMessage("mytopic", "from peer")
	m.Sender = netip.MustParseAddr("5.6.7.8")
	s.deliverFromBus(m)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	})
	require.Empty(t, b.Messages()) // Peer messages are never re-relayed
}

func TestServer_Cluster_DeliverFromBus_WastedWithoutSubscribers(t *testing.T) {
	// A fan-out message that no subscriber here wants is counted as wasted: its topic is unknown
	// here, or known but without subscribers. The latter is not published either, so the fan-out
	// does not keep an idle topic alive (see topicExpungeAfter).
	s := newTestServer(t, newTestConfig(t, ""))
	s.cluster = &fakeCluster{}
	topics, err := s.topicsFromIDs(nil, "idletopic")
	require.Nil(t, err)
	idle := topics[0]
	idle.mu.Lock()
	idle.lastAccess = time.Now().Add(-topicExpungeAfter - time.Minute)
	idle.mu.Unlock()
	wasted := testutil.ToFloat64(metrics.ClusterMessagesWasted)

	// Unknown topic
	s.deliverFromBus(model.NewDefaultMessage("unknowntopic", "from peer"))
	require.Equal(t, wasted+1, testutil.ToFloat64(metrics.ClusterMessagesWasted))

	// Known topic without subscribers: wasted, and still stale
	s.deliverFromBus(model.NewDefaultMessage("idletopic", "from peer"))
	require.Equal(t, wasted+2, testutil.ToFloat64(metrics.ClusterMessagesWasted))
	require.True(t, idle.Stale())

	// Known topic with a subscriber: delivered, not wasted
	idle.Subscribe(func(_ *visitor, m *model.Message) error {
		return nil
	}, nil, func() {})
	s.deliverFromBus(model.NewDefaultMessage("idletopic", "from peer"))
	require.Equal(t, wasted+2, testutil.ToFloat64(metrics.ClusterMessagesWasted))
}

func TestServer_Cluster_FirebaseKeepaliverOnlyOnLeader(t *testing.T) {
	// Every FCM keepalive wakes all subscribed phones, so only the leader may send them;
	// N nodes sending N keepalives would multiply the battery cost for every user
	c := newTestConfig(t, "")
	c.FirebaseKeepaliveInterval = 20 * time.Millisecond
	s := newTestServer(t, c)
	sender := newTestFirebaseSender(100)
	s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true})
	cl := &fakeCluster{notLeader: true}
	s.cluster = cl
	s.closeChan = make(chan bool) // Closed by Stop() in the test cleanup
	go s.runFirebaseKeepaliver()

	// A non-leader node stays silent
	time.Sleep(150 * time.Millisecond)
	require.Empty(t, sender.Messages())

	// The leader sends keepalives
	cl.setLeader(true)
	waitFor(t, func() bool { return len(sender.Messages()) > 0 })
}

func TestServer_Cluster_HealthReflectsCluster(t *testing.T) {
	// A node whose registry heartbeat went stale no longer receives forwarded messages, so
	// health reports it (load balancers check /v1/ready, see TestServer_Cluster_Ready)
	s := newTestServer(t, newTestConfig(t, ""))
	cl := &fakeCluster{}
	s.cluster = cl
	rr := request(t, s, "GET", "/v1/health", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `"healthy":true`)
	cl.setHealthy(false)
	rr = request(t, s, "GET", "/v1/health", "", nil)
	require.Equal(t, 503, rr.Code)
	require.Contains(t, rr.Body.String(), `"healthy":false`)
}

func TestServer_Cluster_IsolatedNodeClosesSubscribers(t *testing.T) {
	// Called by the cluster when this node lost its registration while peers are healthy:
	// every local subscriber connection must end, so clients reconnect to a healthy node
	conf := newTestConfig(t, "")
	conf.CacheDuration = 0 // No cache, so no tail: nothing else gets messages to the subscribers
	s := newTestServer(t, conf)
	done := make(chan struct{})
	go func() {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/mytopic/json", nil)
		s.handle(rr, req)
		close(done)
	}()
	waitFor(t, func() bool {
		topics := topicsSnapshot(s)
		t, ok := topics["mytopic"]
		return ok && t.SubscribersCount() == 1
	})
	s.maybeCloseLocalSubscribers()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber connection still open")
	}
}

func TestServer_Cluster_IsolatedNodeKeepsSubscribersWhileTheTailReads(t *testing.T) {
	// Peers may have dropped this node, but as long as its tail reads the database its
	// subscribers still get every message, a little late. Closing them would send them to
	// nodes that are just as slow (a slow database is what makes heartbeats fail), in a storm.
	s := newTestServer(t, newTestConfig(t, ""))
	done := make(chan struct{})
	go func() {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/mytopic/json", nil)
		s.handle(rr, req)
		close(done)
	}()
	waitFor(t, func() bool {
		topics := topicsSnapshot(s)
		t, ok := topics["mytopic"]
		return ok && t.SubscribersCount() == 1
	})
	waitFor(t, func() bool { return !s.tail.LastRead().IsZero() })
	s.maybeCloseLocalSubscribers()
	select {
	case <-done:
		t.Fatal("subscriber connection closed although the tail reads")
	case <-time.After(2 * time.Second):
	}
}

func TestServer_Cluster_StopDoesNotWaitForCallbackUnderServerLock(t *testing.T) {
	// The mesh's isolation loop calls back into the server (maybeCloseLocalSubscribers, which takes
	// s.mu). Stop must not hold s.mu while it waits for that loop, or a callback already
	// waiting for the lock deadlocks shutdown before the message cache is drained.
	s, err := New(newTestConfig(t, ""))
	require.NoError(t, err)
	// Deliberately no Stop cleanup: a failing test demonstrates the deadlock
	host, err := pg.Open(dbtest.CreateTestPostgresSchema(t))
	require.NoError(t, err)
	pool := db.New(host, nil)
	// A peer that answers the health probe, so this node considers itself isolated rather than
	// looking at a cluster-wide outage
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"healthy":true}`) // The isolation probe requires the health answer, not just a 200
	}))
	defer peer.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c, err := cluster.New(&cluster.Config{
		Enabled: true, NodeID: "a", AdvertiseURL: "http://127.0.0.1:1", Secret: "test",
		// The TTL must be at least a second: last_heartbeat is whole seconds, so a sub-second
		// TTL truncates the liveness cutoff to 0 and the peer inserted below counts as live
		// only within the same wall-clock second, leaving nothing to probe for isolation
		HeartbeatInterval: 10 * time.Millisecond, NodeTTL: time.Second,
		IsolatedFunc: func() {
			once.Do(func() { close(entered) })
			<-release
			s.maybeCloseLocalSubscribers()
		},
	}, pool, func(*model.Message) {})
	require.NoError(t, err)
	s.cluster = c
	// The registry table exists now; the peer must be in it (and reconciled by a heartbeat)
	// before the database goes away, or the isolation check has nobody to probe
	_, err = pool.Exec(`INSERT INTO node_registry VALUES ('b', $1, $2)`, peer.URL, time.Now().Unix())
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	// Break the database: this node's registration goes stale, the peer above stays healthy,
	// and the isolation loop calls IsolatedFunc, which parks inside the callback
	require.NoError(t, pool.Close())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("isolation callback not reached")
	}
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	// Give Stop time to reach cluster.Close (and, in the buggy version, to take s.mu first),
	// then let the parked callback continue: it needs s.mu to finish
	time.Sleep(100 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Stop holds s.mu while cluster.Close waits for an isolation callback needing s.mu")
	}
}

// topicsSnapshot copies the server's topic map, so a test can look at it without holding the lock
func topicsSnapshot(s *Server) map[string]*topic {
	s.mu.RLock()
	defer s.mu.RUnlock()
	topics := make(map[string]*topic, len(s.topics))
	for id, t := range s.topics {
		topics[id] = t
	}
	return topics
}

func TestServer_Cluster_HealthReportsRole(t *testing.T) {
	// A passive node is healthy: role and health are separate answers
	s := newTestServer(t, newTestConfig(t, ""))
	s.cluster = &fakeCluster{role: cluster.RolePassive}
	rr := request(t, s, "GET", "/v1/health", "", nil)
	require.Equal(t, 200, rr.Code)
	require.JSONEq(t, `{"healthy":true,"role":"passive"}`, rr.Body.String())
}

func TestServer_Cluster_Ready(t *testing.T) {
	// Readiness is what a load balancer checks; the cluster decides it (see Cluster.Ready)
	s := newTestServer(t, newTestConfig(t, ""))
	cl := &fakeCluster{}
	s.cluster = cl
	rr := request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 200, rr.Code)
	require.JSONEq(t, `{"ready":true,"role":"active"}`, rr.Body.String())

	cl.setReady(false)
	cl.mu.Lock()
	cl.role = cluster.RolePassive
	cl.mu.Unlock()
	rr = request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 503, rr.Code)
	require.JSONEq(t, `{"ready":false,"role":"passive"}`, rr.Body.String())
}

func TestServer_Cluster_HealthIsTheHeartbeat(t *testing.T) {
	// The only active node with a stale heartbeat (database unreachable) is unhealthy, which
	// monitoring should see, but stays ready, since there is nowhere better for its clients
	s := newTestServer(t, newTestConfig(t, ""))
	cl := &fakeCluster{}
	s.cluster = cl
	cl.setHealthy(false)
	rr := request(t, s, "GET", "/v1/health", "", nil)
	require.Equal(t, 503, rr.Code)
	require.JSONEq(t, `{"healthy":false,"role":"active"}`, rr.Body.String())
	rr = request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 200, rr.Code)
}

func TestServer_ReadyWithoutClustering(t *testing.T) {
	s := newTestServer(t, newTestConfig(t, ""))
	rr := request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 200, rr.Code)
	require.JSONEq(t, `{"ready":true,"role":"active"}`, rr.Body.String())
}
