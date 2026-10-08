package cluster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
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
