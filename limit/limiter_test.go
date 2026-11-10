package limit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLimiter_Value_Bandwidth(t *testing.T) {
	s := newTestService(t)
	l := s.Limiter("k", Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Second,
	})
	require.True(t, l.AllowN(300))
	require.Zero(t, l.Value()) // No daily count; the bucket is what limits it
	require.Equal(t, int64(700), s.Tokens("k", Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Second,
	}))
}

func TestLimiter_AllowNRefundsNegative(t *testing.T) {
	s := newTestService(t)
	l := s.Limiter("k", Rule{
		Kind:   KindMessages,
		Burst:  10,
		Refill: time.Hour,
	})
	require.True(t, l.AllowN(8))
	require.False(t, l.AllowN(3))
	require.Equal(t, int64(8), l.Value())
	require.True(t, l.AllowN(-5)) // Refund, as the limit reader does when a later limiter rejects
	require.True(t, l.AllowN(3))
	require.Equal(t, int64(6), l.Value())
	require.True(t, l.Allow())
	require.Equal(t, int64(7), l.Value())
}
