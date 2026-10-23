package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
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
	mu          sync.Mutex
	messages    []*model.Message
	announced   []string
	notLeader   bool
	notHealthy  bool
	notReady    bool
	role        cluster.Role // Empty means active
	maintenance bool
	peers       cluster.Peers
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

func (b *fakeCluster) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.notReady
}

func (b *fakeCluster) setReady(ready bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notReady = !ready
}

func (b *fakeCluster) Role() cluster.Role {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maintenance {
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

func (b *fakeCluster) SetMaintenance(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maintenance = on
}

func (b *fakeCluster) Maintenance() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maintenance
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
		}, "", func() {})
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
