package pg_test

import (
	"context"
	"testing"

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
