package pg_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

const testRenewInterval = 20 * time.Millisecond // Lease duration 60ms, hold-off 120ms

func TestLeader_AcquireAndFailover(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t) // skips if NTFY_TEST_DATABASE_URL is unset
	const key = int64(42)
	l1 := pg.NewLeader(testDB.Primary(), key, testRenewInterval)
	defer l1.Close()
	// Belief follows the hold-off, it is never instant
	require.False(t, l1.IsLeader())
	waitForLeader(t, l1)
	// A competitor never becomes leader while the leader lives
	l2 := pg.NewLeader(testDB.Primary(), key, testRenewInterval)
	defer l2.Close()
	time.Sleep(300 * time.Millisecond) // Several verification rounds
	require.False(t, l2.IsLeader())
	require.True(t, l1.IsLeader())
	// Close -> the follower takes over
	l1.Close()
	require.False(t, l1.IsLeader())
	waitForLeader(t, l2)
	require.False(t, l1.IsLeader())
}

func TestLeader_ConnectionLossFailover(t *testing.T) {
	// A crashed leader must not wedge the cluster: Postgres releases the session-scoped lock
	// when the pinned connection dies (simulated by terminating the backend), and someone
	// re-acquires. Either node may win; the invariant is one leader eventually, never two.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	hostA, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer hostA.DB.Close()
	hostB, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer hostB.DB.Close()
	const key = int64(43)
	l1 := pg.NewLeader(hostA.DB, key, testRenewInterval)
	defer l1.Close()
	waitForLeader(t, l1)
	l2 := pg.NewLeader(hostB.DB, key, testRenewInterval)
	defer l2.Close()
	// Kill the backend holding the lock (advisory lock keys map to classid/objid)
	_, err = hostB.DB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype = 'advisory' AND objid = $1 AND granted`, key)
	require.Nil(t, err)
	// Eventually exactly one leader again, and never two along the way
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		leader1, leader2 := l1.IsLeader(), l2.IsLeader()
		require.False(t, leader1 && leader2, "two leaders at once")
		if leader1 != leader2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no leader re-emerged after connection loss")
}

func TestLeader_DistinctKeysAreIndependent(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t)
	l1 := pg.NewLeader(testDB.Primary(), 1, testRenewInterval)
	defer l1.Close()
	l2 := pg.NewLeader(testDB.Primary(), 2, testRenewInterval)
	defer l2.Close()
	// Different keys do not compete: both become effective leaders
	waitForLeader(t, l1)
	waitForLeader(t, l2)
}

// waitForLeader waits until the node believes it is the leader, or fails the test
func waitForLeader(t *testing.T, l *pg.Leader) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if l.IsLeader() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("node never became effective leader")
}
