package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
)

// testClusterStatus is a passive node that cannot reach its active peer, while the peer reaches it
var testClusterStatus = &cluster.Status{
	NodeID:  "nyc3-app-2",
	Role:    cluster.RolePassive,
	Leader:  false,
	Healthy: true,
	Time:    1792330000,
	Peers: []*cluster.PeerStatus{
		{
			NodeID:       "nyc3-app-1",
			AdvertiseURL: "http://10.108.0.2:2587",
			Role:         cluster.RoleActive,
			Live:         true,
			HeartbeatAge: 2,
			Out:          cluster.PeerOutbound{BatchesFailed: 219, LastError: `Post "http://10.108.0.2:2587/v1/cluster/message": context deadline exceeded`, LastErrorAt: 1792329990},
			In:           cluster.PeerInbound{Batches: 5120, Messages: 828000, LastAt: 1792329999},
		},
	},
}

func newTestClusterStatusSocket(t *testing.T) string {
	return newTestClusterStatusSocketFor(t, func() *cluster.Status {
		return testClusterStatus
	}, nil)
}

// newTestClusterStatusSocketFor serves the admin API's cluster status on a unix socket, like a
// server with the admin socket enabled: status() for this node (nil: not clustered), nodes() for all=1
func newTestClusterStatusSocketFor(t *testing.T, status func() *cluster.Status, nodes func() []*cluster.NodeStatus) string {
	socket := filepath.Join(shortTempDir(t), "admin.sock")
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cluster/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status() == nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":40070,"http":400,"error":"invalid request: clustering is not enabled on this server"}`))
			return
		}
		if r.URL.Query().Get("all") == "1" {
			require.Nil(t, json.NewEncoder(w).Encode(nodes()))
			return
		}
		require.Nil(t, json.NewEncoder(w).Encode(status()))
	}))
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	return socket
}

// testClusterPairNodes is both nodes' views of a one-way link: app-2 cannot reach app-1, app-1 reaches app-2
func testClusterPairNodes() []*cluster.NodeStatus {
	return []*cluster.NodeStatus{
		{NodeID: "nyc3-app-1", Status: &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
			NodeID: "nyc3-app-2", Role: cluster.RolePassive, Live: true, HeartbeatAge: 1,
			Out: cluster.PeerOutbound{BatchesOK: 5120, Messages: 828000, LastOK: 1792329999},
		}}}},
		{NodeID: "nyc3-app-2", Status: &cluster.Status{NodeID: "nyc3-app-2", Role: cluster.RolePassive, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
			NodeID: "nyc3-app-1", Role: cluster.RoleActive, Live: true, HeartbeatAge: 2,
			Out: cluster.PeerOutbound{BatchesFailed: 219, LastError: "context deadline exceeded", LastErrorAt: 1792329990},
			In:  cluster.PeerInbound{Batches: 5120, Messages: 828000, LastAt: 1792329999},
		}}}},
	}
}

func TestCLI_Cluster_Status(t *testing.T) {
	socket := newTestClusterStatusSocket(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status"}))
	out := stdout.String()
	require.Contains(t, out, "nyc3-app-2")
	require.Contains(t, out, "passive")
	require.Contains(t, out, "nyc3-app-1")
	require.Contains(t, out, "219") // Failed batches
	require.Contains(t, out, "context deadline exceeded")
	require.Contains(t, out, "5120")                          // Batches received
	require.Contains(t, out, "never")                         // No successful send to nyc3-app-1
	require.Contains(t, out, "nyc3-app-1: no batch accepted") // The warning that matters
}

func TestCLI_Cluster_Status_ThisNodeInTheList(t *testing.T) {
	// The node itself is the table's first row, not a line above it; a leader and an unhealthy node say so
	status := &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: false, Time: 1792330000, Peers: []*cluster.PeerStatus{{
		NodeID: "nyc3-app-2", Role: cluster.RolePassive, Live: true, HeartbeatAge: 1, Out: cluster.PeerOutbound{BatchesOK: 7, LastOK: 1792329999},
	}}}
	socket := newTestClusterStatusSocketFor(t, func() *cluster.Status { return status }, nil)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status"}))
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.True(t, strings.HasPrefix(lines[0], "NODE "), "the table comes first:\n%s", stdout.String())
	require.Regexp(t, `^nyc3-app-1 \(this node, leader\)\s+active\s+no\s`, lines[1])
	require.Regexp(t, `^nyc3-app-2\s+passive\s+yes\s+1s ago\s+7\s`, lines[2])
	require.Contains(t, stdout.String(), "nyc3-app-1 (this node): not healthy, its registry heartbeat is stale")
}

func TestCLI_Cluster_Status_JSON(t *testing.T) {
	socket := newTestClusterStatusSocket(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status", "--json"}))
	var status cluster.Status
	require.Nil(t, json.Unmarshal(stdout.Bytes(), &status))
	require.Equal(t, cluster.NodeID("nyc3-app-2"), status.NodeID)
	require.Equal(t, int64(219), status.Peers[0].Out.BatchesFailed)
}

func TestCLI_Cluster_Status_AdminSocketNotEnabled(t *testing.T) {
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "cluster", "status"})
	require.ErrorContains(t, err, "the admin socket is not enabled")
}

func TestCLI_Cluster_Status_ClusterNotEnabled(t *testing.T) {
	socket := newTestClusterStatusSocketFor(t, func() *cluster.Status { return nil }, nil)
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status"})
	require.ErrorContains(t, err, "clustering is not enabled")
}

func TestCLI_Cluster_Status_ServerNotRunning(t *testing.T) {
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", filepath.Join(t.TempDir(), "admin.sock"), "status"})
	require.ErrorContains(t, err, "cannot reach the server on its admin socket")
}

func TestCLI_Cluster_StatusAll(t *testing.T) {
	// Every node's view side by side: the one-way link shows as failures on one side and nothing
	// received on the other, from a single command on either node
	socket := newTestClusterStatusSocketFor(t, func() *cluster.Status { return testClusterStatus }, testClusterPairNodes)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status", "--all"}))
	out := stdout.String()
	require.Regexp(t, `nyc3-app-1\s+active\s+yes`, out) // Leader
	require.Regexp(t, `nyc3-app-2\s+passive\s+no`, out)
	require.Regexp(t, `nyc3-app-1 -> nyc3-app-2\s+5120\s+0\s+1s ago\s+5120`, out)
	require.Regexp(t, `nyc3-app-2 -> nyc3-app-1\s+0\s+219\s+never\s+0\s+never`, out)
	require.Contains(t, out, "nyc3-app-2 -> nyc3-app-1: no batch accepted, 219 failed")
}

func TestCLI_Cluster_StatusAll_PeerUnreachable(t *testing.T) {
	// A peer whose status cannot be fetched is reported, the rest is still printed
	app1 := &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
		NodeID: "nyc3-app-2", AdvertiseURL: "http://127.0.0.1:1", Role: cluster.RolePassive, Live: true,
		Out: cluster.PeerOutbound{BatchesOK: 10, LastOK: 1792329999},
	}}}
	socket := newTestClusterStatusSocketFor(t, func() *cluster.Status { return app1 }, func() []*cluster.NodeStatus {
		return []*cluster.NodeStatus{{NodeID: "nyc3-app-1", Status: app1}, {NodeID: "nyc3-app-2", Error: "connection refused"}}
	})
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status", "--all"}))
	out := stdout.String()
	require.Regexp(t, `nyc3-app-1 -> nyc3-app-2\s+10\s+0\s+1s ago\s+\?`, out)
	require.Contains(t, out, "nyc3-app-2: cannot fetch its status")
}

func TestCLI_Cluster_StatusAll_JSON(t *testing.T) {
	socket := newTestClusterStatusSocketFor(t, func() *cluster.Status { return testClusterStatus }, testClusterPairNodes)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", socket, "status", "--all", "--json"}))
	var nodes []*cluster.NodeStatus
	require.Nil(t, json.Unmarshal(stdout.Bytes(), &nodes))
	require.Len(t, nodes, 2)
	require.Equal(t, cluster.NodeID("nyc3-app-1"), nodes[0].NodeID)
	require.Equal(t, int64(5120), nodes[0].Status.Peers[0].Out.BatchesOK)
	require.Equal(t, cluster.NodeID("nyc3-app-2"), nodes[1].NodeID)
	require.Equal(t, int64(219), nodes[1].Status.Peers[0].Out.BatchesFailed)
}

func TestCLI_Cluster_Status_PeerLeft(t *testing.T) {
	// A peer that left recently is a problem; one that left long ago (decommissioned) is only listed
	status := func(leftAgo int64) func() *cluster.Status {
		return func() *cluster.Status {
			return &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
				NodeID: "nyc3-app-2", Live: false, LastSeen: 1792330000 - leftAgo, Out: cluster.PeerOutbound{BatchesOK: 10, LastOK: 1792330000 - leftAgo},
			}}}
		}
	}
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", newTestClusterStatusSocketFor(t, status(30), nil), "status"}))
	require.Contains(t, stdout.String(), "last seen 30s ago")
	require.Contains(t, stdout.String(), "nyc3-app-2: left the registry 30s ago")

	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--listen-admin-unix", newTestClusterStatusSocketFor(t, status(3*3600), nil), "status"}))
	require.Contains(t, stdout.String(), "last seen 3h0m0s ago")
	require.NotContains(t, stdout.String(), "left the registry")
}

func TestCLI_Cluster_StatusProblems_OutcomeOrderWithinOneSecond(t *testing.T) {
	// Success and failure in the same Unix second: the recorded last outcome decides, not the timestamps
	failedLast := &cluster.Status{Time: 100, Peers: []*cluster.PeerStatus{{
		NodeID: "peer", Live: true,
		Out: cluster.PeerOutbound{BatchesOK: 1, BatchesFailed: 1, LastOK: 100, LastErrorAt: 100, LastError: "connection refused", LastFailed: true},
	}}}
	require.NotEmpty(t, clusterStatusProblems(failedLast, ""))
	recoveredLast := &cluster.Status{Time: 100, Peers: []*cluster.PeerStatus{{
		NodeID: "peer", Live: true,
		Out: cluster.PeerOutbound{BatchesOK: 1, BatchesFailed: 1, LastOK: 100, LastErrorAt: 100, LastError: "connection refused", LastFailed: false},
	}}}
	require.Empty(t, clusterStatusProblems(recoveredLast, ""))
}

// shortTempDir is a temp dir short enough for a unix socket path (at most ~104 bytes)
func shortTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "ntfy-cli")
	require.Nil(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
