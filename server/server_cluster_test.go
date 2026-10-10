package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// fakeCluster records relayed messages and topic announcements so tests can assert that every
// publish path passes through the cluster exactly once, and that subscription hooks fire.
type fakeCluster struct {
	mu         sync.Mutex
	messages   []*model.Message
	announced  []string
	notLeader  bool
	notHealthy bool
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

func (b *fakeCluster) Members() []cluster.Member { return nil }

func (b *fakeCluster) Close() error { return nil }

func (b *fakeCluster) Messages() []*model.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*model.Message{}, b.messages...)
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
	}, "", func() {})
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
	require.Empty(t, b.Messages()) // Peer messages are never re-relayed
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
	// health checks must pull it from rotation (the fail-open policy lives in the checker)
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
	s.closeLocalSubscribers()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber connection still open")
	}
}

func TestServer_Cluster_StopDoesNotWaitForCallbackUnderServerLock(t *testing.T) {
	// The mesh's isolation loop calls back into the server (closeLocalSubscribers, which takes
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
			s.closeLocalSubscribers()
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

func TestServer_Cluster_GapReplaysOntoTheOpenConnection(t *testing.T) {
	// A peer dropped A for a topic, B on the same topic was delivered, and only then does the
	// report arrive. The client's marker is past A (and A's row id can be lower than B's), so no
	// reconnect of its own would ever ask for A; and a reconnect lands on whichever node the load
	// balancer picks. So this node replays the range itself, onto the connection it already has.
	s := newTestServer(t, newTestConfig(t, ""))
	s.replayDelay = 100 * time.Millisecond
	lost := model.NewDefaultMessage("mytopic", "lost in fan-out")
	lost.Time = time.Now().Add(-10 * time.Second).Unix()
	arrived := model.NewDefaultMessage("mytopic", "arrived fine")
	require.Nil(t, s.messageCache.AddMessages([]*model.Message{lost, arrived}))
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	received := make(chan *model.Message, 10)
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		received <- m
		return nil
	}, "", func() { t.Error("the subscriber must not be closed") })

	s.handleDeliveryGap([]string{"mytopic"}, lost.Time)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-received:
			if m.ID == lost.ID { // The hole, replayed without the client asking
				return
			}
		case <-deadline:
			t.Fatal("the lost message was never replayed")
		}
	}
}

func TestServer_Cluster_GapReplaysOnlyTheNewestVersionOfAnEditedMessage(t *testing.T) {
	// The subscriber may already hold v2 of an edited message, and the clients keep whichever
	// version they STORED last: replaying v1 would retire v2 and show v1. So the replay carries
	// one message per sequence, the newest.
	s := newTestServer(t, newTestConfig(t, ""))
	s.replayDelay = 100 * time.Millisecond
	v1 := model.NewDefaultMessage("mytopic", "first draft")
	v1.Time = time.Now().Add(-10 * time.Second).Unix()
	v2 := model.NewDefaultMessage("mytopic", "final text")
	v2.SequenceID = v1.ID
	require.Nil(t, s.messageCache.AddMessages([]*model.Message{v1, v2}))
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	var mu sync.Mutex
	var received []string
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m.Message)
		return nil
	}, "", func() {})

	s.handleDeliveryGap([]string{"mytopic"}, v1.Time)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) > 0
	})
	time.Sleep(200 * time.Millisecond) // Room for a wrong implementation to send the second one
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"final text"}, received)
}

func TestServer_Cluster_UndatedAndAllTopicsGapsClose(t *testing.T) {
	// An undated report (a peer on an older build) and "every topic" carry no position, so there
	// is nothing to replay: the subscribers are closed and their own markers decide.
	s := newTestServer(t, newTestConfig(t, ""))
	topics, err := s.topicsFromIDs(nil, "mytopic", "othertopic")
	require.Nil(t, err)
	var canceled atomic.Int32
	topics[0].Subscribe(func(*visitor, *model.Message) error { return nil }, "", func() { canceled.Add(1) })
	topics[1].Subscribe(func(*visitor, *model.Message) error { return nil }, "", func() { canceled.Add(1) })

	s.handleDeliveryGap([]string{"mytopic"}, 0)
	require.Equal(t, int32(1), canceled.Load()) // Only the gapped topic
	s.handleDeliveryGap([]string{cluster.GapAllTopics}, time.Now().Unix())
	require.Equal(t, int32(3), canceled.Load()) // Every subscriber, including mytopic's again: the fake cancel does not unsubscribe
}

func TestServer_Cluster_SinceIDReplaysWithOverlapInACluster(t *testing.T) {
	// Nodes flush their write batches independently, so across nodes a marker's row id can sit
	// above a message published before it. In a cluster since=<id> therefore replays by time with
	// replayDelay of overlap; single-node replay stays exact.
	older := model.NewDefaultMessage("mytopic", "published first, flushed last")
	older.Time = time.Now().Add(-2 * time.Second).Unix()
	marker := model.NewDefaultMessage("mytopic", "the client's marker")
	marker.Time = time.Now().Add(-1 * time.Second).Unix()

	single := newTestServer(t, newTestConfig(t, ""))
	require.Nil(t, single.messageCache.AddMessages([]*model.Message{marker, older})) // Flushed in this order: older gets the higher row id
	require.Len(t, toMessages(t, request(t, single, "GET", "/mytopic/json?poll=1&since="+marker.ID, "", nil).Body.String()), 1)

	conf := newTestConfig(t, dbtest.CreateTestPostgresSchema(t))
	conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
	conf.ClusterNodeID = "node-a"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	clustered := newTestServer(t, conf)
	require.Nil(t, clustered.messageCache.AddMessages([]*model.Message{marker, older}))
	messages := toMessages(t, request(t, clustered, "GET", "/mytopic/json?poll=1&since="+marker.ID, "", nil).Body.String())
	require.Len(t, messages, 1) // The overlap, but never the marker itself: the client has that one by definition
	require.Equal(t, "published first, flushed last", messages[0].Message)
}

// breakable is a cluster listener whose peer API can be made to reject everything, which is what
// a peer sees while a node restarts (the 74ms window between registering and listening) or is
// otherwise unreachable: the batch is refused, and the origin records a gap.
type breakable struct {
	handler http.Handler
	broken  atomic.Bool
}

func (b *breakable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b.broken.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	b.handler.ServeHTTP(w, r)
}

// clusterNode is one full server in a test cluster, with its peer API on a real loopback listener
type clusterNode struct {
	*Server
	peerAPI *breakable
}

// newTestClusterNode starts a full server as a cluster member on a shared schema, serving its
// peer API on loopback. Fan-out lingers briefly so the tests run fast; the heartbeat (3s) is the
// real one, so a gap report takes up to one heartbeat to reach its peer.
func newTestClusterNode(t *testing.T, schemaDSN, nodeID string) *clusterNode {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = nodeID
	conf.ClusterListen = listener.Addr().String()
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://" + listener.Addr().String()
	conf.ClusterBatchLinger = 50 * time.Millisecond
	s := newTestServer(t, conf)
	peerAPI := &breakable{handler: s.cluster}
	srv := &http.Server{Handler: peerAPI}
	go srv.Serve(listener)
	t.Cleanup(func() { srv.Close() })
	return &clusterNode{Server: s, peerAPI: peerAPI}
}

// newTestCluster starts three nodes on one schema and waits until each sees the other two
func newTestCluster(t *testing.T) (a, b, c *clusterNode) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	a, b, c = newTestClusterNode(t, schemaDSN, "node-a"), newTestClusterNode(t, schemaDSN, "node-b"), newTestClusterNode(t, schemaDSN, "node-c")
	for _, n := range []*clusterNode{a, b, c} {
		waitFor(t, func() bool { return len(n.cluster.Members()) == 3 })
	}
	return a, b, c
}

// streamClient is a subscriber connected to one node, the way a phone holds a stream open
type streamClient struct {
	mu       sync.Mutex
	received []*model.Message
	closed   atomic.Bool
}

func subscribeStream(t *testing.T, n *clusterNode, topic string) *streamClient {
	c := &streamClient{}
	topics, err := n.topicsFromIDs(nil, topic)
	require.Nil(t, err)
	topics[0].Subscribe(func(_ *visitor, m *model.Message) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.received = append(c.received, m)
		return nil
	}, "", func() { c.closed.Store(true) })
	return c
}

func (c *streamClient) ids() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.received))
	for _, m := range c.received {
		ids = append(ids, m.ID)
	}
	return ids
}

func (c *streamClient) has(id string) bool {
	return slices.Contains(c.ids(), id)
}

// waitForReplay waits long enough for a gap report (one heartbeat) plus the replay delay
func waitForReplay(t *testing.T, f func() bool) {
	waitForWithMaxWait(t, 15*time.Second, f)
}

func publishOn(t *testing.T, n *clusterNode, path, body string) *model.Message {
	response := request(t, n.Server, "PUT", path, body, nil)
	require.Equal(t, 200, response.Code)
	return toMessage(t, response.Body.String())
}

func TestServer_Cluster3_LostBatchIsReplayedOntoTheOpenConnection(t *testing.T) {
	// Node B's peer API is down (a restart) while A publishes. The batch is refused, nothing
	// newer follows, and B's subscriber still gets the message, on the connection it has.
	a, b, _ := newTestCluster(t)
	phone := subscribeStream(t, b, "alerts")
	b.peerAPI.broken.Store(true)
	lost := publishOn(t, a, "/alerts", "sent while B was restarting")
	time.Sleep(500 * time.Millisecond) // Let the batch fail
	b.peerAPI.broken.Store(false)

	waitForReplay(t, func() bool { return phone.has(lost.ID) })
	require.False(t, phone.closed.Load(), "the subscriber must not be closed")
}

func TestServer_Cluster3_LostBatchThenNewerFromAnotherNodeInTheSameSecond(t *testing.T) {
	// The case a client's own marker can never repair: A's message to B is lost, C's message on
	// the same topic arrives right after, in the same second. The subscriber on B must end up
	// with both, the lost one replayed after the newer one.
	a, b, c := newTestCluster(t)
	phone := subscribeStream(t, b, "alerts")
	b.peerAPI.broken.Store(true)
	lost := publishOn(t, a, "/alerts", "from A, lost")
	time.Sleep(500 * time.Millisecond)
	b.peerAPI.broken.Store(false)
	newer := publishOn(t, c, "/alerts", "from C, delivered")
	waitFor(t, func() bool { return phone.has(newer.ID) })
	require.False(t, phone.has(lost.ID)) // Live delivery really did miss it

	waitForReplay(t, func() bool { return phone.has(lost.ID) })
	ids := phone.ids()
	require.Greater(t, slices.Index(ids, lost.ID), slices.Index(ids, newer.ID), "the lost message arrives after the newer one")
	require.False(t, phone.closed.Load())
}

func TestServer_Cluster3_LostBatchThenNewerSecondsLater(t *testing.T) {
	// Same, with the newer message well after the lost one, so the two are not in the same second
	a, b, c := newTestCluster(t)
	phone := subscribeStream(t, b, "alerts")
	b.peerAPI.broken.Store(true)
	lost := publishOn(t, a, "/alerts", "from A, lost")
	time.Sleep(500 * time.Millisecond)
	b.peerAPI.broken.Store(false)
	time.Sleep(2 * time.Second)
	newer := publishOn(t, c, "/alerts", "from C, two seconds later")
	waitFor(t, func() bool { return phone.has(newer.ID) })

	waitForReplay(t, func() bool { return phone.has(lost.ID) })
	require.False(t, phone.closed.Load())
}

func TestServer_Cluster3_EditedMessageKeepsItsNewestVersion(t *testing.T) {
	// v1 of an edited message is lost on the way to B, v2 (same sequence id) comes through C.
	// The replay must not hand B's subscriber v1: the clients keep whichever version they stored
	// last, so v1 after v2 would show the stale text.
	a, b, c := newTestCluster(t)
	phone := subscribeStream(t, b, "alerts")
	b.peerAPI.broken.Store(true)
	v1 := publishOn(t, a, "/alerts/seq-1", "first draft")
	time.Sleep(500 * time.Millisecond)
	b.peerAPI.broken.Store(false)
	v2 := publishOn(t, c, "/alerts/seq-1", "final text")
	require.Equal(t, v1.SequenceID, v2.SequenceID)
	waitFor(t, func() bool { return phone.has(v2.ID) })

	// The replay runs after the report (one heartbeat) and the replay delay; give it time to go
	// wrong, then check v1 never arrived
	time.Sleep(6 * time.Second)
	require.False(t, phone.has(v1.ID), "the stale version must not be replayed")
	require.True(t, phone.has(v2.ID))
}

func TestServer_Cluster3_ClientReconnectsToAnotherNodeAfterTheGap(t *testing.T) {
	// B's subscriber got C's message live while A's was lost, then dropped off before the replay
	// reached it and came back through the load balancer on C, with the newer message as its
	// marker. In a cluster since=<id> replays by time with overlap, so C hands it the lost one.
	a, b, c := newTestCluster(t)
	phone := subscribeStream(t, b, "alerts")
	b.peerAPI.broken.Store(true)
	lost := publishOn(t, a, "/alerts", "from A, lost")
	time.Sleep(500 * time.Millisecond)
	b.peerAPI.broken.Store(false)
	newer := publishOn(t, c, "/alerts", "from C, delivered")
	waitFor(t, func() bool { return phone.has(newer.ID) })

	messages := toMessages(t, request(t, c.Server, "GET", "/alerts/json?poll=1&since="+newer.ID, "", nil).Body.String())
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		ids = append(ids, m.ID)
	}
	require.Contains(t, ids, lost.ID)
}

func TestServer_Cluster3_ReconnectCoversOutOfOrderRowIDs(t *testing.T) {
	// No failure at all: X is published on A and Y on C right after, so X has the lower row id.
	// A client that saw Y but dropped off before X's fan-out reached it reconnects with since=Y.
	// Row-id replay would skip X; the cluster replays by time with overlap instead.
	a, b, c := newTestCluster(t)
	x := publishOn(t, a, "/alerts", "X, lower row id")
	y := publishOn(t, c, "/alerts", "Y, the marker")
	waitFor(t, func() bool {
		stored, err := b.messageCache.Message(x.ID)
		return err == nil && stored != nil
	})

	messages := toMessages(t, request(t, b.Server, "GET", "/alerts/json?poll=1&since="+y.ID, "", nil).Body.String())
	require.Len(t, messages, 1) // X; never Y itself, which the client has by definition
	require.Equal(t, x.ID, messages[0].ID)
}

func TestServer_Cluster_ReconnectNeverResendsTheMarkerFromTheTopicWindow(t *testing.T) {
	// A cluster replays since=<id> by time, and the topic's window is read by time too, so the
	// client's own marker, which it has by definition, could come back from the window on every
	// reconnect while it is still remembered. It must not.
	conf := newTestConfig(t, dbtest.CreateTestPostgresSchema(t))
	conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
	conf.ClusterNodeID = "node-a"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	s := newTestServer(t, conf)
	marker := toMessage(t, request(t, s, "PUT", "/mytopic", "the marker", nil).Body.String())
	waitFor(t, func() bool {
		stored, err := s.messageCache.Message(marker.ID)
		return err == nil && stored != nil
	})
	require.Empty(t, toMessages(t, request(t, s, "GET", "/mytopic/json?poll=1&since="+marker.ID, "", nil).Body.String()))
}
