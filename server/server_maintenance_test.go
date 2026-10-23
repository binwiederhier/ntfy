package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
)

func TestServer_Maintenance_ClosesStreamsGradually(t *testing.T) {
	s, _ := newDrainTestServer(t, 2*time.Second, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RoleActive))
	streams := make([]<-chan struct{}, 0)
	for i := 0; i < 20; i++ {
		streams = append(streams, openDrainTestStream(s, "/mytopic/json"))
	}
	waitFor(t, func() bool {
		return s.streams.Len() == 20
	})

	// Halfway through, some streams are closed, but not all of them
	drained := make(chan struct{})
	start := time.Now()
	go func() {
		s.Drain(context.Background())
		close(drained)
	}()
	time.Sleep(time.Second)
	closed := countClosed(streams)
	require.Greater(t, closed, 3)
	require.Less(t, closed, 17)

	// At the end, all of them are, and the drain is done
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not finish")
	}
	require.Less(t, time.Since(start), 4*time.Second)
	waitFor(t, func() bool {
		return countClosed(streams) == 20
	})
	require.Equal(t, 0, s.streams.Len())
}

func TestServer_Maintenance_RejectsNewSubscriptionsOnly(t *testing.T) {
	// In maintenance mode, new streams get 503 (the load balancer retries them on another node), but
	// publishes and polls are short and still served: a load balancer does not retry a POST
	s, _ := newDrainTestServer(t, 3*time.Second, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RoleActive))
	openDrainTestStream(s, "/othertopic/json") // Keeps the drain going for the whole test
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Drain(ctx)
	waitFor(t, func() bool {
		return s.maintenance.Load()
	})

	rr := request(t, s, "GET", "/mytopic/json", "", nil)
	require.Equal(t, 503, rr.Code)
	require.Equal(t, errHTTPServiceUnavailableMaintenance.Code, toHTTPError(t, rr.Body.String()).Code)
	rr = request(t, s, "GET", "/mytopic/ws", "", map[string]string{
		"Upgrade": "websocket",
	})
	require.Equal(t, 503, rr.Code)

	rr = request(t, s, "POST", "/mytopic", "hi there", nil)
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), "hi there")

	rr = request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 503, rr.Code)
	require.JSONEq(t, `{"ready":false,"role":"active","maintenance":true}`, rr.Body.String())
}

func TestServer_Maintenance_SkippedWithoutAnotherActiveNode(t *testing.T) {
	// Draining the last active node would send its clients nowhere: a passive peer does not count
	s, _ := newDrainTestServer(t, 2*time.Second, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RolePassive))
	stream := openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	start := time.Now()
	s.Drain(context.Background())
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.False(t, s.maintenance.Load())
	require.Equal(t, 0, countClosed([]<-chan struct{}{stream}))
}

func TestServer_Maintenance_SkippedWithZeroDuration(t *testing.T) {
	s, _ := newDrainTestServer(t, 0, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RoleActive))
	stream := openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.Drain(context.Background())
	require.False(t, s.maintenance.Load())
	require.Equal(t, 0, countClosed([]<-chan struct{}{stream}))
}

func TestServer_Maintenance_SkippedOnPassiveNode(t *testing.T) {
	s, cl := newDrainTestServer(t, 2*time.Second, drainTestMember("node-1", cluster.RolePassive), drainTestMember("node-2", cluster.RoleActive))
	cl.mu.Lock()
	cl.role = cluster.RolePassive
	cl.mu.Unlock()
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.Drain(context.Background())
	require.False(t, s.maintenance.Load())
}

func TestServer_Maintenance_StopsWhenCanceled(t *testing.T) {
	// A second SIGTERM cancels the drain, and the shutdown goes on at once
	s, _ := newDrainTestServer(t, time.Minute, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RoleActive))
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	ctx, cancel := context.WithCancel(context.Background())
	drained := make(chan struct{})
	go func() {
		s.Drain(ctx)
		close(drained)
	}()
	waitFor(t, func() bool {
		return s.maintenance.Load()
	})
	cancel()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("drain did not stop when canceled")
	}
}

func TestServer_Maintenance_StreamsCountsConnectionsNotTopics(t *testing.T) {
	// One connection subscribed to three topics is one stream to close, not three
	s, _ := newDrainTestServer(t, time.Second)
	streamHTTP := openDrainTestStream(s, "/a,b,c/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.streams.CancelSome(1)
	select {
	case <-streamHTTP:
	case <-time.After(2 * time.Second):
		t.Fatal("stream not closed")
	}
	waitFor(t, func() bool {
		return s.streams.Len() == 0
	})
}

func newDrainTestServer(t *testing.T, drainDuration time.Duration, members ...cluster.Member) (*Server, *fakeCluster) {
	conf := newTestConfig(t, "")
	conf.ClusterNodeID = "node-1"
	conf.ClusterDrainDuration = drainDuration
	s := newTestServer(t, conf)
	cl := &fakeCluster{
		members: members,
	}
	s.cluster = cl
	return s, cl
}

func drainTestMember(nodeID string, role cluster.Role) cluster.Member {
	return cluster.Member{
		NodeID:  cluster.NodeID(nodeID),
		Role:    role,
		Healthy: true,
	}
}

// openDrainTestStream opens a subscriber stream; the returned channel is closed when it ends
func openDrainTestStream(s *Server, path string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		s.handle(rr, req)
		close(done)
	}()
	return done
}

func countClosed(streams []<-chan struct{}) int {
	closed := 0
	for _, stream := range streams {
		select {
		case <-stream:
			closed++
		default:
		}
	}
	return closed
}

func TestServer_Maintenance_ActsAsPassiveInTheCluster(t *testing.T) {
	// Peers must stop counting a node in maintenance mode as a place for clients, and leadership
	// must move off it, so the cluster is told
	s, cl := newDrainTestServer(t, time.Second, drainTestMember("node-1", cluster.RoleActive), drainTestMember("node-2", cluster.RoleActive))
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.Drain(context.Background())
	cl.mu.Lock()
	defer cl.mu.Unlock()
	require.True(t, cl.maintenance)
}

func TestServer_Maintenance_NoDelayedMessagesClaimed(t *testing.T) {
	// A node in maintenance mode is on its way out: it must not claim due delayed messages, which
	// the next leader sends instead
	s := newTestServer(t, newTestConfig(t, ""))
	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1h",
	})
	require.Equal(t, 200, response.Code)
	msg := toMessage(t, response.Body.String())
	require.Nil(t, s.messageCache.UpdateMessageTime(msg.ID, time.Now().Add(-10*time.Second).Unix()))

	s.maintenance.Store(true)
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 0, len(toMessages(t, response.Body.String())))

	s.maintenance.Store(false)
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 1, len(toMessages(t, response.Body.String())))
}
