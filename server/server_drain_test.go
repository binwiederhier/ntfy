package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
)

func TestServer_Drain_ClosesStreamsGradually(t *testing.T) {
	s, _ := newDrainTestServer(t, 2*time.Second, true)
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

func TestServer_Drain_DrainingIsReadOnly(t *testing.T) {
	// While draining, new streams and all publishes get 503: the load balancers stopped sending
	// requests during the withdrawing phase, so these are stragglers. Reads (polls) are still served.
	s, _ := newDrainTestServer(t, 3*time.Second, true)
	rr := request(t, s, "PUT", "/mytopic", "published before", nil)
	require.Equal(t, 200, rr.Code)
	openDrainTestStream(s, "/othertopic/json") // Keeps the drain going for the whole test
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Drain(ctx)
	waitFor(t, func() bool {
		return s.readOnly.Load()
	})

	rr = request(t, s, "GET", "/mytopic/json", "", nil)
	require.Equal(t, 503, rr.Code)
	require.Equal(t, errHTTPServiceUnavailableDraining.Code, toHTTPError(t, rr.Body.String()).Code)
	rr = request(t, s, "GET", "/mytopic/ws", "", map[string]string{
		"Upgrade": "websocket",
	})
	require.Equal(t, 503, rr.Code)

	for _, method := range []string{"POST", "PUT"} {
		rr = request(t, s, method, "/mytopic", "hi there", nil)
		require.Equal(t, 503, rr.Code)
		require.Equal(t, errHTTPServiceUnavailableDraining.Code, toHTTPError(t, rr.Body.String()).Code)
	}
	rr = request(t, s, "GET", "/mytopic/publish?message=hi+there", "", nil)
	require.Equal(t, 503, rr.Code)

	rr = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), "published before")
	require.NotContains(t, rr.Body.String(), "hi there")

	rr = request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 503, rr.Code)
	require.JSONEq(t, `{"ready":false,"role":"passive","draining":true}`, rr.Body.String())
}

func TestServer_Drain_WithdrawingServesEverything(t *testing.T) {
	// While withdrawing, the node is not ready, so the readiness agents take it out of the load
	// balancers, but it still serves everything: requests already on their way must succeed
	s, cl := newDrainTestServer(t, time.Second, true)
	s.config.ClusterDrainDelay = 2 * time.Second
	openDrainTestStream(s, "/othertopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Drain(ctx)
	waitFor(t, func() bool {
		return s.cluster.Draining()
	})

	rr := request(t, s, "GET", "/v1/ready", "", nil)
	require.Equal(t, 503, rr.Code)
	require.JSONEq(t, `{"ready":false,"role":"passive","draining":true}`, rr.Body.String())
	cl.mu.Lock()
	require.True(t, cl.draining) // Acts as passive from the start: leadership moves away now
	cl.mu.Unlock()
	rr = request(t, s, "PUT", "/mytopic", "hi there", nil)
	require.Equal(t, 200, rr.Code)
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 2
	})
	require.False(t, s.readOnly.Load())

	// After the delay, it drains
	waitFor(t, func() bool {
		return s.readOnly.Load()
	})
	rr = request(t, s, "PUT", "/mytopic", "too late", nil)
	require.Equal(t, 503, rr.Code)
}

func TestServer_Drain_CanceledWhileWithdrawing(t *testing.T) {
	s, _ := newDrainTestServer(t, time.Second, true)
	s.config.ClusterDrainDelay = time.Minute
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
		return s.cluster.Draining()
	})
	cancel()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("drain did not stop when canceled while withdrawing")
	}
	require.False(t, s.readOnly.Load())
}

func TestServer_Drain_SkippedWithoutAnotherActiveNode(t *testing.T) {
	// Draining the last active node would send its clients nowhere: a passive peer does not count
	s, _ := newDrainTestServer(t, 2*time.Second, false)
	stream := openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	start := time.Now()
	s.Drain(context.Background())
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.False(t, s.cluster.Draining())
	require.Equal(t, 0, countClosed([]<-chan struct{}{stream}))
}

func TestServer_Drain_ZeroDurationClosesAllAtOnce(t *testing.T) {
	// A drain duration of 0 still drains (withdrawing, then read-only), but closes every stream at once
	s, cl := newDrainTestServer(t, 0, true)
	streams := make([]<-chan struct{}, 0)
	for i := 0; i < 5; i++ {
		streams = append(streams, openDrainTestStream(s, "/mytopic/json"))
	}
	waitFor(t, func() bool {
		return s.streams.Len() == 5
	})
	start := time.Now()
	s.Drain(context.Background())
	require.Less(t, time.Since(start), time.Second)
	waitFor(t, func() bool {
		return countClosed(streams) == 5
	})
	require.True(t, s.readOnly.Load())
	cl.mu.Lock()
	defer cl.mu.Unlock()
	require.True(t, cl.draining)
}

func TestServer_Drain_SkippedOnPassiveNode(t *testing.T) {
	s, cl := newDrainTestServer(t, 2*time.Second, true)
	cl.mu.Lock()
	cl.role = cluster.RolePassive
	cl.mu.Unlock()
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.Drain(context.Background())
	require.False(t, s.cluster.Draining())
}

func TestServer_Drain_StopsWhenCanceled(t *testing.T) {
	// A second SIGTERM cancels the drain, and the shutdown goes on at once
	s, _ := newDrainTestServer(t, time.Minute, true)
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
		return s.cluster.Draining()
	})
	cancel()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("drain did not stop when canceled")
	}
}

func TestServer_Drain_StreamsCountsConnectionsNotTopics(t *testing.T) {
	// One connection subscribed to three topics is one stream to close, not three
	s, _ := newDrainTestServer(t, time.Second, true)
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

func newDrainTestServer(t *testing.T, drainDuration time.Duration, activePeer bool) (*Server, *fakeCluster) {
	conf := newTestConfig(t, "")
	conf.ClusterDrainDelay = 0 // Most tests are about the draining phase
	conf.ClusterDrainDuration = drainDuration
	s := newTestServer(t, conf)
	cl := &fakeCluster{}
	if activePeer {
		cl.peers = cluster.Peers{
			&cluster.Peer{
				NodeID: "node-2",
				Role:   cluster.RoleActive,
			},
		}
	}
	s.cluster = cl
	return s, cl
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

func TestServer_Drain_ActsAsPassiveInTheCluster(t *testing.T) {
	// Peers must stop counting a draining node as a place for clients, and leadership
	// must move off it, so the cluster is told
	s, cl := newDrainTestServer(t, time.Second, true)
	openDrainTestStream(s, "/mytopic/json")
	waitFor(t, func() bool {
		return s.streams.Len() == 1
	})
	s.Drain(context.Background())
	cl.mu.Lock()
	defer cl.mu.Unlock()
	require.True(t, cl.draining)
}

func TestServer_Drain_NoDelayedMessagesClaimed(t *testing.T) {
	// A draining node is on its way out: it must not claim due delayed messages, which
	// the next leader sends instead
	s := newTestServer(t, newTestConfig(t, ""))
	cl := &fakeCluster{}
	s.cluster = cl
	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1h",
	})
	require.Equal(t, 200, response.Code)
	msg := toMessage(t, response.Body.String())
	require.Nil(t, s.messageCache.UpdateMessageTime(msg.ID, time.Now().Add(-10*time.Second).Unix()))

	cl.SetDraining(true)
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 0, len(toMessages(t, response.Body.String())))

	cl.SetDraining(false)
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 1, len(toMessages(t, response.Body.String())))
}

func TestServer_Drain_SingleNodeShutdownIsQuiet(t *testing.T) {
	// Without clustering there is nowhere to send clients: a shutdown stops at once, and says
	// nothing about draining (every self-hosted server would log it on every restart)
	var out bytes.Buffer
	log.SetOutput(&out)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
	})
	s := newTestServer(t, newTestConfig(t, ""))
	s.Drain(context.Background())
	require.False(t, s.cluster.Draining())
	require.NotContains(t, strings.ToLower(out.String()), "drain")
}
