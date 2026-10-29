package server

import (
	"context"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionGauge_AcquireUpToLimit(t *testing.T) {
	g := newSubscriptionGauge()
	for i := 0; i < 3; i++ {
		require.True(t, g.Acquire("ip:1.2.3.4", 3))
	}
	require.False(t, g.Acquire("ip:1.2.3.4", 3))
	require.True(t, g.Acquire("ip:5.6.7.8", 3)) // Keys are counted separately
}

func TestSubscriptionGauge_ReleaseForgetsKey(t *testing.T) {
	g := newSubscriptionGauge()
	require.True(t, g.Acquire("ip:1.2.3.4", 2))
	require.True(t, g.Acquire("ip:1.2.3.4", 2))
	require.False(t, g.Acquire("ip:1.2.3.4", 2))
	g.Release("ip:1.2.3.4")
	require.True(t, g.Acquire("ip:1.2.3.4", 2)) // A released slot is free again
	g.Release("ip:1.2.3.4")
	g.Release("ip:1.2.3.4")
	require.Empty(t, g.counts) // No key is left behind once its subscriptions are gone
}

func TestSubscriptionGauge_ConcurrentNeverExceedsLimit(t *testing.T) {
	g := newSubscriptionGauge()
	var acquired atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.Acquire("ip:1.2.3.4", 10) {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(10), acquired.Load())
	for i := 0; i < 10; i++ {
		g.Release("ip:1.2.3.4")
	}
	require.Empty(t, g.counts)
}

func TestVisitor_SubscriptionLimitSharedPerKey(t *testing.T) {
	c := newTestConfig(t, "")
	c.VisitorSubscriptionLimit = 2
	s := newTestServer(t, c)
	ip := netip.MustParseAddr("9.9.9.9")
	v1 := s.visitor(ip, nil)
	v2 := s.visitor(ip, nil) // A second request from the same IP shares the slots
	require.True(t, v1.SubscriptionAllowed())
	require.True(t, v2.SubscriptionAllowed())
	require.False(t, v1.SubscriptionAllowed())
	require.True(t, s.visitor(netip.MustParseAddr("1.2.3.4"), nil).SubscriptionAllowed())
	v1.RemoveSubscription()
	require.True(t, v2.SubscriptionAllowed())
}

func TestServer_SubscriptionLimit_ReleasedWhenStreamCloses(t *testing.T) {
	c := newTestConfig(t, "")
	c.VisitorSubscriptionLimit = 1
	s := newTestServer(t, c)

	// The open stream takes the only slot
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		request(t, s, "GET", "/mytopic/json", "", nil, func(r *http.Request) {
			*r = *r.WithContext(ctx)
		})
		close(done)
	}()
	waitFor(t, func() bool {
		s.visitors.subscriptions.mu.Lock()
		defer s.visitors.subscriptions.mu.Unlock()
		return len(s.visitors.subscriptions.counts) == 1
	})
	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 429, rr.Code)
	require.Equal(t, 42903, toHTTPError(t, rr.Body.String()).Code)

	// Closing the stream gives it back
	cancel()
	<-done
	rr = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Empty(t, s.visitors.subscriptions.counts)
}
