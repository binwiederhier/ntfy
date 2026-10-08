package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
)

const (
	testSecret = "s3cret"
)

// openTestPool opens a dedicated connection pool to the given test schema, so that each simulated
// node has its own pool like real nodes would.
func openTestPool(t testing.TB, dsn string) *db.DB {
	host, err := pg.Open(dsn)
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	return d
}

func newTestMeshConfig(nodeID, advertiseURL string) *Config {
	return &Config{
		Enabled:           true,
		NodeID:            NodeID(nodeID),
		AdvertiseURL:      advertiseURL,
		Secret:            testSecret,
		HeartbeatInterval: 100 * time.Millisecond,
		// The liveness window AND the leadership hold-off. One second is the floor: heartbeats
		// are stored in whole seconds, so a shorter TTL truncates the SQL cutoff to zero and a
		// peer counts as live only inside the wall-clock second it registered in, which makes
		// every test that waits for a peer to appear or leave a coin flip
		NodeTTL:         time.Second,
		MaxMessageBytes: 1 << 20,
	}
}

// registerFakePeer registers a fake peer via the registry (creating the table if the mesh has
// not been constructed yet): tests register fakes before the mesh boots, since its first
// heartbeat caches the peer list. The fake never refreshes its heartbeat.
func registerFakePeer(t testing.TB, pool *db.DB, nodeID NodeID, url string) {
	t.Helper()
	reg, err := registry.New(pool, string(nodeID), url, time.Minute)
	require.Nil(t, err)
	require.Nil(t, reg.Register())
}

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestMesh_CrossNodeDelivery(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var received []*model.Message
	var meshB *meshCluster
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meshB.ServeHTTP(w, r)
	}))
	defer srvB.Close()
	meshB, err := newMeshCluster(newTestMeshConfig("node-b", srvB.URL), poolB, func(m *model.Message) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m)
	})
	require.Nil(t, err)
	defer meshB.Close()
	meshA, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, func(m *model.Message) {
		t.Error("node A must not receive its own relayed message")
	})
	require.Nil(t, err)
	defer meshA.Close()
	msg := model.NewDefaultMessage("mytopic", "hello cross-node")
	require.Nil(t, meshA.ForwardMessage(msg))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "mytopic", received[0].Topic)
	require.Equal(t, "hello cross-node", received[0].Message)
}

func TestMesh_PeerAPI_Auth(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var delivered int
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, func(m *model.Message) {
		delivered++
	})
	require.Nil(t, err)
	defer mesh.Close()
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "hi"))
	require.Nil(t, err)
	payload := assembleMessageBody([][]byte{frag})

	// Wrong secret -> 401, not delivered
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", MessagePath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, "wrong")
	req.Header.Set(originHeader, "node-b")
	mesh.ServeHTTP(rr, req)
	require.Equal(t, 401, rr.Code)

	// Missing secret -> 401, not delivered
	rr = httptest.NewRecorder()
	mesh.ServeHTTP(rr, httptest.NewRequest("POST", MessagePath, strings.NewReader(string(payload))))
	require.Equal(t, 401, rr.Code)
	require.Equal(t, 0, delivered)

	// Missing origin -> 400, not delivered
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", MessagePath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeHTTP(rr, req)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 0, delivered)

	// Correct secret and origin -> 200, delivered
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", MessagePath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, testSecret)
	req.Header.Set(originHeader, "node-b")
	mesh.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, 1, delivered)
}

func TestMesh_PeerAPI_SelfOrigin(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var delivered int
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, func(m *model.Message) {
		delivered++
	})
	require.Nil(t, err)
	defer mesh.Close()

	// A request that carries this node's own broadcasts must not be re-delivered (loop prevention)
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "loop"))
	require.Nil(t, err)
	payload := assembleMessageBody([][]byte{frag})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", MessagePath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, testSecret)
	req.Header.Set(originHeader, "node-a") // Same as the receiving node's ID
	mesh.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, 0, delivered)
}

func TestMesh_SlowPeerIsolation(t *testing.T) {
	// A wedged peer must not delay delivery to healthy peers: each peer has its own queue and
	// delivery worker. With a shared send queue (the design this replaces), the slow peer's
	// requests would occupy all delivery workers and starve the fast peer.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	fastReceived := 0 // Messages, not requests: with batching, one request can carry many
	srvFast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		messages, err := unmarshalMessageBody(body, 1<<20)
		require.Nil(t, err)
		mu.Lock()
		fastReceived += len(messages)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srvFast.Close()
	release := make(chan struct{})
	srvSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // Wedged until the end of the test
		w.WriteHeader(http.StatusOK)
	}))
	defer srvSlow.Close()
	defer close(release)
	// Register the fake peers before the mesh boots; its first heartbeat caches the peer list
	for i, url := range []string{srvFast.URL, srvSlow.URL} {
		registerFakePeer(t, pool, NodeID(fmt.Sprintf("node-fake-%d", i)), url)
	}
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	const n = 20
	for i := 0; i < n; i++ {
		require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))))
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return fastReceived == n
	})
}

func TestMesh_BatchCoalescing(t *testing.T) {
	// Messages published within the linger window arrive as batches: fewer HTTP requests than
	// messages, with nothing lost. Fails against a one-request-per-message sender.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	requests, messages := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		decoded, err := unmarshalMessageBody(body, 1<<20)
		require.Nil(t, err)
		mu.Lock()
		requests++
		messages += len(decoded)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	registerFakePeer(t, pool, "node-fake", srv.URL)
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.BatchLinger = 150 * time.Millisecond
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	const n = 20
	for i := 0; i < n; i++ {
		require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))))
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return messages == n
	})
	mu.Lock()
	defer mu.Unlock()
	require.Less(t, requests, 5, "expected %d messages coalesced into few requests, got %d", n, requests)
}

func TestMesh_DeadPeerRemovedAndRejoin(t *testing.T) {
	// A peer that dies ungracefully (no Deregister) stops refreshing its heartbeat: after the
	// TTL it no longer counts as live (no more sends), its queue/worker are reconciled away, the
	// leader prunes its registry row, and a re-registered peer starts receiving again.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	received := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		messages, err := unmarshalMessageBody(body, 1<<20)
		require.Nil(t, err)
		mu.Lock()
		received += len(messages)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.NodeTTL = time.Second // The floor: a shorter TTL truncates the SQL liveness cutoff to zero
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	// The fake peer registers once and then "dies": its heartbeat is never refreshed
	registerFakePeer(t, pool, "node-dead", srv.URL)
	_, err = mesh.registry.Refresh() // See the peer now; the short TTL would otherwise expire it first
	require.Nil(t, err)
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", "while alive")))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received == 1
	})
	// After the TTL, the peer is no longer live: its queue is reconciled away and its registry
	// row is pruned by the leader (this mesh is the only real node, so it holds the lock)
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.queues) == 0
	})
	waitFor(t, func() bool {
		var count int
		require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = 'node-dead'`).Scan(&count))
		return count == 0
	})
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", "while dead")))
	time.Sleep(250 * time.Millisecond) // Give a wrong implementation time to deliver anyway
	mu.Lock()
	require.Equal(t, 1, received) // Only the first message arrived
	mu.Unlock()
	// The peer comes back (same node ID, fresh heartbeat) and receives messages again; the
	// relay retries because the peer list is cached for up to the node TTL
	registerFakePeer(t, pool, "node-dead", srv.URL)
	waitFor(t, func() bool {
		require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", "after rejoin")))
		mu.Lock()
		defer mu.Unlock()
		return received > 1
	})
}

func TestMesh_ForwardAfterClose(t *testing.T) {
	// A ForwardMessage racing shutdown (e.g. an in-flight publish during server Stop) must not spawn
	// a new peer queue and worker after Close: the worker would never exit (its queue is never
	// closed) and nothing waits for it.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	registerFakePeer(t, pool, "node-peer", "http://127.0.0.1:1")
	require.Nil(t, mesh.Close())
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", "too late"))) // Dropped silently
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	require.Empty(t, mesh.queues)
}

func TestMesh_LeaderFailover(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	meshA, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, nil)
	require.Nil(t, err)
	defer meshA.Close()
	meshB, err := newMeshCluster(newTestMeshConfig("node-b", "http://127.0.0.1:1"), poolB, nil)
	require.Nil(t, err)
	defer meshB.Close()
	// Exactly one node becomes leader
	waitFor(t, func() bool {
		return meshA.IsLeader() != meshB.IsLeader() // Exactly one
	})
	// The leader steps down; the follower takes over
	leader, follower := meshA, meshB
	if meshB.IsLeader() {
		leader, follower = meshB, meshA
	}
	require.Nil(t, leader.Close())
	waitFor(t, follower.IsLeader)
}

func TestMesh_CloseDeregisters(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	var count int
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = 'node-a'`).Scan(&count))
	require.Equal(t, 1, count)
	require.Nil(t, mesh.Close())
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = 'node-a'`).Scan(&count))
	require.Equal(t, 0, count)
}

// postState delivers a state envelope to a mesh's peer API, as a peer would.
func postState(c *meshCluster, origin NodeID, state *apiState) *httptest.ResponseRecorder {
	body, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", StatePath, bytes.NewReader(body))
	req.Header.Set(secretHeader, testSecret)
	req.Header.Set(originHeader, string(origin))
	c.ServeHTTP(rr, req)
	return rr
}

func TestMesh_HealthyReflectsRegistration(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	require.True(t, mesh.Healthy()) // Registered synchronously at construction
	// Stale heartbeat: peers stop forwarding to this node, so it must report unhealthy
	mesh.mu.Lock()
	mesh.lastRegistered = time.Now().Add(-2 * mesh.conf.NodeTTL)
	mesh.mu.Unlock()
	require.False(t, mesh.Healthy())
	// A successful heartbeat restores health
	require.Nil(t, mesh.heartbeat())
	require.True(t, mesh.Healthy())
}

func TestMesh_SubscriberCancelBroadcast(t *testing.T) {
	// A subscriber-cancel broadcast from node A must invoke node B's CancelFunc; node A's own
	// callback must not fire (no loop).
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var receivedB []*SubscriberCancel
	canceledA := 0

	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	confB := newTestMeshConfig("node-b", "http://"+listenerB.Addr().String())
	confB.CancelFunc = func(cancel *SubscriberCancel) {
		mu.Lock()
		defer mu.Unlock()
		receivedB = append(receivedB, cancel)
	}
	meshB, err := newMeshCluster(confB, poolB, nil)
	require.Nil(t, err)
	defer meshB.Close()
	srvB := &http.Server{Handler: meshB}
	go srvB.Serve(listenerB)
	defer srvB.Close()

	confA := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	confA.CancelFunc = func(_ *SubscriberCancel) {
		mu.Lock()
		defer mu.Unlock()
		canceledA++
	}
	meshA, err := newMeshCluster(confA, poolA, nil)
	require.Nil(t, err)
	defer meshA.Close()

	meshA.BroadcastState(&State{SubscriberCancels: []*SubscriberCancel{
		{Topic: "mytopic", ExceptUserID: "u_owner"},
		{Topic: "up*", UserID: "u_revoked"},
	}})
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedB) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "mytopic", receivedB[0].Topic)
	require.Equal(t, "u_owner", receivedB[0].ExceptUserID)
	require.Equal(t, "up*", receivedB[1].Topic)
	require.Equal(t, "u_revoked", receivedB[1].UserID)
	require.Equal(t, 0, canceledA)
}

func TestMesh_ForwardsEveryTopicToEveryPeer(t *testing.T) {
	// Fan-out is unconditional: a peer gets every message and drops the ones it has no
	// subscriber for. Nothing about a peer's subscriptions may gate delivery, which is what
	// routing by subscription knowledge used to do (removed: ordering made it lose messages).
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var topics []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		messages, err := unmarshalMessageBody(body, 1<<20)
		require.Nil(t, err)
		mu.Lock()
		for _, m := range messages {
			topics = append(topics, m.Topic)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	registerFakePeer(t, pool, "node-b", srv.URL)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("some-topic", "one")))
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("other-topic", "two")))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(topics) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	require.ElementsMatch(t, []string{"some-topic", "other-topic"}, topics)
}

func TestMesh_TopicAnnouncementInvokesTopicsAddedFunc(t *testing.T) {
	// A first-subscriber announcement from node A must reach node B's TopicsAddedFunc, which
	// the server uses to drop a stale UnifiedPush rate-visitor miss
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var addedB []string

	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	confB := newTestMeshConfig("node-b", "http://"+listenerB.Addr().String())
	confB.TopicsAddedFunc = func(topics []string) {
		mu.Lock()
		defer mu.Unlock()
		addedB = append(addedB, topics...)
	}
	meshB, err := newMeshCluster(confB, poolB, nil)
	require.Nil(t, err)
	defer meshB.Close()
	srvB := &http.Server{Handler: meshB}
	go srvB.Serve(listenerB)
	defer srvB.Close()

	meshA, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, nil)
	require.Nil(t, err)
	defer meshA.Close()

	meshA.BroadcastState(&State{AddedTopics: []string{"up123456789012"}})
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(addedB) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"up123456789012"}, addedB)
}

func TestMesh_IsolatedFuncWhenPeersHealthy(t *testing.T) {
	// A node that lost its database (registration goes stale) while a peer is still healthy
	// is isolated: peers stop forwarding to it, so its subscribers would silently receive
	// nothing. IsolatedFunc tells the server to close them so clients reconnect elsewhere.
	isolatedTest(t, http.StatusOK, `{"healthy":true}`, true)
}

func TestMesh_NoIsolatedFuncWhenNoPeerHealthy(t *testing.T) {
	// Full database outage: every node is unhealthy, the mesh keeps delivering on its cached
	// peer view, so subscribers must be kept (fail open)
	isolatedTest(t, http.StatusServiceUnavailable, `{"healthy":false}`, false)
}

func TestMesh_NoIsolatedFuncWhenAPeerAnswers200WithoutBeingNtfy(t *testing.T) {
	// A 200 is not evidence that the peer is a healthy ntfy: a misconfigured advertise URL can
	// point at a proxy or an unrelated service that answers 200 for any path. Believing that
	// would close this node's subscribers while nothing else can serve them, which is the one
	// thing the isolation check must not do.
	isolatedTest(t, http.StatusOK, "<html>hello from some proxy</html>", false)
}

func isolatedTest(t *testing.T, peerHealthStatus int, peerHealthBody string, wantIsolated bool) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == HealthPath {
			w.WriteHeader(peerHealthStatus)
			io.WriteString(w, peerHealthBody)
		}
	}))
	defer peer.Close()

	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	pool := db.New(host, nil)
	var isolated atomic.Int32
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.NodeTTL = time.Second // The floor: a shorter TTL truncates the SQL liveness cutoff to zero
	conf.IsolatedFunc = func() { isolated.Add(1) }
	// Registered last: the fake peer never heartbeats again, so its row must still be fresh
	// when the mesh takes its first peer snapshot
	registerFakePeer(t, openTestPool(t, schemaDSN), "node-peer", peer.URL)
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.knownPeers) == 1
	})
	require.Equal(t, int32(0), isolated.Load()) // Healthy node: never isolated

	pool.Close() // Database lost: registration fails from now on
	waitFor(t, func() bool { return !mesh.Healthy() })
	time.Sleep(2 * conf.HeartbeatInterval) // Give the isolation loop its chance to decide
	require.Equal(t, wantIsolated, isolated.Load() > 0)
}

func TestMesh_IsolatedFuncWhenDatabaseHangs(t *testing.T) {
	// A network partition makes database calls hang (dropped packets) instead of failing, which
	// blocks the heartbeat loop. Isolation must still be detected (Healthy is time-based).
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"healthy":true}`) // Answers like a healthy ntfy, which is what the probe requires
	}))
	defer peer.Close()

	proxy := newFreezableProxy(t, schemaDSN)
	var isolated atomic.Int32
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.NodeTTL = time.Second // The floor: a shorter TTL truncates the SQL liveness cutoff to zero
	conf.IsolatedFunc = func() { isolated.Add(1) }
	// Registered last, and through its own pool: the fake peer never heartbeats again, so its
	// row must still be fresh when the mesh takes its first peer snapshot
	registerFakePeer(t, openTestPool(t, schemaDSN), "node-peer", peer.URL)
	mesh, err := newMeshCluster(conf, openTestPool(t, proxy.dsn), nil)
	require.Nil(t, err)
	defer mesh.Close()
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.knownPeers) == 1
	})

	proxy.freeze()
	waitFor(t, func() bool { return isolated.Load() > 0 })
}

// freezableProxy forwards TCP to the test database until frozen; then it stops moving bytes
// (and new connections hang), like a partition that drops packets
type freezableProxy struct {
	dsn    string
	frozen atomic.Bool
}

func newFreezableProxy(t *testing.T, dsn string) *freezableProxy {
	u, err := url.Parse(dsn)
	require.Nil(t, err)
	target := u.Host
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	t.Cleanup(func() { listener.Close() })
	p := &freezableProxy{}
	u.Host = listener.Addr().String()
	p.dsn = u.String()
	pipe := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			for p.frozen.Load() {
				time.Sleep(50 * time.Millisecond) // Hold everything, never close: a black hole
			}
			if err != nil {
				dst.Close()
				return
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
	}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				client.Close()
				continue
			}
			go pipe(server, client)
			go pipe(client, server)
		}
	}()
	return p
}

func (p *freezableProxy) freeze() {
	p.frozen.Store(true)
}

func TestMesh_MembersEndpoint(t *testing.T) {
	// The LB agents ask a node which nodes are live, so each LB can maintain its own upstream
	// list instead of a central monitor pushing it. Secret-authenticated, read-only.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	registerFakePeer(t, pool, "node-peer", "http://192.168.1.50:2587")
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://192.168.1.10:2587"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.knownPeers) == 1
	})

	// Without the shared secret: rejected
	rr := httptest.NewRecorder()
	mesh.ServeHTTP(rr, httptest.NewRequest("GET", MembersPath, nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	// With it: this node plus its live peers, with the addresses an agent needs
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", MembersPath, nil)
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var members []Member
	require.Nil(t, json.Unmarshal(rr.Body.Bytes(), &members))
	byID := make(map[NodeID]Member)
	for _, m := range members {
		byID[m.NodeID] = m
	}
	require.Equal(t, 2, len(byID))
	require.Equal(t, "http://192.168.1.10:2587", byID["node-a"].AdvertiseURL)
	require.True(t, byID["node-a"].Healthy)
	require.Equal(t, "http://192.168.1.50:2587", byID["node-peer"].AdvertiseURL)
}

func TestMesh_UndeliveredBatchIsReportedAsAGap(t *testing.T) {
	// A batch the peer rejects is lost: its subscribers there would sit on an open connection
	// missing a message. The peer must be told which topics, so it can make those clients
	// replay. The report is retried until it lands, since an unreachable peer is the usual
	// reason for a gap in the first place.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var gaps []string
	rejectState := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == MessagePath {
			w.WriteHeader(http.StatusInternalServerError) // Every batch is rejected
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if rejectState { // The first gap report fails too
			rejectState = false
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		var state apiState
		require.Nil(t, json.Unmarshal(body, &state))
		gaps = append(gaps, state.Gaps...)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	registerFakePeer(t, pool, "node-b", srv.URL)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("gapped-topic", "lost")))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(gaps) > 0
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"gapped-topic"}, gaps) // Reported once, after one failed attempt
}

func TestMesh_ReportedGapInvokesGapFunc(t *testing.T) {
	// The receiving side hands the gapped topics to the server, which closes their subscribers
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var received []string
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.GapFunc = func(topics []string, _ int64) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, topics...)
	}
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	rr := postState(mesh, "node-b", &apiState{Gaps: []string{"topic-1", "topic-2"}})
	require.Equal(t, 200, rr.Code)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"topic-1", "topic-2"}, received)
}

func TestMesh_ManyGappedTopicsCollapseToAll(t *testing.T) {
	// A peer that was unreachable for a while can have gaps in more topics than are worth
	// enumerating; past the cap the report degenerates to "every topic"
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	for i := 0; i <= gapMaxTopics; i++ {
		mesh.recordGap("node-b", []string{fmt.Sprintf("topic-%d", i)}, time.Now().Unix())
	}
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	require.Equal(t, []string{GapAllTopics}, mesh.gaps["node-b"].topics)
}

func TestMesh_GapReportCarriesTheOldestLostMessageTime(t *testing.T) {
	// Closing the peer's subscribers only helps if their clients can work out what they missed,
	// and they cannot: a client that received a newer message on the same topic reconnects with
	// that newer marker and never asks for the older hole. The report therefore dates the gap
	// with the oldest message in it, so the peer can replay exactly that range.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	var reported apiState
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == MessagePath {
			w.WriteHeader(http.StatusInternalServerError) // Every batch is rejected
			return
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		var state apiState
		require.Nil(t, json.Unmarshal(body, &state))
		if len(state.Gaps) > 0 {
			mu.Lock()
			reported = state
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	registerFakePeer(t, pool, "node-b", srv.URL)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()

	oldest := model.NewDefaultMessage("gapped-topic", "lost first")
	oldest.Time = time.Now().Add(-30 * time.Second).Unix()
	newest := model.NewDefaultMessage("gapped-topic", "lost second")
	require.Nil(t, mesh.ForwardMessage(newest))
	require.Nil(t, mesh.ForwardMessage(oldest))
	// The two may be reported in one batch or in two, so wait for the oldest to show up: a
	// report that only ever carries the newer time would never satisfy this
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reported.GapSince == oldest.Time
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"gapped-topic"}, reported.Gaps)
}

func TestMesh_GapKeepsTheOldestOfSeveralLostMessages(t *testing.T) {
	// Gaps for one peer accumulate between heartbeats, from failed batches and from messages
	// dropped on a full queue. The report has to date the whole accumulation at its oldest
	// message, or the replay starts after the first hole.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	older := time.Now().Add(-45 * time.Second).Unix()
	newer := time.Now().Unix()

	mesh.recordGap("node-b", []string{"topic-1"}, older)
	mesh.recordGap("node-b", []string{"topic-2"}, newer)
	mesh.recordGap("node-c", []string{"topic-3"}, newer)
	mesh.recordGap("node-c", []string{"topic-4"}, older)

	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	require.Equal(t, older, mesh.gaps["node-b"].since) // Oldest wins, whichever order they arrive in
	require.Equal(t, older, mesh.gaps["node-c"].since)
}

func TestMesh_MembersOfAnIsolatedNodeAreNotReportedHealthy(t *testing.T) {
	// A node that lost the database keeps serving its last peer view, so it answers the LB
	// agents with a membership that may be minutes out of date, and used to mark every one of
	// those peers healthy. An agent that happens to ask this node then pins its upstreams to a
	// frozen list. The answer has to carry how much this node still knows: nothing.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	registerFakePeer(t, pool, "node-peer", "http://192.168.1.50:2587")
	conf := newTestMeshConfig("node-a", "http://192.168.1.10:2587")
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.knownPeers) == 1
	})
	healthyPeers := func() int {
		n := 0
		for _, m := range mesh.Members() {
			if m.NodeID != conf.NodeID && m.Healthy {
				n++
			}
		}
		return n
	}
	require.Equal(t, 1, healthyPeers()) // While the view is fresh, the peer counts

	require.Nil(t, pool.Close()) // The database goes away: heartbeats and reads fail
	waitFor(t, func() bool { return healthyPeers() == 0 })
	for _, m := range mesh.Members() {
		require.False(t, m.Healthy, "an isolated node reported %s as healthy", m.NodeID)
	}
}

func TestMesh_HealthEndpoint(t *testing.T) {
	// Peers probe this to decide whether a node that lost its registration has somewhere better
	// to send its subscribers, so it answers this node's own registration freshness, and it is
	// secret-authenticated like the rest of the peer API
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()

	rr := httptest.NewRecorder()
	mesh.ServeHTTP(rr, httptest.NewRequest("GET", HealthPath, nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code) // No secret: rejected like every peer path

	probe := func() *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", HealthPath, nil)
		req.Header.Set(secretHeader, testSecret)
		mesh.ServeHTTP(rr, req)
		return rr
	}
	rr = probe()
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"healthy":true`)

	// Registration goes stale once the database is gone, and the answer follows it
	require.Nil(t, pool.Close())
	waitFor(t, func() bool { return !mesh.Healthy() })
	rr = probe()
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	require.Contains(t, rr.Body.String(), `"healthy":false`)
}
