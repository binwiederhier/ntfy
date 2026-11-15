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
	require.True(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 10,
	}, 3).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:   KindBandwidth,
		Burst:  100,
		Refill: time.Second,
	}, 42).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:   KindRequests,
		Burst:  100,
		Refill: time.Second,
	}, 1).OK)
	u := s.Usage("k")
	require.Equal(t, int64(3), u.Count(KindMessages))
	require.Equal(t, int64(0), u.Count(KindEmails))
	require.Equal(t, int64(0), u.Count(KindBandwidth)) // No daily count, so that it does not hold the key (see Prune)
	require.Equal(t, int64(0), u.Count(KindRequests))
}
