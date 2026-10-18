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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cluster.StatusPath || r.Header.Get("X-Cluster-Secret") != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Nil(t, json.NewEncoder(w).Encode(testClusterStatus))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
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
