package cluster

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

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

func TestMesh_MembersEndpointReportsRoles(t *testing.T) {
	// LB agents route client traffic to active nodes only, so every member carries its role,
	// this node's from its config and each peer's from the registry
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	registerFakePeerWithRole(t, pool, "node-active", "http://192.168.1.50:2587", RoleActive)
	registerFakePeerWithRole(t, pool, "node-passive", "http://192.168.1.51:2587", RolePassive)
	conf := newTestMeshConfig("node-a", "http://192.168.1.10:2587")
	conf.Role = RolePassive
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	waitFor(t, func() bool {
		mesh.mu.Lock()
		defer mesh.mu.Unlock()
		return len(mesh.knownPeers) == 2
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", MembersPath, nil)
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var members []map[string]any // Raw JSON, since the field name is the contract with the agents
	require.Nil(t, json.Unmarshal(rr.Body.Bytes(), &members))
	roles := make(map[string]any)
	for _, m := range members {
		roles[m["node_id"].(string)] = m["role"]
	}
	require.Equal(t, map[string]any{"node-a": "passive", "node-active": "active", "node-passive": "passive"}, roles)
}

func TestMesh_Status_RequiresSecret(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:1"), openTestPool(t, schemaDSN), func(m *model.Message) {})
	require.Nil(t, err)
	defer mesh.Close()

	rr := httptest.NewRecorder()
	mesh.ServeHTTP(rr, httptest.NewRequest("GET", StatusPath, nil))
	require.Equal(t, 401, rr.Code)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", StatusPath, nil)
	req.Header.Set(secretHeader, testSecret)
	mesh.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	var status Status
	require.Nil(t, json.NewDecoder(rr.Body).Decode(&status))
	require.Equal(t, NodeID("node-a"), status.NodeID)
	require.Equal(t, RoleActive, status.Role)
	require.True(t, status.Healthy)
	require.NotZero(t, status.Time)
}

func TestMesh_Status_OneWayLinkFailure(t *testing.T) {
	// Reproduces a firewall dropping one direction of a link (prod, 2026-10-18): node B cannot
	// reach node A's cluster port, node A reaches node B fine. Both nodes stay healthy, so only
	// the per-peer status shows it: B's batches to A all fail, and A never receives from B.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	deadURL := "http://" + dead.Addr().String()
	dead.Close() // Nothing listens here any more: B's sends to A are refused

	var receivedB atomic.Int32
	var meshB *meshCluster
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { meshB.ServeHTTP(w, r) }))
	defer srvB.Close()
	meshA, err := newMeshCluster(newTestMeshConfig("status-a", deadURL), openTestPool(t, schemaDSN), func(m *model.Message) {})
	require.Nil(t, err)
	defer meshA.Close()
	meshB, err = newMeshCluster(newTestMeshConfig("status-b", srvB.URL), openTestPool(t, schemaDSN), func(m *model.Message) { receivedB.Add(1) })
	require.Nil(t, err)
	defer meshB.Close()
	waitFor(t, func() bool { return len(meshA.registry.Peers()) == 1 && len(meshB.registry.Peers()) == 1 })

	sentErrorsBefore := testutil.ToFloat64(metrics.ClusterPeerBatchesSent.WithLabelValues("status-a", "error"))
	receivedBefore := testutil.ToFloat64(metrics.ClusterPeerBatchesReceived.WithLabelValues("status-a"))
	require.Nil(t, meshA.ForwardMessage(model.NewDefaultMessage("mytopic", "a to b")))
	require.Nil(t, meshB.ForwardMessage(model.NewDefaultMessage("mytopic", "b to a")))
	waitFor(t, func() bool {
		return receivedB.Load() == 1 && peerStatus(meshB.Status(), "status-a").Out.BatchesFailed >= 1
	})

	// B's view: everything to A failed, A's batches arrived
	statusB := meshB.Status()
	require.Equal(t, NodeID("status-b"), statusB.NodeID)
	a := peerStatus(statusB, "status-a")
	require.True(t, a.Live)
	require.Equal(t, RoleActive, a.Role)
	require.Equal(t, deadURL, a.AdvertiseURL)
	require.Less(t, a.HeartbeatAge, int64(10))
	require.Equal(t, int64(0), a.Out.BatchesOK)
	require.Equal(t, int64(0), a.Out.LastOK)
	require.NotEmpty(t, a.Out.LastError)
	require.NotZero(t, a.Out.LastErrorAt)
	require.Equal(t, int64(1), a.In.Batches)
	require.Equal(t, int64(1), a.In.Messages)
	require.NotZero(t, a.In.LastAt)

	// A's view: its batch to B went through, and nothing ever came back from B
	b := peerStatus(meshA.Status(), "status-b")
	require.Equal(t, int64(1), b.Out.BatchesOK)
	require.Equal(t, int64(1), b.Out.Messages)
	require.Equal(t, int64(0), b.Out.BatchesFailed)
	require.NotZero(t, b.Out.LastOK)
	require.Equal(t, int64(0), b.In.Batches)
	require.Zero(t, b.In.LastAt)

	// The same, per peer, in the metrics
	require.Greater(t, testutil.ToFloat64(metrics.ClusterPeerBatchesSent.WithLabelValues("status-a", "error")), sentErrorsBefore)
	require.Equal(t, receivedBefore+1, testutil.ToFloat64(metrics.ClusterPeerBatchesReceived.WithLabelValues("status-a")))
	require.NotZero(t, testutil.ToFloat64(metrics.ClusterPeerLastSendSuccess.WithLabelValues("status-b")))

	// And over HTTP, the way the CLI reads it
	fetched, err := NewClient(testSecret, "").Status(context.Background(), srvB.URL)
	require.Nil(t, err)
	require.Equal(t, NodeID("status-b"), fetched.NodeID)
	require.GreaterOrEqual(t, peerStatus(fetched, "status-a").Out.BatchesFailed, int64(1))
	_, err = NewClient("wrong", "").Status(context.Background(), srvB.URL)
	require.ErrorContains(t, err, "401")
}

func peerStatus(status *Status, nodeID NodeID) *PeerStatus {
	for _, p := range status.Peers {
		if p.NodeID == nodeID {
			return p
		}
	}
	return &PeerStatus{}
}
