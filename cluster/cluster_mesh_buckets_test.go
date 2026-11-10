package cluster

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/metrics"
)

func init() {
	bucketRestoreTimeout = 200 * time.Millisecond // Most mesh tests register fake peers at addresses that never answer
}

// testRequestRule is the request bucket of the tests' limiters
var testRequestRule = limit.Rule{
	Kind:   limit.KindRequests,
	Burst:  60,
	Refill: time.Hour,
}

// newTestBucketMesh creates a mesh with a limiter that tracks its deltas, serving its peer API on a
// free port, so peers can send it rounds and read its snapshot
func newTestBucketMesh(t *testing.T, schemaDSN, nodeID string) (*meshCluster, *limit.Service) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	limiter := limit.New(&limit.Config{
		TrackDeltas: true,
	})
	conf := newTestMeshConfig(nodeID, "http://"+listener.Addr().String())
	conf.Limiter = limiter
	mesh, err := newMeshCluster(conf, openTestPool(t, schemaDSN), nil)
	require.Nil(t, err)
	srv := &http.Server{Handler: mesh}
	go srv.Serve(listener)
	t.Cleanup(func() {
		srv.Close()
		mesh.Close()
	})
	return mesh, limiter
}

func TestMesh_GossipChargesPeers(t *testing.T) {
	// What node A's visitors use is charged into node B's buckets by the next gossip round, so a
	// client spread over the nodes gets its limits once, not once per node
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	_, limiterB := newTestBucketMesh(t, schemaDSN, "node-b")
	_, limiterA := newTestBucketMesh(t, schemaDSN, "node-a")
	require.True(t, limiterA.Allow("ip:1.2.3.4", testRequestRule, 42).OK)
	waitFor(t, func() bool {
		return limiterB.Tokens("ip:1.2.3.4", testRequestRule) == 18
	})
}

func TestMesh_BroadcastBucketsSendsEveryKind(t *testing.T) {
	// A round carries every key and kind, with the bucket's rule, so a peer that never saw the key
	// creates its buckets
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	_, limiterB := newTestBucketMesh(t, schemaDSN, "node-b")
	meshA, _ := newTestBucketMesh(t, schemaDSN, "node-a")
	auth := limit.Rule{
		Kind:   limit.KindAuth,
		Burst:  30,
		Refill: time.Hour,
	}
	meshA.broadcastBuckets(limit.Buckets{
		"ip:1.2.3.4": {
			limit.KindRequests: {
				Used:   42,
				Burst:  testRequestRule.Burst,
				Refill: testRequestRule.Refill,
			},
			limit.KindAuth: {
				Used:   2,
				Burst:  auth.Burst,
				Refill: auth.Refill,
			},
		},
	})
	waitFor(t, func() bool {
		return limiterB.Tokens("ip:1.2.3.4", testRequestRule) == 18 && limiterB.Tokens("ip:1.2.3.4", auth) == 28
	})
}

func TestMesh_RestoresBucketsFromPeers(t *testing.T) {
	// A starting node takes the buckets over from its live peers before it serves; a peer that
	// does not answer is left out
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	meshB, limiterB := newTestBucketMesh(t, schemaDSN, "node-b")
	require.True(t, limiterB.Allow("ip:1.2.3.4", testRequestRule, 15).OK)
	// A snapshot leaves out what the next round sends
	meshB.gossipBuckets()
	meshC, err := newMeshCluster(newTestMeshConfig("node-c", "http://127.0.0.1:1"), openTestPool(t, schemaDSN), nil) // Nothing listens
	require.Nil(t, err)
	defer meshC.Close()
	_, limiterA := newTestBucketMesh(t, schemaDSN, "node-a")
	require.Equal(t, int64(45), limiterA.Tokens("ip:1.2.3.4", testRequestRule))
}

func TestMesh_CloseSendsLastBuckets(t *testing.T) {
	// A node that closes sends what it used since its last gossip round, so an orderly restart
	// loses nothing
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	_, limiterB := newTestBucketMesh(t, schemaDSN, "node-b")
	meshA, limiterA := newTestBucketMesh(t, schemaDSN, "node-a")
	require.True(t, limiterA.Allow("ip:1.2.3.4", testRequestRule, 60).OK)
	require.Nil(t, meshA.Close())
	require.Equal(t, int64(0), limiterB.Tokens("ip:1.2.3.4", testRequestRule))
}

func TestMesh_CloseWithGossipRunning(t *testing.T) {
	// A node that closes while its gossip loop holds a round it has not sent yet still gets that
	// round to its peers: Close ends the loop, which finishes its round, before the last round
	entered, release := make(chan struct{}, 1), make(chan struct{})
	testHookBucketGossip = func(c *meshCluster, buckets limit.Buckets) {
		if c.conf.NodeID == "node-a" && len(buckets) > 0 {
			entered <- struct{}{}
			<-release
		}
	}
	t.Cleanup(func() {
		testHookBucketGossip = nil // Registered before the meshes, so it runs after they closed
	})
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	_, limiterB := newTestBucketMesh(t, schemaDSN, "node-b")
	meshA, limiterA := newTestBucketMesh(t, schemaDSN, "node-a")
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release) // Runs before the meshes close, so a failed test does not leave a loop blocked
		}
	})
	require.True(t, limiterA.Allow("ip:1.2.3.4", testRequestRule, 60).OK)
	select {
	case <-entered: // The loop took the round, and has not sent it
	case <-time.After(10 * time.Second):
		t.Fatal("the gossip loop did not take a round")
	}
	closed := make(chan struct{})
	go func() {
		meshA.Close()
		close(closed)
	}()
	waitFor(t, func() bool {
		select {
		case <-meshA.ctx.Done(): // Close is waiting for the loop
			return true
		default:
			return false
		}
	})
	close(release)
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not finish")
	}
	require.Equal(t, int64(0), limiterB.Tokens("ip:1.2.3.4", testRequestRule))
}

func TestMesh_BucketSendErrorsCountedApart(t *testing.T) {
	// A peer that does not take a round of buckets (it restarts, every deploy) is counted under its
	// own path, apart from failed message batches, which alert
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	registerFakePeer(t, pool, "node-b", "http://127.0.0.1:1") // Nothing listens
	mesh, err := newMeshCluster(newTestMeshConfig("node-a", "http://127.0.0.1:2"), pool, nil)
	require.Nil(t, err)
	defer mesh.Close()

	messageErrors := testutil.ToFloat64(metrics.ClusterSendErrors.WithLabelValues(MessagePath))
	bucketErrors := testutil.ToFloat64(metrics.ClusterSendErrors.WithLabelValues(BucketsPath))
	mesh.broadcastBuckets(limit.Buckets{
		"ip:1.2.3.4": {
			limit.KindRequests: {
				Used:   1,
				Burst:  60,
				Refill: 5 * time.Second,
			},
		},
	})
	waitFor(t, func() bool {
		return testutil.ToFloat64(metrics.ClusterSendErrors.WithLabelValues(BucketsPath)) == bucketErrors+1
	})
	require.Equal(t, messageErrors, testutil.ToFloat64(metrics.ClusterSendErrors.WithLabelValues(MessagePath)))
}
