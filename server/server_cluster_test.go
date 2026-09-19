package server

import (
	"database/sql"
	"encoding/json"
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
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
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
	response := request(t, s, "POST", "/v1/internal/message",
		`{"message":{"id":"x1","time":1,"event":"message","topic":"mytopic","message":"sneaky"}}`,
		map[string]string{"X-Cluster-Secret": "s3cret", "X-Cluster-Origin": "node-b"})
	require.Equal(t, 404, response.Code)
	time.Sleep(250 * time.Millisecond) // Delivery is async; give a wrong implementation time to fail
	mu.Lock()
	require.Empty(t, received)
	mu.Unlock()
	// The same request against the cluster listener handler DOES deliver
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/internal/message",
		strings.NewReader(`{"message":{"id":"x2","time":1,"event":"message","topic":"mytopic","message":"legit"}}`))
	require.Nil(t, err)
	req.Header.Set("X-Cluster-Secret", "s3cret")
	req.Header.Set("X-Cluster-Origin", "node-b")
	s.clusterHandler().ServeHTTP(rr, req)
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
	srvB := &http.Server{Handler: sB.clusterHandler()}
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

func TestServer_Cluster_FirstSubscriberAnnounces(t *testing.T) {
	// A topic gaining its FIRST subscriber is announced to peers exactly once, so publishers on
	// other nodes stop skipping this node for it without waiting for the next state push.
	s := newTestServer(t, newTestConfig(t, ""))
	b := &fakeCluster{}
	s.cluster = b
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	subscriber := func(_ *visitor, _ *model.Message) error { return nil }
	topics[0].Subscribe(subscriber, "", func() {})
	waitFor(t, func() bool {
		return len(b.Announced()) == 1 && b.Announced()[0] == "mytopic"
	})
	// A second subscriber does not re-announce
	topics[0].Subscribe(subscriber, "", func() {})
	time.Sleep(250 * time.Millisecond)
	require.Len(t, b.Announced(), 1)
}

func TestServer_Cluster_ManagerPrunesOnlyOnLeader(t *testing.T) {
	c := newTestConfig(t, "")
	s := newTestServer(t, c)
	cl := &fakeCluster{notLeader: true}
	s.cluster = cl

	// Publish and expire a message
	rr := request(t, s, "POST", "/mytopic", "hi", nil)
	require.Equal(t, 200, rr.Code)
	m := toMessage(t, rr.Body.String())
	require.Nil(t, s.messageCache.ExpireMessages("mytopic"))

	// A non-leader node leaves shared-database pruning to the leader
	s.execManager()
	_, err := s.messageCache.Message(m.ID)
	require.Nil(t, err)

	// Once this node is the leader, the same run prunes
	cl.setLeader(true)
	s.execManager()
	_, err = s.messageCache.Message(m.ID)
	require.Equal(t, model.ErrMessageNotFound, err)
}

func TestServer_Cluster_StatsResetOnlyOnLeader(t *testing.T) {
	c := newTestConfigWithAuthFile(t, "")
	s := newTestServer(t, c)
	cl := &fakeCluster{notLeader: true}
	s.cluster = cl

	// An anonymous visitor with an in-memory message count
	v := newVisitor(c, s.messageCache, s.userManager, s.quota, netip.MustParseAddr("1.2.3.4"), nil)
	require.True(t, v.MessageAllowed())
	s.mu.Lock()
	s.visitors["ip:1.2.3.4"] = v
	s.mu.Unlock()
	require.Equal(t, int64(1), v.Stats().Messages)

	// A user with persisted stats in the (shared) user database
	require.Nil(t, s.userManager.AddUser("phil", "phil1234", user.RoleUser, false))
	authDB, err := sql.Open("sqlite3", c.AuthFile)
	require.Nil(t, err)
	defer authDB.Close()
	_, err = authDB.Exec(`UPDATE user SET stats_messages = 5 WHERE user = 'phil'`)
	require.Nil(t, err)

	// A non-leader node resets its own in-memory visitor stats, but leaves the user database
	// to the leader
	s.resetStats()
	require.Equal(t, int64(0), v.Stats().Messages)
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(5), u.Stats.Messages)

	// The leader resets the user database too
	cl.setLeader(true)
	s.resetStats()
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(0), u.Stats.Messages)
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
	// The cluster listener's health endpoint reflects the same state
	rr2 := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/v1/health", nil)
	require.Nil(t, err)
	s.clusterHandler().ServeHTTP(rr2, req)
	require.Equal(t, 503, rr2.Code)
}

func TestServer_Cluster_MessageQuotaEnforcedAcrossNodes(t *testing.T) {
	// Two nodes sharing one Postgres schema: the daily message quota is cluster-wide, so a
	// visitor spreading publishes across nodes must not get N times the limit. Enforcement is
	// eventually consistent (usage flush interval), so the test waits between phases.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = "127.0.0.1:1" // Enables clustering; fan-out target never actually called
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		conf.VisitorUsageFlushInterval = 150 * time.Millisecond
		conf.VisitorMessageDailyLimit = 4
		return newTestServer(t, conf)
	}
	sA, sB := newNode("node-a"), newNode("node-b")

	// Two publishes on each node, all within the limit of 4; generous sleeps between phases
	// let each node push its own usage and pull the other's
	for _, s := range []*Server{sA, sB} {
		for i := 0; i < 2; i++ {
			response := request(t, s, "PUT", "/mytopic", "test", nil)
			require.Equal(t, 200, response.Code)
		}
		time.Sleep(600 * time.Millisecond)
	}

	// The cluster-wide quota (4) is exhausted: a single further publish on either node must be
	// rejected. Each node locally only counted 2 of the 4 messages, so a 429 here can only come
	// from cluster-wide enforcement, not from the local limiter.
	require.Equal(t, 429, request(t, sB, "PUT", "/mytopic", "test", nil).Code)
	require.Equal(t, 429, request(t, sA, "PUT", "/mytopic", "test", nil).Code)
}

func TestServer_Cluster_RequestLimitBurnsAcrossNodes(t *testing.T) {
	// The request token bucket stays per-node (algorithm unchanged), but nodes burn tokens for
	// usage their peers report, so a visitor rotating across N nodes gets roughly one bucket,
	// not N of them.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = "127.0.0.1:1" // Enables clustering; fan-out target never actually called
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		conf.VisitorUsageFlushInterval = 150 * time.Millisecond
		conf.VisitorRequestLimitBurst = 10
		conf.VisitorRequestLimitReplenish = time.Minute // Effectively no replenishment during the test
		return newTestServer(t, conf)
	}
	sA, sB := newNode("node-a"), newNode("node-b")

	// Consume 6 of the 10 tokens on node A, let the usage propagate to node B
	for i := 0; i < 6; i++ {
		require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "test", nil).Code)
	}
	time.Sleep(600 * time.Millisecond)

	// Node B must have burned A's 6 tokens from its own bucket: 4 requests left, the 5th fails.
	// Without the burn-down, B would happily serve 10.
	for i := 0; i < 4; i++ {
		require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "test", nil).Code, "request %d on node B", i+1)
	}
	require.Equal(t, 429, request(t, sB, "PUT", "/mytopic", "test", nil).Code)
}

func TestServer_Cluster_UnifiedPushRateVisitorAcrossNodes(t *testing.T) {
	// With subscriber-based rate limiting, a UnifiedPush publish requires a prior subscriber
	// (else 507) and is billed to the subscriber, not the publisher. In-memory that only works
	// when both hit the same node; the shared assignment store must bridge nodes: subscribe on
	// node B, publish on node A.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = "127.0.0.1:1" // Enables clustering; fan-out target never actually called
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		conf.VisitorUsageFlushInterval = 150 * time.Millisecond
		conf.VisitorSubscriberRateLimiting = true
		return newTestServer(t, conf)
	}
	sA, sB := newNode("node-a"), newNode("node-b")

	// Publishing without any subscriber must return 507 (checked on node B; the failed lookup
	// is cached per topic for a while, so node A must not probe before the subscribe)
	require.Equal(t, 507, request(t, sB, "PUT", "/up123456789012?up=1", "test", nil).Code)

	// Subscribe on node B (default test IP 9.9.9.9 becomes the rate visitor)
	require.Equal(t, 200, request(t, sB, "GET", "/up123456789012/json?poll=1", "", nil).Code)

	// Publish on node A from a DIFFERENT IP: must succeed (no 507) and be billed to the
	// subscriber's visitor (9.9.9.9), not the publisher's (8.8.8.8)
	response := request(t, sA, "PUT", "/up123456789012?up=1", "test", nil, func(r *http.Request) {
		r.RemoteAddr = "8.8.8.8:1234"
	})
	require.Equal(t, 200, response.Code)
	require.Equal(t, int64(1), sA.quota.Totals("ip:9.9.9.9").Messages)
	require.Equal(t, int64(0), sA.quota.Totals("ip:8.8.8.8").Messages)
}

func TestServer_Cluster_ReservationTakeoverCancelsAcrossNodes(t *testing.T) {
	// Reserving a topic kicks everyone else's live subscribers. That must reach subscribers
	// connected to OTHER nodes, or they keep receiving messages they just lost access to.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	// Node B with a real cluster listener (receives the cancel broadcast)
	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	confB := newTestConfig(t, schemaDSN)
	confB.ClusterNodeID = "node-b"
	confB.ClusterListen = listenerB.Addr().String()
	confB.ClusterSecret = "s3cret"
	confB.ClusterAdvertiseURL = "http://" + listenerB.Addr().String()
	confB.AuthDefault = user.PermissionReadWrite
	sB := newTestServer(t, confB)
	srvB := &http.Server{Handler: sB.clusterHandler()}
	go srvB.Serve(listenerB)
	defer srvB.Close()
	// Node A takes the reservation
	confA := newTestConfig(t, schemaDSN)
	confA.ClusterNodeID = "node-a"
	confA.ClusterListen = "127.0.0.1:1"
	confA.ClusterSecret = "s3cret"
	confA.ClusterAdvertiseURL = "http://127.0.0.1:1"
	confA.AuthDefault = user.PermissionReadWrite
	confA.EnableReservations = true
	sA := newTestServer(t, confA)

	require.Nil(t, sA.userManager.AddTier(&user.Tier{Code: "pro", MessageLimit: 100, ReservationLimit: 2}))
	require.Nil(t, sA.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, sA.userManager.ChangeTier("phil", "pro"))

	// Anonymous subscriber on node B
	topics, err := sB.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	canceled := make(chan bool, 1)
	topics[0].Subscribe(func(_ *visitor, _ *model.Message) error { return nil }, "", func() {
		canceled <- true
	})

	// phil reserves the topic on node A: node B's anonymous subscriber must be canceled
	response := request(t, sA, "POST", "/v1/account/reservation", `{"topic":"mytopic","everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber on node B was not canceled after reservation takeover on node A")
	}
}

func TestServer_Cluster_AccountStatsShowClusterTotals(t *testing.T) {
	// The account usage display must show cluster-wide usage, not the node-local limiter
	// view: behind a load balancer, two page loads would otherwise show different numbers
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(nodeID string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = nodeID
		conf.ClusterListen = "127.0.0.1:1"
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		conf.VisitorUsageFlushInterval = 150 * time.Millisecond
		return newTestServer(t, conf)
	}
	sA, sB := newNode("node-a"), newNode("node-b")

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "one", nil).Code)
	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "two", nil).Code)
	time.Sleep(600 * time.Millisecond) // Push on A, pull on B

	response := request(t, sB, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	var account apiAccountResponse
	require.Nil(t, json.Unmarshal(response.Body.Bytes(), &account))
	require.Equal(t, int64(2), account.Stats.Messages)
}

func TestServer_Cluster_MatrixRejectDecisionFromSharedState(t *testing.T) {
	// The Matrix pushkey rejection must be decided from the shared topic table when
	// clustered: the per-node topic object is often freshly created (so its in-memory age
	// says "too early"), while the shared record proves nobody has subscribed for days
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	conf.BaseURL = "http://127.0.0.1:12345"
	conf.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, conf)

	// A topic that existed for days with no subscriber activity (written by "another node")
	old := time.Now().Add(-80 * time.Hour).Unix()
	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer host.DB.Close()
	_, err = host.DB.Exec(`INSERT INTO topic (id, created_at, last_subscribed_at, last_published_at) VALUES ('up123456789012', $1, $1, $1)`, old)
	require.Nil(t, err)

	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/up123456789012?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, `{"rejected":["http://127.0.0.1:12345/up123456789012?up=1"]}`+"\n", response.Body.String())

	// A topic with RECENT subscriber activity (per the shared record) must NOT be rejected,
	// even though there is no rate visitor right now
	recent := time.Now().Add(-1 * time.Hour).Unix()
	_, err = host.DB.Exec(`INSERT INTO topic (id, created_at, last_subscribed_at) VALUES ('up999456789012', $1, $1)`, recent)
	require.Nil(t, err)
	notification2 := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/up999456789012?up=1"}]}}`
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification2, nil)
	require.Equal(t, 507, response.Code, response.Body.String())
}

func TestServer_Cluster_TopicLivenessRecorded(t *testing.T) {
	// Subscribes (and their keepalives) and successful publishes must be recorded in the
	// shared topic table, so any node can make liveness decisions about a topic
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	conf.KeepaliveInterval = 200 * time.Millisecond
	conf.TopicStoreUpdateInterval = 100 * time.Millisecond
	s := newTestServer(t, conf)

	// Publish: recorded with the publisher's visitor key
	require.Equal(t, 200, request(t, s, "PUT", "/livetopic", "hi", nil).Code)
	waitFor(t, func() bool {
		info, err := s.topicStore.Get("livetopic")
		return err == nil && info.LastPublisherKey == "ip:9.9.9.9" && !info.LastPublishedAt.IsZero()
	})

	// Subscribe: initial record plus keepalive refreshes
	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/livetopic/json", subscribeRR)
	waitFor(t, func() bool {
		info, err := s.topicStore.Get("livetopic")
		return err == nil && !info.LastSubscribedAt.IsZero()
	})
	info1, err := s.topicStore.Get("livetopic")
	require.Nil(t, err)
	// Keepalives must refresh the subscriber record; timestamps have second granularity, so
	// wait until the recorded time visibly advances
	waitForWithMaxWait(t, 10*time.Second, func() bool {
		info2, err := s.topicStore.Get("livetopic")
		return err == nil && info2.LastSubscribedAt.After(info1.LastSubscribedAt)
	})
	subscribeCancel()
}

func TestServer_Cluster_TopicLastAccessSeededAtBoot(t *testing.T) {
	// A restart restores every cached topic into memory. Without seeding, all of them look
	// freshly accessed and linger for topicExpungeAfter; seeded from the shared record, topics
	// idle cluster-wide are expunged at the first manager run, active ones are kept.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(id string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = id
		conf.ClusterListen = "127.0.0.1:1"
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		return newTestServer(t, conf)
	}
	s1 := newNode("node-a")
	require.Equal(t, 200, request(t, s1, "PUT", "/idletopic", "hi", nil).Code)
	require.Equal(t, 200, request(t, s1, "PUT", "/hottopic", "hi", nil).Code)
	require.Equal(t, 200, request(t, s1, "PUT", "/unrecordedtopic", "hi", nil).Code)
	waitFor(t, func() bool {
		for _, id := range []string{"idletopic", "hottopic", "unrecordedtopic"} {
			if _, err := s1.topicStore.Get(id); err != nil {
				return false
			}
		}
		return true
	})

	// Rewrite history: idletopic was last touched 20h ago, hottopic 1h ago, and
	// unrecordedtopic has no shared record at all
	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer host.DB.Close()
	set := func(topic string, at time.Time) {
		_, err := host.DB.Exec(`UPDATE topic SET created_at = $2, last_published_at = $2 WHERE id = $1`, topic, at.Unix())
		require.Nil(t, err)
	}
	set("idletopic", time.Now().Add(-20*time.Hour))
	set("hottopic", time.Now().Add(-time.Hour))
	_, err = host.DB.Exec(`DELETE FROM topic WHERE id = 'unrecordedtopic'`)
	require.Nil(t, err)

	// "Restart": a fresh node boots with all three topics from the message cache
	s2 := newNode("node-b")
	s2.execManager()
	topics := topicsSnapshot(s2)
	require.NotContains(t, topics, "idletopic")
	require.Contains(t, topics, "hottopic")
	require.Contains(t, topics, "unrecordedtopic") // No shared record: local rule (just booted)
}

func TestServer_Cluster_TopicExpungeGatedBySharedActivity(t *testing.T) {
	// A topic idle on this node must survive expunge while another node records activity
	// for it in the shared table; its local last access adopts the shared one
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	s := newTestServer(t, conf)

	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer host.DB.Close()
	sharedRecent := time.Now().Add(-time.Hour).Truncate(time.Second)
	_, err = host.DB.Exec(`INSERT INTO topic (id, created_at, last_published_at) VALUES ('elsewhere', $1, $1), ('deadtopic', $2, $2)`,
		sharedRecent.Unix(), time.Now().Add(-20*time.Hour).Unix())
	require.Nil(t, err)

	locallyIdle := time.Now().Add(-20 * time.Hour)
	for _, id := range []string{"elsewhere", "deadtopic", "unrecordedtopic"} {
		topic, err := s.topicFromID(nil, id)
		require.Nil(t, err)
		topic.mu.Lock()
		topic.lastAccess = locallyIdle
		topic.mu.Unlock()
	}

	s.execManager()
	topics := topicsSnapshot(s)
	require.Contains(t, topics, "elsewhere")
	require.NotContains(t, topics, "deadtopic")
	require.NotContains(t, topics, "unrecordedtopic")
	require.Equal(t, sharedRecent.Unix(), topics["elsewhere"].LastAccess().Unix())
}

// topicsSnapshot copies the server's topic map, so assertions never run under s.mu (a failing
// require under the lock would deadlock the server's cleanup)
func topicsSnapshot(s *Server) map[string]*topic {
	s.mu.RLock()
	defer s.mu.RUnlock()
	topics := make(map[string]*topic, len(s.topics))
	for id, t := range s.topics {
		topics[id] = t
	}
	return topics
}

func TestServer_Cluster_MessageStatsDoNotCompound(t *testing.T) {
	// Each node writes only its own new publishes as a delta. A node must never write back the
	// peer counts it folded in: on the harness the shared counter compounded every manager tick
	// until it overflowed bigint.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func(id string) *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = id
		conf.ClusterListen = "127.0.0.1:1"
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		return newTestServer(t, conf)
	}
	sA, sB := newNode("node-a"), newNode("node-b")
	for i := 0; i < 3; i++ {
		require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "a", nil).Code)
	}
	for i := 0; i < 2; i++ {
		require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "b", nil).Code)
	}
	for tick := 0; tick < 4; tick++ {
		sA.updateAndWriteStats(0)
		sB.updateAndWriteStats(0)
	}
	total, err := sA.messageCache.Stats()
	require.Nil(t, err)
	require.Equal(t, int64(5), total)
	sA.mu.RLock()
	defer sA.mu.RUnlock()
	require.Equal(t, int64(5), sA.messages)
}

func TestServer_Cluster_MessageStatsNotReaddedAfterRestart(t *testing.T) {
	// A (re)started node loads the shared total; its first flush must not write that total
	// back as a "delta" (the flush marker has to start at the loaded value)
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	newNode := func() *Server {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = "node-a"
		conf.ClusterListen = "127.0.0.1:1"
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
		return newTestServer(t, conf)
	}
	s1 := newNode()
	for i := 0; i < 3; i++ {
		require.Equal(t, 200, request(t, s1, "PUT", "/mytopic", "hi", nil).Code)
	}
	s1.updateAndWriteStats(0)
	s1.Stop()

	s2 := newNode()
	s2.updateAndWriteStats(0)
	s2.updateAndWriteStats(0)
	total, err := s2.messageCache.Stats()
	require.Nil(t, err)
	require.Equal(t, int64(3), total)
}
