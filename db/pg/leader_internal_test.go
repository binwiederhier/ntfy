package pg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The lease logic is pure time arithmetic, so it is unit-tested here without a database; the
// external leader tests cover the loop end to end.

func TestLeader_Lease_HoldoffMeansNoLeaderRatherThanTwo(t *testing.T) {
	// Freshly acquired lock: belief must wait out the hold-off
	l := &Leader{renewInterval: 20 * time.Second} // Lease duration 1m, hold-off 2m
	l.acquiredAt = time.Now()
	l.renewedAt = l.acquiredAt
	require.False(t, l.IsLeader())
	// Once the hold-off has passed (and verification is fresh), belief begins
	l.acquiredAt = time.Now().Add(-3 * time.Minute)
	l.renewedAt = time.Now()
	require.True(t, l.IsLeader())
}

func TestLeader_Lease_ExpiredLeaseRevokesLeadership(t *testing.T) {
	// A leader that cannot renew its lease (wedged process, long GC pause) must stop
	// believing once the lease expires, even though the lock may still be held
	l := &Leader{renewInterval: 20 * time.Second} // Lease duration 1m, hold-off 2m
	l.acquiredAt = time.Now().Add(-time.Hour)
	l.renewedAt = time.Now().Add(-2 * time.Minute) // Lease expired
	require.False(t, l.IsLeader())
	l.renewedAt = time.Now() // Fresh renewal restores belief
	require.True(t, l.IsLeader())
}

func TestLeader_Lease_ReleasedIsNeverLeader(t *testing.T) {
	// release() zeroes renewedAt, which fails the lease check no matter how old the tenure
	l := &Leader{renewInterval: 20 * time.Second} // Lease duration 1m, hold-off 2m
	l.acquiredAt = time.Now().Add(-time.Hour)
	require.False(t, l.IsLeader())
}
