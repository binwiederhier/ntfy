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
		Enabled:             true,
		NodeID:              NodeID(nodeID),
		AdvertiseURL:        advertiseURL,
		Secret:              testSecret,
		HeartbeatInterval:   100 * time.Millisecond,
		LeaderRenewInterval: 20 * time.Millisecond, // Lease duration 60ms, hold-off 120ms; keeps leadership tests fast
		NodeTTL:             time.Second,           // Also the peer cache bound; short so fake peers registered mid-test are seen quickly
		MaxMessageBytes:     1 << 20,
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
	payload := assembleMessageBody([]*fragment{{topic: "mytopic", data: frag}})

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
	payload := assembleMessageBody([]*fragment{{topic: "mytopic", data: frag}})
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
	conf.NodeTTL = 300 * time.Millisecond // Fast expiry so the test observes TTL-based removal
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
	isolatedTest(t, http.StatusOK, true)
}

func TestMesh_NoIsolatedFuncWhenNoPeerHealthy(t *testing.T) {
	// Full database outage: every node is unhealthy, the mesh keeps delivering on its cached
	// peer view, so subscribers must be kept (fail open)
	isolatedTest(t, http.StatusServiceUnavailable, false)
}

func isolatedTest(t *testing.T, peerHealthStatus int, wantIsolated bool) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			w.WriteHeader(peerHealthStatus)
		}
	}))
	defer peer.Close()
	registerFakePeer(t, openTestPool(t, schemaDSN), "node-peer", peer.URL)

	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	pool := db.New(host, nil)
	var isolated atomic.Int32
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.NodeTTL = 300 * time.Millisecond
	conf.IsolatedFunc = func() { isolated.Add(1) }
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
	time.Sleep(time.Second)
	require.False(t, mesh.Healthy())
	require.Equal(t, wantIsolated, isolated.Load() > 0)
}

func TestMesh_IsolatedFuncWhenDatabaseHangs(t *testing.T) {
	// A network partition makes database calls hang (dropped packets) instead of failing, which
	// blocks the heartbeat loop. Isolation must still be detected (Healthy is time-based).
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer peer.Close()
	registerFakePeer(t, openTestPool(t, schemaDSN), "node-peer", peer.URL)

	proxy := newFreezableProxy(t, schemaDSN)
	var isolated atomic.Int32
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.NodeTTL = 300 * time.Millisecond
	conf.IsolatedFunc = func() { isolated.Add(1) }
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
	conf.GapFunc = func(topics []string) {
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
		mesh.recordGap("node-b", []string{fmt.Sprintf("topic-%d", i)})
	}
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	require.Equal(t, []string{GapAllTopics}, mesh.gaps["node-b"])
}
