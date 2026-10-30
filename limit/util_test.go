package limit

import (
	"fmt"
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
	// Pinned: every node and every restart must agree on a key's offset (FNV-1a 64, unseeded)
	require.Equal(t, 62084*time.Second, keyOffset("ip:1.2.3.4")) // 17:14:44 UTC
	require.Equal(t, 21643*time.Second, keyOffset("user:u_1"))   // 06:00:43 UTC
	require.Equal(t, 13706*time.Second, keyOffset("k"))          // 03:48:26 UTC

	// Spread: a thousand keys reset in every hour of the day, on whole seconds
	hours := make(map[time.Duration]bool)
	for i := 0; i < 1000; i++ {
		offset := keyOffset(fmt.Sprintf("ip:10.0.%d.%d", i/256, i%256))
		require.GreaterOrEqual(t, offset, time.Duration(0))
		require.Less(t, offset, oneDay)
		require.Zero(t, offset%time.Second)
		hours[offset/time.Hour] = true
	}
	require.Len(t, hours, 24)
}

func TestPeriodStart(t *testing.T) {
	// A key's period starts at its offset into the UTC day and lasts exactly 24 hours
	start := time.Date(2026, 10, 25, 3, 48, 26, 0, time.UTC) // keyOffset("k")
	require.Equal(t, start.Unix(), periodStart("k", start))
	require.Equal(t, start.Unix(), periodStart("k", start.Add(oneDay-time.Second)))
	require.Equal(t, start.Add(oneDay).Unix(), periodStart("k", start.Add(oneDay)))
	require.Equal(t, start.Add(-oneDay).Unix(), periodStart("k", start.Add(-time.Second)))
	require.Equal(t, start.Add(oneDay), periodEnd("k", start))
	require.Equal(t, start.Add(oneDay), periodEnd("k", start.Add(oneDay-time.Second)))

	// Midnight UTC is not a boundary for a key whose offset is not 0
	midnight := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	require.Equal(t, periodStart("k", midnight.Add(-time.Second)), periodStart("k", midnight))

	// Another key resets at another time
	noon := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 10, 25, 17, 14, 44, 0, time.UTC), periodEnd("ip:1.2.3.4", noon))
	require.Equal(t, time.Date(2026, 10, 26, 3, 48, 26, 0, time.UTC), periodEnd("k", noon))
}
