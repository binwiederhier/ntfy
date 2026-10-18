package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func newTestClusterStatusServer(t *testing.T) string {
	return newTestClusterStatusServerFor(t, func() *cluster.Status { return testClusterStatus })
}

func newTestClusterStatusServerFor(t *testing.T, status func() *cluster.Status) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cluster.StatusPath || r.Header.Get("X-Cluster-Secret") != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Nil(t, json.NewEncoder(w).Encode(status()))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// newTestClusterPair serves two nodes' views of a one-way link: app-2 cannot reach app-1, app-1 reaches app-2
func newTestClusterPair(t *testing.T) (app2Addr string) {
	var app1Addr string
	app2Addr = newTestClusterStatusServerFor(t, func() *cluster.Status {
		return &cluster.Status{NodeID: "nyc3-app-2", Role: cluster.RolePassive, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
			NodeID: "nyc3-app-1", AdvertiseURL: "http://" + app1Addr, Role: cluster.RoleActive, Live: true, HeartbeatAge: 2,
			Out: cluster.PeerOutbound{BatchesFailed: 219, LastError: "context deadline exceeded", LastErrorAt: 1792329990},
			In:  cluster.PeerInbound{Batches: 5120, Messages: 828000, LastAt: 1792329999},
		}}}
	})
	app1Addr = newTestClusterStatusServerFor(t, func() *cluster.Status {
		return &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
			NodeID: "nyc3-app-2", AdvertiseURL: "http://" + app2Addr, Role: cluster.RolePassive, Live: true, HeartbeatAge: 1,
			Out: cluster.PeerOutbound{BatchesOK: 5120, Messages: 828000, LastOK: 1792329999},
			In:  cluster.PeerInbound{},
		}}}
	})
	return app2Addr
}

func TestCLI_Cluster_Status(t *testing.T) {
	addr := newTestClusterStatusServer(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "s3cret", "status"}))
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

func TestCLI_Cluster_Status_JSON(t *testing.T) {
	addr := newTestClusterStatusServer(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "s3cret", "status", "--json"}))
	var status cluster.Status
	require.Nil(t, json.Unmarshal(stdout.Bytes(), &status))
	require.Equal(t, cluster.NodeID("nyc3-app-2"), status.NodeID)
	require.Equal(t, int64(219), status.Peers[0].Out.BatchesFailed)
}

func TestCLI_Cluster_Status_WildcardListen(t *testing.T) {
	// A listener bound to all interfaces is queried on localhost
	addr := newTestClusterStatusServer(t)
	port := addr[strings.LastIndex(addr, ":"):]
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", port, "--experimental-cluster-secret", "s3cret", "status"}))
	require.Contains(t, stdout.String(), "nyc3-app-2")
}

func TestCLI_Cluster_Status_WrongSecret(t *testing.T) {
	addr := newTestClusterStatusServer(t)
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "wrong", "status"})
	require.ErrorContains(t, err, "401")
}

func TestCLI_Cluster_Status_NotEnabled(t *testing.T) {
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "cluster", "status"})
	require.ErrorContains(t, err, "clustering is not enabled")
}

func TestCLI_Cluster_StatusAll(t *testing.T) {
	// Every node's view side by side: the one-way link shows as failures on one side and nothing
	// received on the other, from a single command on either node
	addr := newTestClusterPair(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "s3cret", "status", "--all"}))
	out := stdout.String()
	require.Regexp(t, `nyc3-app-1\s+active\s+yes`, out) // Leader
	require.Regexp(t, `nyc3-app-2\s+passive\s+no`, out)
	require.Regexp(t, `nyc3-app-1 -> nyc3-app-2\s+5120\s+0\s+1s ago\s+5120`, out)
	require.Regexp(t, `nyc3-app-2 -> nyc3-app-1\s+0\s+219\s+never\s+0\s+never`, out)
	require.Contains(t, out, "nyc3-app-2 -> nyc3-app-1: no batch accepted, 219 failed")
}

func TestCLI_Cluster_StatusAll_PeerUnreachable(t *testing.T) {
	// A peer whose status cannot be fetched is reported, the rest is still printed
	addr := newTestClusterStatusServerFor(t, func() *cluster.Status {
		return &cluster.Status{NodeID: "nyc3-app-1", Role: cluster.RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*cluster.PeerStatus{{
			NodeID: "nyc3-app-2", AdvertiseURL: "http://127.0.0.1:1", Role: cluster.RolePassive, Live: true,
			Out: cluster.PeerOutbound{BatchesOK: 10, LastOK: 1792329999},
		}}}
	})
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "s3cret", "status", "--all"}))
	out := stdout.String()
	require.Regexp(t, `nyc3-app-1 -> nyc3-app-2\s+10\s+0\s+1s ago\s+\?`, out)
	require.Contains(t, out, "nyc3-app-2: cannot fetch its status")
}

func TestCLI_Cluster_StatusAll_JSON(t *testing.T) {
	addr := newTestClusterPair(t)
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", addr, "--experimental-cluster-secret", "s3cret", "status", "--all", "--json"}))
	var nodes []*clusterNodeStatus
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
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", newTestClusterStatusServerFor(t, status(30)), "--experimental-cluster-secret", "s3cret", "status"}))
	require.Contains(t, stdout.String(), "last seen 30s ago")
	require.Contains(t, stdout.String(), "nyc3-app-2: left the registry 30s ago")

	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "cluster", "--experimental-cluster-listen", newTestClusterStatusServerFor(t, status(3*3600)), "--experimental-cluster-secret", "s3cret", "status"}))
	require.Contains(t, stdout.String(), "last seen 3h0m0s ago")
	require.NotContains(t, stdout.String(), "left the registry")
}
