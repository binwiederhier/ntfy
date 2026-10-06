package registry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
)

// Leadership is derived from membership, so these tests drive Register/Refresh by hand (the
// mesh's heartbeat does both every tick) and assert the one invariant that matters: never two
// leaders, and eventually one.

// leaderTestTTL is the liveness window and the promotion hold-off. Registry calls are bounded
// at ttl/2 (see opContext), so a tighter value makes these tests fail on a slow or contended
// database rather than on the behaviour they are checking.
const leaderTestTTL = time.Second

func tick(t *testing.T, rs ...*Registry) {
	t.Helper()
	for _, r := range rs {
		require.Nil(t, r.Register())
		_, err := r.Refresh()
		require.Nil(t, err)
	}
}

func waitForLeader(t *testing.T, r *Registry, others ...*Registry) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tick(t, append([]*Registry{r}, others...)...)
		for _, o := range others {
			require.False(t, r.IsLeader() && o.IsLeader(), "two leaders at once")
		}
		if r.IsLeader() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("never became leader")
}

func TestRegistry_LowestLiveNodeLeads(t *testing.T) {
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	c, err := New(pool, "app3", "http://10.0.0.3:2587", leaderTestTTL)
	require.Nil(t, err)
	// Nobody leads before the hold-off has elapsed
	tick(t, a, b, c)
	require.False(t, a.IsLeader())
	waitForLeader(t, a, b, c)
	require.False(t, b.IsLeader())
	require.False(t, c.IsLeader())
}

func TestRegistry_LeaderFailoverHasAGapNotAnOverlap(t *testing.T) {
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, a, b)
	// app1 stops heartbeating (crashed, or cut off from the database). Only app2 ticks now, so
	// app1's belief must lapse on its own registration deadline before app2's hold-off ends.
	started := time.Now()
	for !b.IsLeader() {
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		require.Less(t, time.Since(started), 5*time.Second, "app2 never took over")
		tick(t, b)
		time.Sleep(10 * time.Millisecond)
	}
	require.False(t, a.IsLeader())
	t.Logf("takeover after the leader stopped heartbeating: %v (TTL %v)", time.Since(started).Round(time.Millisecond), leaderTestTTL)
}

func TestRegistry_ReturningLowerNodePreemptsWithoutOverlap(t *testing.T) {
	// The case that makes this approach need a hold-off at all: app1 comes back and will take
	// leadership from app2 purely because its id sorts lower. It must not believe until app2's
	// own belief can no longer be alive.
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, b)
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	tick(t, a)
	require.False(t, a.IsLeader(), "a returning lower-id node must wait out the hold-off")
	// Both keep ticking: at no point may both believe
	started := time.Now()
	for !a.IsLeader() {
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		require.Less(t, time.Since(started), 5*time.Second, "app1 never took over")
		tick(t, a, b)
		time.Sleep(10 * time.Millisecond)
	}
	require.False(t, b.IsLeader(), "the incumbent must have stepped down")
	t.Logf("preemption by a returning lower id took %v", time.Since(started).Round(time.Millisecond))
}

func TestRegistry_PartitionedLeaderStepsDown(t *testing.T) {
	// A leader that cannot reach the database stops believing within one TTL, because its
	// registration deadline runs out. Peers stop seeing it as live at the same point.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, a)
	require.Nil(t, pool.Close()) // The database is gone for this node
	require.Error(t, a.Register())
	require.Eventually(t, func() bool { return !a.IsLeader() }, 2*time.Second, 5*time.Millisecond)
}

func TestRegistry_LivenessUsesTheDatabaseClock(t *testing.T) {
	// Liveness and leadership compare timestamps across nodes, so they must not depend on any
	// node's clock: a row written with a badly skewed clock is judged by the database's now().
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	a, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	// A lower-id node whose clock is an hour ahead: it would look live forever if the cutoff
	// came from a node's clock, and would hold leadership away from app2
	insertNodeAt(t, pool, "app1", "http://10.0.0.1:2587", time.Now().Add(-time.Hour))
	waitForLeader(t, a)
}

func TestRegistry_FrozenLeaderMustReEarnTheHoldOff(t *testing.T) {
	// A process that was frozen (SIGSTOP, a paused VM, a long stall) comes back holding an old
	// observation. Counting that pause toward the promotion hold-off let it believe again the
	// moment it registered once, while the incumbent had not yet noticed it was back: two
	// leaders, measured at 436ms in a two-process run before this was fixed.
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, a, b)

	// app1 freezes: no heartbeats, no reads. Its own deadlines lapse and app2 takes over,
	// which means ticking only app2 from here (ticking app1 would thaw it).
	time.Sleep(2 * leaderTestTTL)
	require.False(t, a.IsLeader())
	frozen := time.Now()
	for !b.IsLeader() {
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		require.Less(t, time.Since(frozen), 5*time.Second, "app2 never took over")
		tick(t, b)
		time.Sleep(10 * time.Millisecond)
	}

	// app1 thaws. One register plus one refresh must NOT hand leadership straight back.
	require.Nil(t, a.Register())
	_, err = a.Refresh()
	require.Nil(t, err)
	require.False(t, a.IsLeader(), "a thawed node believed again without re-earning the hold-off")
	require.True(t, b.IsLeader(), "the incumbent should still hold it at this point")

	// And once it has observed continuously for a hold-off, it may take over, with b stepping
	// down first (that part is TestRegistry_ReturningLowerNodePreemptsWithoutOverlap)
	started := time.Now()
	for !a.IsLeader() {
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		require.Less(t, time.Since(started), 5*time.Second, "app1 never recovered leadership")
		tick(t, a, b)
		time.Sleep(10 * time.Millisecond)
	}
}
