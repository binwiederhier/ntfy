package registry

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func openTestPool(t *testing.T, dsn string) *db.DB {
	t.Helper()
	host, err := pg.Open(dsn)
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestRegistry_NewDoesNotRegister(t *testing.T) {
	// New only sets up the schema and the identity handle; joining the cluster is an explicit
	// Register call, owned by the caller (the mesh registers synchronously at construction).
	// This keeps read-only uses (ops tooling, future admin endpoints) side-effect free.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Equal(t, 0, countRows(t, pool, "node-1"))
	require.Nil(t, r1.Register())
	require.Equal(t, 1, countRows(t, pool, "node-1"))
}

func TestRegistry_RegisterAndPeers(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r1.Register())
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	// Each node sees the other, never itself
	peers, err := r1.Refresh()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-2", peers[0].NodeID)
	require.Equal(t, "http://10.0.0.2:2587", peers[0].AdvertiseURL)
	peers, err = r2.Refresh()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-1", peers[0].NodeID)
}

func TestRegistry_ReRegisterUpdatesAdvertiseURL(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// The same node comes back under a new address; the upsert replaces the row
	old, err := New(pool, "node-2", "http://old:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, old.Register())
	renewed, err := New(pool, "node-2", "http://new:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, renewed.Register())
	peers, err := r1.Refresh()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "http://new:2587", peers[0].AdvertiseURL)
}

func TestRegistry_PeersIsTheLastRefresh(t *testing.T) {
	// Peers never queries: it is empty before the first Refresh, and a node joining afterwards
	// stays invisible until the next one (the heartbeat's job)
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Empty(t, r1.Peers())
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	require.Empty(t, r1.Peers())
	_, err = r1.Refresh()
	require.Nil(t, err)
	require.Len(t, r1.Peers(), 1)
}

func TestRegistry_TTLExcludesSilentNodes(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// A node whose heartbeat is older than the TTL does not count as live
	insertNodeAt(t, pool, "node-silent", "http://10.0.0.9:2587", time.Now().Add(-2*time.Minute))
	peers, err := r1.Refresh()
	require.Nil(t, err)
	require.Empty(t, peers)
}

func TestRegistry_PruneDeletesLongDeadOnly(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// One node beyond the 3x TTL grace period, one merely stale
	insertNodeAt(t, pool, "node-long-dead", "http://10.0.0.8:2587", time.Now().Add(-4*time.Minute))
	insertNodeAt(t, pool, "node-slow", "http://10.0.0.9:2587", time.Now().Add(-2*time.Minute))
	require.Nil(t, r1.Prune())
	require.Equal(t, 0, countRows(t, pool, "node-long-dead"))
	require.Equal(t, 1, countRows(t, pool, "node-slow")) // Slow, not dead: kept
}

func TestRegistry_Deregister(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r1.Register())
	require.Equal(t, 1, countRows(t, pool, "node-1"))
	require.Nil(t, r1.Deregister())
	require.Equal(t, 0, countRows(t, pool, "node-1"))
}

func TestRegistry_PeersSurvivesDatabaseOutage(t *testing.T) {
	// During a database outage, Refresh fails but Peers keeps serving the last snapshot, without
	// a query and without an error: fan-out keeps flowing to known peers, and the publish path
	// neither waits on the database nor logs a warning per message.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	_, err = r1.Refresh()
	require.Nil(t, err)
	require.Nil(t, pool.Close())
	_, err = r1.Refresh()
	require.NotNil(t, err)
	started := time.Now()
	peers := r1.Peers()
	require.Less(t, time.Since(started), 100*time.Millisecond) // No database round trip, let alone a timeout
	require.Len(t, peers, 1)
	require.Equal(t, "node-2", peers[0].NodeID)
}

func TestRegistry_ConcurrentCreate(t *testing.T) {
	// Multiple nodes cold-booting on a fresh database must not race on table creation: CREATE
	// TABLE IF NOT EXISTS is not atomic in PostgreSQL, so creation is serialized via an advisory
	// lock. Without it, this test fails sporadically with a duplicate-key error on pg_class.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			pool, err := pg.Open(schemaDSN)
			if err != nil {
				errs <- err
				return
			}
			defer pool.DB.Close()
			_, err = New(db.New(pool, nil), fmt.Sprintf("node-%d", i), "http://127.0.0.1:1", time.Second)
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		require.Nil(t, <-errs)
	}
}

func TestRegistry_SchemaVersionWritten(t *testing.T) {
	// The registry participates in the shared schema_version framework like every other store,
	// so future table changes can be applied as migrations.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	_, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	var version int
	require.Nil(t, pool.QueryRow(`SELECT version FROM schema_version WHERE store = $1`, schemaStoreKey).Scan(&version))
	require.Equal(t, schemaVersion, version)
	// Setup is idempotent: a second node boots against the migrated schema
	_, err = New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
}

func TestRegistry_SchemaVersionFromTheFuture(t *testing.T) {
	// A node running older code must refuse to touch a schema migrated by newer code
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	_, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	_, err = pool.Exec(`UPDATE schema_version SET version = 99 WHERE store = $1`, schemaStoreKey)
	require.Nil(t, err)
	_, err = New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Error(t, err)
}

// insertNodeAt registers a fake node with a chosen heartbeat. Register() stamps the database
// clock, which is what production wants but leaves tests no way to age a row.
func insertNodeAt(t *testing.T, pool *db.DB, nodeID, url string, heartbeat time.Time) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO node_registry (node_id, advertise_url, last_heartbeat) VALUES ($1, $2, $3)`, nodeID, url, heartbeat.Unix())
	require.Nil(t, err)
}

func countRows(t *testing.T, pool *db.DB, nodeID string) int {
	t.Helper()
	var count int
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = $1`, nodeID).Scan(&count))
	return count
}

func TestRegistry_RefreshReplacesSnapshot(t *testing.T) {
	// Refresh (called by the mesh heartbeat) sees a newly joined node and replaces the snapshot
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r1.Register())
	_, err = r1.Refresh()
	require.Nil(t, err)
	require.Len(t, r1.Peers(), 0) // Snapshot taken while alone

	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	require.Len(t, r1.Peers(), 0) // Still the old snapshot

	peers, err := r1.Refresh()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-2", peers[0].NodeID)
	require.Len(t, r1.Peers(), 1) // Snapshot replaced
}

// Leadership is derived from membership, so these tests drive Register/Refresh by hand (the
// mesh's heartbeat does both every tick) and assert the one invariant that matters: never two
// leaders, and eventually one.

// leaderTestTTL is the liveness window and the promotion hold-off. Registry calls are bounded
// at ttl/2 (see opContext), so a tighter value makes these tests fail on a slow or contended
// database rather than on the behaviour they are checking.
const (
	leaderTestTTL = time.Second
	// Everything leadership does is a multiple of the TTL (lease, hold-off), and a takeover
	// costs TTL + hold-off, so the waits are derived rather than guessed
	leaderTestLease   = leaseFactor * leaderTestTTL
	leaderTestHoldoff = holdoffFactor * leaderTestTTL
	leaderTestWait    = 3 * (leaderTestTTL + leaderTestHoldoff)
	// How often the leadership tests drive a tick. Each tick is a round trip per registry, so
	// this is also the query rate they put on the database while running in parallel; the
	// windows under test are seconds, so 50ms granularity costs nothing.
	leaderTestPoll = 50 * time.Millisecond
)

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
	deadline := time.Now().Add(leaderTestWait)
	for time.Now().Before(deadline) {
		tick(t, append([]*Registry{r}, others...)...)
		for _, o := range others {
			require.False(t, r.IsLeader() && o.IsLeader(), "two leaders at once")
		}
		if r.IsLeader() {
			return
		}
		time.Sleep(leaderTestPoll)
	}
	t.Fatal("never became leader")
}

func TestRegistry_LowestLiveNodeLeads(t *testing.T) {
	t.Parallel() // Own schema and own registries, so these run alongside each other
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
	t.Parallel() // Own schema and own registries, so these run alongside each other
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
		require.Less(t, time.Since(started), leaderTestWait, "app2 never took over")
		tick(t, b)
		time.Sleep(leaderTestPoll)
	}
	require.False(t, a.IsLeader())
	t.Logf("takeover after the leader stopped heartbeating: %v (TTL %v)", time.Since(started).Round(time.Millisecond), leaderTestTTL)
}

func TestRegistry_ReturningLowerNodePreemptsWithoutOverlap(t *testing.T) {
	// The case that makes this approach need a hold-off at all: app1 comes back and will take
	// leadership from app2 purely because its id sorts lower. It must not believe until app2's
	// own belief can no longer be alive.
	t.Parallel() // Own schema and own registries, so these run alongside each other
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
		require.Less(t, time.Since(started), leaderTestWait, "app1 never took over")
		tick(t, a, b)
		time.Sleep(leaderTestPoll)
	}
	require.False(t, b.IsLeader(), "the incumbent must have stepped down")
	t.Logf("preemption by a returning lower id took %v", time.Since(started).Round(time.Millisecond))
}

func TestRegistry_PartitionedLeaderStepsDown(t *testing.T) {
	// A leader that cannot reach the database stops believing within one TTL, because its
	// registration deadline runs out. Peers stop seeing it as live at the same point.
	t.Parallel() // Own schema and own registries, so these run alongside each other
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, a)
	require.Nil(t, pool.Close()) // The database is gone for this node
	require.Error(t, a.Register())
	require.Eventually(t, func() bool { return !a.IsLeader() }, 2*time.Second, 5*time.Millisecond)
}

func TestRegistry_LivenessIsDecidedByTheDatabaseClock(t *testing.T) {
	// Liveness timestamps are compared across nodes, so both the write and the cutoff have to
	// come from the database: with node clocks, one running fast declares live peers dead and
	// one running behind looks dead to everybody. Behaviour alone cannot tell the two apart
	// here, because the test process and the database share a clock, so this pins the mechanism
	// as well. (The cross-host version is a harness drill: a node's clock pushed an hour ahead
	// changed neither liveness nor leadership.)
	require.Contains(t, upsertNodeQuery, "EXTRACT(EPOCH FROM now())", "heartbeats must be stamped by the database")
	require.Contains(t, selectLiveNodesQuery, "EXTRACT(EPOCH FROM now())", "the liveness cutoff must be evaluated by the database")
	require.Contains(t, pruneStaleNodesQuery, "EXTRACT(EPOCH FROM now())", "pruning must use the database clock")

	// And the behaviour that follows: this node's heartbeat lands on the database's clock, a row
	// as an hour-fast node would have written it still counts as live, and one from an hour-slow
	// node does not.
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	r, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r.Register())
	var skew int64
	require.Nil(t, pool.QueryRow(`SELECT EXTRACT(EPOCH FROM now())::BIGINT - last_heartbeat FROM node_registry WHERE node_id = 'node-1'`).Scan(&skew))
	require.LessOrEqual(t, skew, int64(1))

	insertNodeAt(t, pool, "node-fast", "http://10.0.0.2:2587", time.Now().Add(time.Hour))
	insertNodeAt(t, pool, "node-slow", "http://10.0.0.3:2587", time.Now().Add(-time.Hour))
	peers, err := r.Refresh()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-fast", peers[0].NodeID)
}

func TestRegistry_HeartbeatWithoutObservingIsNotLeadership(t *testing.T) {
	// A node can keep its registration fresh while its view of the registry goes stale: its
	// refresh stalls, or it only ever writes. It must stop believing then, because the hold-off
	// it earned rests on an observation that no longer holds. Without that check the incumbent
	// keeps leading while a returning lower-id node earns its own hold-off, and both believe.
	t.Parallel() // Own schema and own registries, so these run alongside each other
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, b)

	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	deadline := time.Now().Add(leaderTestWait)
	for time.Now().Before(deadline) {
		require.Nil(t, b.Register()) // The incumbent's registration stays fresh...
		tick(t, a)                   // ...but only app1 ever looks at the registry
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		if a.IsLeader() {
			require.False(t, b.IsLeader(), "the incumbent kept believing on a stale observation")
			return
		}
		time.Sleep(leaderTestPoll)
	}
	t.Fatal("app1 never took over")
}

func TestRegistry_FrozenLeaderMustReEarnTheHoldOff(t *testing.T) {
	// A process that was frozen (SIGSTOP, a paused VM, a long stall) comes back holding an old
	// observation. Counting that pause toward the promotion hold-off let it believe again the
	// moment it registered once, while the incumbent had not yet noticed it was back: two
	// leaders, measured at 436ms in a two-process run before this was fixed.
	t.Parallel() // Own schema and own registries, so these run alongside each other
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	a, err := New(pool, "app1", "http://10.0.0.1:2587", leaderTestTTL)
	require.Nil(t, err)
	b, err := New(pool, "app2", "http://10.0.0.2:2587", leaderTestTTL)
	require.Nil(t, err)
	waitForLeader(t, a, b)

	// app1 freezes: no heartbeats, no reads. Its own deadlines lapse and app2 takes over,
	// which means ticking only app2 from here (ticking app1 would thaw it).
	time.Sleep(2 * leaderTestLease)
	require.False(t, a.IsLeader())
	frozen := time.Now()
	for !b.IsLeader() {
		require.False(t, a.IsLeader() && b.IsLeader(), "two leaders at once")
		require.Less(t, time.Since(frozen), leaderTestWait, "app2 never took over")
		tick(t, b)
		time.Sleep(leaderTestPoll)
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
		require.Less(t, time.Since(started), leaderTestWait, "app1 never recovered leadership")
		tick(t, a, b)
		time.Sleep(leaderTestPoll)
	}
}

func TestRegistry_LeaseLapsesBeforeAnySuccessorBegins(t *testing.T) {
	// The invariant behind "never two leaders": a deposed leader's belief must end before any
	// successor can finish its hold-off. The successor cannot even start counting until the old
	// leader's row is stale, one TTL after its last heartbeat, so the comparison is
	// lease < TTL + hold-off. Asserted here because the factors are tunable.
	require.Less(t, leaseFactor, 1+holdoffFactor,
		"a leader could still believe while a successor starts: lease %d TTLs, successor starts at %d TTLs",
		leaseFactor, 1+holdoffFactor)
	// And belief must not outlive liveness by more than the hold-off can absorb, nor be so
	// short that it ends before peers would even notice this node is gone
	require.GreaterOrEqual(t, leaseFactor, 1, "the lease must cover the liveness window")
}

func TestRegistry_ViewIsOnlyFreshAfterARecentRefresh(t *testing.T) {
	// Peers serves the last known list when the database is unreachable, which keeps fan-out
	// alive but must not be mistaken for the current membership: anything that reports
	// membership outwards (the member list the LB agents read) needs to know the difference.
	t.Parallel()
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	r, err := New(pool, "node-a", "http://127.0.0.1:1", leaderTestTTL)
	require.Nil(t, err)
	require.False(t, r.Fresh(), "a registry that has never read the database is not fresh")

	require.Nil(t, r.Register())
	_, err = r.Refresh()
	require.Nil(t, err)
	require.True(t, r.Fresh())

	// The view ages out on its own, without any failed call
	deadline := time.Now().Add(leaderTestWait)
	for r.Fresh() && time.Now().Before(deadline) {
		time.Sleep(leaderTestPoll)
	}
	require.False(t, r.Fresh(), "the view never went stale")
}
