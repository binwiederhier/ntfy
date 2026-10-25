package limit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShardIndex(t *testing.T) {
	seen := make(map[int]bool)
	for _, key := range []string{"ip:1.2.3.4", "ip:1.2.3.5", "user:u_abc", "ip:2001:db8::", "a", "b", "c", "d", "e", "f"} {
		i := shardIndex(key)
		require.GreaterOrEqual(t, i, 0)
		require.Less(t, i, shardCount)
		require.Equal(t, i, shardIndex(key)) // Stable
		seen[i] = true
	}
	require.Greater(t, len(seen), 3) // Spreads
}

func TestUntilReset(t *testing.T) {
	resetTime := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC) // Midnight UTC
	now := time.Date(2026, 10, 25, 23, 59, 0, 0, time.UTC)
	require.Equal(t, time.Minute, untilReset(now, resetTime))
	now = time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	require.Equal(t, 24*time.Hour, untilReset(now, resetTime))
	resetTime = time.Date(0, 0, 0, 6, 30, 0, 0, time.UTC)
	now = time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)
	require.Equal(t, 30*time.Minute, untilReset(now, resetTime))
}
