package server

import (
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
