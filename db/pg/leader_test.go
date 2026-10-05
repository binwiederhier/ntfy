package pg_test

import (
	"net"
	"net/url"
	"strings"
	"sync/atomic"
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
	// when the pinned connection dies (simulated by terminating the backend), and the follower
	// takes over. The old belief must expire before the new one begins, never two leaders.
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
	// Kill the backend holding the lock. The key in pg_locks is the schema-scoped one, split
	// into classid (high 32 bits) and objid (low 32 bits)
	classID, objID := uint32(uint64(l1.Key())>>32), uint32(uint64(l1.Key()))
	var terminated int
	require.Nil(t, hostB.DB.QueryRow(`SELECT COUNT(pg_terminate_backend(pid)) FROM pg_locks WHERE locktype = 'advisory' AND classid = $1::oid AND objid = $2::oid AND granted`, int64(classID), int64(objID)).Scan(&terminated))
	require.Equal(t, 1, terminated, "the leader's backend was not found")
	// The old leader notices on its next renewal, the follower only after its hold-off
	waitForNoLeader(t, l1)
	waitForLeader(t, l2)
	require.False(t, l1.IsLeader())
}

func TestLeader_SilentLeaderIsFencedByTheServer(t *testing.T) {
	// A leader whose packets are black-holed (not a clean crash) leaves its backend idle, and
	// Postgres would keep that backend, and the lock, until TCP keepalives give up (hours).
	// The pinned session therefore carries an idle_session_timeout, so the server itself drops
	// a silent leader and the follower takes over without anyone terminating backends.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	proxy := newBlackholeProxy(t, schemaDSN)
	hostA, err := pg.Open(proxy.dsn)
	require.Nil(t, err)
	defer hostA.DB.Close()
	hostB, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	defer hostB.DB.Close()
	l1 := pg.NewLeader(hostA.DB, 45, testRenewInterval)
	defer l1.Close()
	waitForLeader(t, l1)
	l2 := pg.NewLeader(hostB.DB, 45, testRenewInterval)
	defer l2.Close()
	proxy.blackhole()
	waitForNoLeader(t, l1) // Lease expires on the silent side
	waitForLeader(t, l2)   // Server fenced the zombie session; the follower acquired
	require.False(t, l1.IsLeader())
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

func TestLeader_SchemaScopedKeys(t *testing.T) {
	// Advisory locks are database-global, but a "cluster" is defined by its schema (all real
	// nodes share search_path=public; test schemas and multi-tenant databases do not). Two
	// leaders with the same base key on DIFFERENT schemas must both win, or concurrently
	// running test binaries steal each other's leadership.
	dsnA := dbtest.CreateTestPostgresSchema(t)
	dsnB := dbtest.CreateTestPostgresSchema(t)
	hostA, err := pg.Open(dsnA)
	require.Nil(t, err)
	defer hostA.DB.Close()
	hostB, err := pg.Open(dsnB)
	require.Nil(t, err)
	defer hostB.DB.Close()

	leaderA := pg.NewLeader(hostA.DB, pg.LeaderLockKey, 50*time.Millisecond)
	defer leaderA.Close()
	waitForLeader(t, leaderA)
	leaderB := pg.NewLeader(hostB.DB, pg.LeaderLockKey, 50*time.Millisecond)
	defer leaderB.Close()
	waitForLeader(t, leaderB) // Must win too: different schema, independent leadership
}

// waitForNoLeader waits until the node no longer believes it is the leader, or fails the test
func waitForNoLeader(t *testing.T, l *pg.Leader) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !l.IsLeader() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("node still believes it is the leader")
}

// blackholeProxy relays TCP to the database and can stop relaying while keeping every
// connection open, which is what a partitioned database looks like to the client
type blackholeProxy struct {
	dsn    string
	wedged atomic.Bool
	done   chan struct{} // Closed at test end; wedged relays wait on it, which also keeps their sockets alive
}

func newBlackholeProxy(t *testing.T, dsn string) *blackholeProxy {
	t.Helper()
	u, err := url.Parse(dsn)
	require.Nil(t, err)
	target := u.Host
	if !strings.Contains(target, ":") {
		target += ":5432"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	u.Host = ln.Addr().String()
	p := &blackholeProxy{dsn: u.String(), done: make(chan struct{})}
	t.Cleanup(func() {
		ln.Close()
		close(p.done)
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.relay(conn, target)
		}
	}()
	return p
}

func (p *blackholeProxy) blackhole() {
	p.wedged.Store(true)
}

func (p *blackholeProxy) relay(down net.Conn, target string) {
	up, err := net.Dial("tcp", target)
	if err != nil {
		down.Close()
		return
	}
	go p.copy(up, down)
	p.copy(down, up)
}

func (p *blackholeProxy) copy(dst, src net.Conn) {
	buf := make([]byte, 16*1024)
	for {
		if p.wedged.Load() {
			// Hold both sockets open and forward nothing until the test ends. The conns must
			// stay referenced here, or the GC finalizes them and the kernel sends a FIN, which
			// is a clean disconnect rather than a black-hole.
			<-p.done
			src.Close()
			dst.Close()
			return
		}
		src.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
	}
}
