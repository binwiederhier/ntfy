package server

import (
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// fakeCluster records relayed messages and topic announcements so tests can assert that every
// publish path passes through the cluster exactly once, and that subscription hooks fire.
type fakeCluster struct {
	mu        sync.Mutex
	messages  []*model.Message
	announced []string
	notLeader bool
}

func (b *fakeCluster) Relay(m *model.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, m)
	return nil
}

func (b *fakeCluster) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func (b *fakeCluster) AnnounceTopics(topics []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.announced = append(b.announced, topics...)
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

func TestServer_Cluster_PublishRelaysOnce(t *testing.T) {
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

func TestServer_Cluster_SyncEventRelays(t *testing.T) {
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
	v := newVisitor(c, s.messageCache, s.userManager, netip.MustParseAddr("1.2.3.4"), nil)
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
