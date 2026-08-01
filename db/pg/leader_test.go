package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func TestLeader_AcquireAndFailover(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t) // skips if NTFY_TEST_DATABASE_URL is unset
	const key = int64(42)
	ctx := context.Background()
	l1 := pg.NewLeader(testDB.Primary(), key)
	l2 := pg.NewLeader(testDB.Primary(), key)
	defer l1.Release()
	defer l2.Release()
	// First to try wins; the second stays follower
	require.True(t, l1.TryAcquire(ctx))
	require.False(t, l2.TryAcquire(ctx))
	require.True(t, l1.IsLeader())
	require.False(t, l2.IsLeader())
	// Repeated TryAcquire on the leader is a no-op ping and reports leadership
	require.True(t, l1.TryAcquire(ctx))
	// Release -> the follower can take over
	l1.Release()
	require.False(t, l1.IsLeader())
	require.True(t, l2.TryAcquire(ctx))
	require.True(t, l2.IsLeader())
}

func TestLeader_ConnectionLossFailover(t *testing.T) {
	// A leader that dies without calling Release (crash, network loss) must not wedge the
	// cluster: the advisory lock is session-scoped, so Postgres releases it when the pinned
	// connection dies, and a follower can take over. Simulated by terminating the lock-holding
	// backend server-side.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	hostA, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer hostA.DB.Close()
	hostB, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer hostB.DB.Close()
	const key = int64(43)
	ctx := context.Background()
	l1 := pg.NewLeader(hostA.DB, key)
	l2 := pg.NewLeader(hostB.DB, key)
	defer l1.Release()
	defer l2.Release()
	require.True(t, l1.TryAcquire(ctx))
	require.False(t, l2.TryAcquire(ctx))
	// Kill the backend holding the lock (advisory lock keys map to classid/objid)
	_, err = hostB.DB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype = 'advisory' AND objid = $1 AND granted`, key)
	require.Nil(t, err)
	// The lock is auto-released; the follower takes over
	waitForCond(t, func() bool { return l2.TryAcquire(ctx) })
	// The old leader discovers its dead connection on the next attempt and stays follower
	require.False(t, l1.TryAcquire(ctx))
}

func waitForCond(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestLeader_DistinctKeysAreIndependent(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t)
	ctx := context.Background()
	l1 := pg.NewLeader(testDB.Primary(), 1)
	l2 := pg.NewLeader(testDB.Primary(), 2)
	defer l1.Release()
	defer l2.Release()
	require.True(t, l1.TryAcquire(ctx))
	require.True(t, l2.TryAcquire(ctx)) // Different keys do not compete
}
