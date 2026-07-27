package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const testSecret = "s3cret"

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
		NodeTTL:           time.Second, // Also the peer cache bound; short so fake peers registered mid-test are seen quickly
		MaxMessageBytes:   1 << 20,
		StateInterval:     time.Minute, // Individual tests lower this to exercise state pushes
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
	}, nil)
	require.Nil(t, err)
	defer meshB.Close()
	meshA, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, func(m *model.Message) {
		t.Error("node A must not receive its own relayed message")
	}, nil)
	require.Nil(t, err)
	defer meshA.Close()
	msg := model.NewDefaultMessage("mytopic", "hello cross-node")
	require.Nil(t, meshA.Relay(msg))
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
	}, nil)
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
	}, nil)
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
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	const n = 20
	for i := 0; i < n; i++ {
		require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))))
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
	mesh, err := newMeshCluster(conf, pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	const n = 20
	for i := 0; i < n; i++ {
		require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))))
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
	mesh, err := newMeshCluster(conf, pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	// The fake peer registers once and then "dies": its heartbeat is never refreshed
	registerFakePeer(t, pool, "node-dead", srv.URL)
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", "while alive")))
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
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", "while dead")))
	time.Sleep(250 * time.Millisecond) // Give a wrong implementation time to deliver anyway
	mu.Lock()
	require.Equal(t, 1, received) // Only the first message arrived
	mu.Unlock()
	// The peer comes back (same node ID, fresh heartbeat) and receives messages again; the
	// relay retries because the peer list is cached for up to the node TTL
	registerFakePeer(t, pool, "node-dead", srv.URL)
	waitFor(t, func() bool {
		require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", "after rejoin")))
		mu.Lock()
		defer mu.Unlock()
		return received > 1
	})
}

func TestMesh_RelayAfterClose(t *testing.T) {
	// A Relay racing shutdown (e.g. an in-flight publish during server Stop) must not spawn
	// a new peer queue and worker after Close: the worker would never exit (its queue is never
	// closed) and nothing waits for it.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
	require.Nil(t, err)
	registerFakePeer(t, pool, "node-peer", "http://127.0.0.1:1")
	require.Nil(t, mesh.Close())
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("mytopic", "too late"))) // Dropped silently
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	require.Empty(t, mesh.queues)
}

func TestMesh_LeaderFailover(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	meshA, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, nil, nil)
	require.Nil(t, err)
	defer meshA.Close()
	meshB, err := newMeshCluster(newTestMeshConfig("node-b", "http://127.0.0.1:1"), poolB, nil, nil)
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
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
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

// topicFilter builds a marshaled Bloom filter over the given topics.
func topicFilter(t *testing.T, topics ...string) []byte {
	t.Helper()
	filter := util.NewBloomFilter(len(topics), 0.01)
	for _, topic := range topics {
		filter.Add(topic)
	}
	data, err := filter.MarshalBinary()
	require.Nil(t, err)
	return data
}

func TestMesh_RouteSkipsUnsubscribedPeer(t *testing.T) {
	// A peer whose fresh state provably excludes a topic is not contacted for it; a topic in its
	// state is delivered as usual.
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
	registerFakePeer(t, pool, "node-b", srv.URL)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	// node-b reports subscribers only for "subscribed-topic"
	rr := postState(mesh, "node-b", &apiState{Topics: &apiStateTopics{Filter: topicFilter(t, "subscribed-topic")}})
	require.Equal(t, 200, rr.Code)
	// A topic outside the peer's state is skipped
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("other-topic", "skipped")))
	time.Sleep(300 * time.Millisecond) // Give a wrong implementation time to deliver anyway
	mu.Lock()
	require.Equal(t, 0, received)
	mu.Unlock()
	// A topic inside the peer's state is delivered
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("subscribed-topic", "delivered")))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received == 1
	})
}

func TestMesh_RouteBroadcastsOnStaleState(t *testing.T) {
	// State too old to trust cannot justify skipping: the peer is broadcast to as if unknown.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	received := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == MessagePath { // The mesh also pushes state here; count only messages
			mu.Lock()
			received++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	registerFakePeer(t, pool, "node-b", srv.URL)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	rr := postState(mesh, "node-b", &apiState{Topics: &apiStateTopics{Filter: topicFilter(t, "subscribed-topic")}})
	require.Equal(t, 200, rr.Code)
	// Age the state beyond the trust window
	mesh.statesMu.Lock()
	mesh.states["node-b"].updatedAt = time.Now().Add(-time.Hour)
	mesh.statesMu.Unlock()
	require.Nil(t, mesh.Relay(model.NewDefaultMessage("other-topic", "broadcast anyway")))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received == 1
	})
}

func TestMesh_StatePushReplacesAndRemoves(t *testing.T) {
	// Node A periodically pushes a full snapshot of its live topics to node B; each snapshot
	// REPLACES B's knowledge, so topics that lost their subscribers disappear without any
	// explicit removal protocol.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	var topicsMu sync.Mutex
	topicsA := []string{"topic-1"}
	source := func() []string {
		topicsMu.Lock()
		defer topicsMu.Unlock()
		return append([]string{}, topicsA...)
	}
	var meshB *meshCluster
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meshB.ServeHTTP(w, r)
	}))
	defer srvB.Close()
	meshB, err := newMeshCluster(newTestMeshConfig("node-b", srvB.URL), poolB, nil, nil)
	require.Nil(t, err)
	defer meshB.Close()
	confA := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	confA.StateInterval = 200 * time.Millisecond
	meshA, err := newMeshCluster(confA, poolA, nil, source)
	require.Nil(t, err)
	defer meshA.Close()
	// B learns A's topics via the periodic push
	knows := func(topic string) func() bool {
		return func() bool {
			meshB.statesMu.Lock()
			defer meshB.statesMu.Unlock()
			state, ok := meshB.states["node-a"]
			return ok && state.topics.Contains(topic)
		}
	}
	waitFor(t, knows("topic-1"))
	// A's subscribers change; the next snapshot replaces the old knowledge entirely
	topicsMu.Lock()
	topicsA = []string{"topic-2"}
	topicsMu.Unlock()
	waitFor(t, knows("topic-2"))
	waitFor(t, func() bool { return !knows("topic-1")() })
}

func TestMesh_AnnounceClosesWindow(t *testing.T) {
	// A topic gaining its first subscriber is announced immediately, so peers learn about it
	// without waiting for the next full state push.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	var meshB *meshCluster
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meshB.ServeHTTP(w, r)
	}))
	defer srvB.Close()
	meshB, err := newMeshCluster(newTestMeshConfig("node-b", srvB.URL), poolB, nil, nil)
	require.Nil(t, err)
	defer meshB.Close()
	confA := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	confA.StateInterval = 200 * time.Millisecond // One full push establishes the baseline
	meshA, err := newMeshCluster(confA, poolA, nil, func() []string { return []string{"existing"} })
	require.Nil(t, err)
	defer meshA.Close()
	waitFor(t, func() bool {
		meshB.statesMu.Lock()
		defer meshB.statesMu.Unlock()
		_, ok := meshB.states["node-a"]
		return ok
	})
	// Announcements merge into the baseline right away
	meshA.AnnounceTopics([]string{"fresh-topic"})
	waitFor(t, func() bool {
		meshB.statesMu.Lock()
		defer meshB.statesMu.Unlock()
		state, ok := meshB.states["node-a"]
		return ok && state.topics.Contains("fresh-topic")
	})
}

func TestMesh_StateOfDepartedPeerPruned(t *testing.T) {
	// peerState is push-driven and can arrive before the peer is visible in the registry, so it
	// must survive reconcile while fresh -- but a departed peer's state must not leak forever:
	// once it is both absent from the registry and stale past the trust window, it is pruned.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil, nil)
	require.Nil(t, err)
	defer mesh.Close()
	rr := postState(mesh, "node-gone", &apiState{Topics: &apiStateTopics{Filter: topicFilter(t, "some-topic")}})
	require.Equal(t, 200, rr.Code)
	// Fresh state of an unknown peer survives reconcile (the new-node visibility window)
	mesh.reconcilePeers(nil)
	mesh.statesMu.Lock()
	_, ok := mesh.states["node-gone"]
	mesh.statesMu.Unlock()
	require.True(t, ok)
	// Stale state of an absent peer is pruned
	mesh.statesMu.Lock()
	mesh.states["node-gone"].updatedAt = time.Now().Add(-time.Hour)
	mesh.statesMu.Unlock()
	mesh.reconcilePeers(nil)
	mesh.statesMu.Lock()
	_, ok = mesh.states["node-gone"]
	mesh.statesMu.Unlock()
	require.False(t, ok)
}
