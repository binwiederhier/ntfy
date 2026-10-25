package limit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKind_String_ParseKind(t *testing.T) {
	for k := Kind(0); k < numKinds; k++ {
		parsed, ok := ParseKind(k.String())
		require.True(t, ok, k.String())
		require.Equal(t, k, parsed)
	}
	require.Equal(t, "messages", KindMessages.String())
	require.Equal(t, "bandwidth", KindBandwidth.String())
	_, ok := ParseKind("nope")
	require.False(t, ok)
}

func TestUsage_Count(t *testing.T) {
	s := newTestService(t)
	require.True(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 10}, 3).OK)
	require.True(t, s.Allow("k", Rule{Kind: KindBandwidth, Burst: 100, Refill: time.Second}, 42).OK)
	u := s.Usage("k")
	require.Equal(t, int64(3), u.Count(KindMessages))
	require.Equal(t, int64(42), u.Count(KindBandwidth))
	require.Equal(t, int64(0), u.Count(KindEmails))
}

func TestLimiter_AllowNRefundsNegative(t *testing.T) {
	s := newTestService(t)
	l := s.Limiter("k", Rule{Kind: KindBandwidth, Burst: 10, Refill: time.Hour})
	require.True(t, l.AllowN(8))
	require.False(t, l.AllowN(3))
	require.Equal(t, int64(8), l.Value())
	require.True(t, l.AllowN(-5)) // Refund, as the limit reader does when a later limiter rejects
	require.True(t, l.AllowN(3))
	require.Equal(t, int64(6), l.Value())
	require.True(t, l.Allow())
	require.Equal(t, int64(7), l.Value())
}
