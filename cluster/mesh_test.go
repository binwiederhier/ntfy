package cluster

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
)

const testSecret = "s3cret"

// openTestPool opens a dedicated connection pool to the given test schema, so that each simulated
// node has its own pool like real nodes would.
func openTestPool(t *testing.T, dsn string) *db.DB {
	host, err := pg.Open(dsn)
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	return d
}

func newTestMeshConfig(nodeID, advertiseURL string) Config {
	return Config{
		Enabled:           true,
		NodeID:            nodeID,
		AdvertiseURL:      advertiseURL,
		Secret:            testSecret,
		HeartbeatInterval: 100 * time.Millisecond,
		NodeTTL:           5 * time.Second,
		MaxMessageBytes:   1 << 20,
	}
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
	var meshB *Mesh
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meshB.ServeFanout(w, r)
	}))
	defer srvB.Close()
	meshB, err := newMesh(newTestMeshConfig("node-b", srvB.URL), poolB, func(m *model.Message) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m)
	})
	require.Nil(t, err)
	defer meshB.Close()
	meshA, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, func(m *model.Message) {
		t.Error("node A must not receive its own broadcast")
	})
	require.Nil(t, err)
	defer meshA.Close()
	msg := model.NewDefaultMessage("mytopic", "hello cross-node")
	require.Nil(t, meshA.Broadcast(msg))
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

func TestMesh_ServeFanout_Auth(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var delivered int
	mesh, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, func(m *model.Message) {
		delivered++
	})
	require.Nil(t, err)
	defer mesh.Close()
	payload, err := marshalEnvelope("node-b", model.NewDefaultMessage("mytopic", "hi"))
	require.Nil(t, err)

	// Wrong secret -> 401, not delivered
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", FanoutPath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, "wrong")
	mesh.ServeFanout(rr, req)
	require.Equal(t, 401, rr.Code)

	// Missing secret -> 401, not delivered
	rr = httptest.NewRecorder()
	mesh.ServeFanout(rr, httptest.NewRequest("POST", FanoutPath, strings.NewReader(string(payload))))
	require.Equal(t, 401, rr.Code)
	require.Equal(t, 0, delivered)

	// Correct secret -> 200, delivered
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", FanoutPath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeFanout(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, 1, delivered)
}

func TestMesh_ServeFanout_SelfOriginAndUnknownKind(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var delivered int
	mesh, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, func(m *model.Message) {
		delivered++
	})
	require.Nil(t, err)
	defer mesh.Close()

	// An envelope that originated on this node must not be re-delivered (loop prevention)
	payload, err := marshalEnvelope("node-a", model.NewDefaultMessage("mytopic", "loop"))
	require.Nil(t, err)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", FanoutPath, strings.NewReader(string(payload)))
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeFanout(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, 0, delivered)

	// Unknown envelope kinds are ignored with a 200 (mixed-version rolling deploys)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", FanoutPath, strings.NewReader(`{"kind":"bloom-gossip","origin":"node-b"}`))
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeFanout(rr, req)
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
	fastReceived := 0
	srvFast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fastReceived++
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
	mesh, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	// Register the fake peers directly in the registry; they are just rows with fresh heartbeats
	for i, url := range []string{srvFast.URL, srvSlow.URL} {
		_, err := pool.Exec(upsertNodeQuery, fmt.Sprintf("node-fake-%d", i), url, time.Now().Unix())
		require.Nil(t, err)
	}
	const n = 20
	for i := 0; i < n; i++ {
		require.Nil(t, mesh.Broadcast(model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))))
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return fastReceived == n
	})
}

func TestMesh_LeaderFailover(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	meshA, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), poolA, nil)
	require.Nil(t, err)
	defer meshA.Close()
	meshB, err := newMesh(newTestMeshConfig("node-b", "http://127.0.0.1:1"), poolB, nil)
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

func TestRegistry_ConcurrentCreate(t *testing.T) {
	// Multiple nodes cold-booting on a fresh database must not race on table creation: CREATE
	// TABLE IF NOT EXISTS is not atomic in PostgreSQL, so creation is serialized via an advisory
	// lock. Without it, this test fails sporadically with a duplicate-key error on pg_class.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			pool, err := pg.Open(schemaDSN)
			if err != nil {
				errs <- err
				return
			}
			defer pool.DB.Close()
			_, err = newRegistry(db.New(pool, nil), fmt.Sprintf("node-%d", i), "http://127.0.0.1:1", time.Second)
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		require.Nil(t, <-errs)
	}
}

func TestMesh_CloseDeregisters(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	mesh, err := newMesh(newTestMeshConfig("node-a", "http://127.0.0.1:1"), pool, nil)
	require.Nil(t, err)
	var count int
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = 'node-a'`).Scan(&count))
	require.Equal(t, 1, count)
	require.Nil(t, mesh.Close())
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = 'node-a'`).Scan(&count))
	require.Equal(t, 0, count)
}
