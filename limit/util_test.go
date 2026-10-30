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

func TestKeyOffset_StableAndSpread(t *testing.T) {
	// Pinned values: every node and every restart must agree on a key's offset (FNV-1a, not a
	// seeded hash)
	require.Equal(t, int64(66084), keyOffset("ip:1.2.3.4")) // 18:21:24 UTC
	require.Equal(t, int64(84363), keyOffset("user:u_1"))   // 23:26:03 UTC
	require.Equal(t, int64(24810), keyOffset("k"))          // 06:53:30 UTC
}

func TestPeriodOf(t *testing.T) {
	// A key's period starts at its offset into the UTC day and lasts exactly 24 hours
	start := time.Date(2026, 10, 25, 6, 53, 30, 0, time.UTC) // keyOffset("k")
	p := periodOf("k", start)
	require.Equal(t, p, periodOf("k", start.Add(oneDay-time.Second)))
	require.Equal(t, p+1, periodOf("k", start.Add(oneDay)))
	require.Equal(t, p-1, periodOf("k", start.Add(-time.Second)))
	require.Equal(t, start.Add(oneDay), periodEnd("k", p))

	// Midnight UTC is not a boundary for anyone but a key with offset 0
	midnight := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	require.Equal(t, periodOf("k", midnight.Add(-time.Second)), periodOf("k", midnight))

	// Another key resets at another time
	require.Equal(t, time.Date(2026, 10, 25, 18, 21, 24, 0, time.UTC), periodEnd("ip:1.2.3.4", periodOf("ip:1.2.3.4", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))))
}
