package pg

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
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

func TestLeader_RenewalNoticesLostLock(t *testing.T) {
	// Advisory locks are not replicated: after a database failover the old primary, if still
	// reachable, answers pings but no longer holds our lock. Renewal must check the lock, not
	// just the connection, and drop leadership when it is gone.
	dsn := os.Getenv("NTFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	host, err := Open(dsn)
	require.Nil(t, err)
	defer host.DB.Close()
	l := NewLeader(host.DB, time.Now().UnixNano(), 20*time.Millisecond)
	defer l.Close()
	deadline := time.Now().Add(5 * time.Second)
	for !l.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, l.IsLeader())
	// Take the lock away underneath the live connection
	l.mu.Lock()
	conn := l.conn
	l.mu.Unlock()
	_, err = conn.ExecContext(t.Context(), "SELECT pg_advisory_unlock_all()")
	require.Nil(t, err)
	deadline = time.Now().Add(time.Second)
	for l.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.False(t, l.IsLeader(), "still leader although the lock is gone")
}

// pausingConnector wraps the real driver and parks the renewal goroutine after it has consumed
// a successful renewal result, as if it were descheduled before publishing the result
type pausingConnector struct {
	driver.Connector
	received chan struct{}
	resume   chan struct{}
}

func (c *pausingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &pausingConn{Conn: conn, gate: c}, nil
}

type pausingConn struct {
	driver.Conn
	gate *pausingConnector
}

func (c *pausingConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err != nil {
		return nil, err
	}
	if strings.Contains(q, "pg_is_in_recovery") {
		return &pausingRows{Rows: rows, gate: c.gate}, nil
	}
	return rows, nil
}

func (c *pausingConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}

type pausingRows struct {
	driver.Rows
	gate *pausingConnector
	once sync.Once
}

func (r *pausingRows) Close() error {
	err := r.Rows.Close()
	r.once.Do(func() { close(r.gate.received); <-r.gate.resume })
	return err
}

func TestLeader_StaleSuccessfulRenewalMustNotRestoreLeadership(t *testing.T) {
	// A renewal that succeeded on the wire but whose goroutine is paused before it records the
	// result is stale evidence: meanwhile the server fences the idle session and a follower wins.
	// Recording the renewal with the time it was issued (not processed) keeps the old leader's
	// lease where it belongs, expired.
	dsn := os.Getenv("NTFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	config, err := pgx.ParseConfig(dsn)
	require.Nil(t, err)
	gate := &pausingConnector{Connector: stdlib.GetConnector(*config), received: make(chan struct{}), resume: make(chan struct{})}
	pool := sql.OpenDB(gate)
	defer pool.Close()
	const interval = 50 * time.Millisecond
	baseKey := time.Now().UnixNano()
	old := &Leader{db: pool, key: baseKey, renewInterval: interval}
	defer old.release()
	old.tryAcquireOrRenew(context.Background())
	require.NotNil(t, old.conn)
	old.acquiredAt = time.Now().Add(-time.Hour)
	require.True(t, old.IsLeader())
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), old.leaseDuration())
	defer cancel()
	go func() { old.tryAcquireOrRenew(ctx); close(done) }()
	<-gate.received
	// No more renewals can complete; the server's idle timeout releases the session, and an
	// independent follower takes over
	host, err := Open(dsn)
	require.Nil(t, err)
	defer host.DB.Close()
	follower := NewLeader(host.DB, baseKey, interval)
	defer follower.Close()
	deadline := time.Now().Add(5 * time.Second)
	for !follower.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	followerWon := follower.IsLeader()
	close(gate.resume)
	<-done
	require.True(t, followerWon, "follower should acquire after the server fences the paused session")
	require.False(t, old.IsLeader(), "stale renewal resurrected the old leader while the follower leads")
}

// blockingTransport is a database connection whose writes can be parked: writes then go to a
// pipe nobody reads, which is what a transport that cannot accept a byte looks like to the
// driver (the real driver and its deadlines stay in play).
type blockingTransport struct {
	net.Conn
	blocked atomic.Bool
	writer  net.Conn
	reader  net.Conn
}

func (c *blockingTransport) Write(p []byte) (int, error) {
	if c.blocked.Load() {
		return c.writer.Write(p)
	}
	return c.Conn.Write(p)
}

func (c *blockingTransport) SetDeadline(t time.Time) error {
	c.writer.SetDeadline(t)
	return c.Conn.SetDeadline(t)
}

func (c *blockingTransport) Close() error {
	c.writer.Close()
	c.reader.Close()
	return c.Conn.Close()
}

func TestLeader_ReleaseIsBoundedWhenTheTransportBlocks(t *testing.T) {
	// Releasing runs no query, but the driver still sends a Terminate on close, so a transport
	// that cannot accept a write delays it by that message's deadline (5s in pgx today). The
	// bound is what shutdown depends on: if a driver change ever made the close unbounded, a
	// partitioned database would hang ntfy's shutdown instead of delaying it.
	dsn := os.Getenv("NTFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	config, err := pgx.ParseConfig(dsn)
	require.Nil(t, err)
	var transport *blockingTransport
	config.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		w, r := net.Pipe()
		transport = &blockingTransport{Conn: conn, writer: w, reader: r}
		return transport, nil
	}
	pool := sql.OpenDB(stdlib.GetConnector(*config))
	defer pool.Close()
	l := &Leader{db: pool, key: time.Now().UnixNano(), renewInterval: defaultRenewInterval}
	l.tryAcquireOrRenew(context.Background())
	require.NotNil(t, l.conn)

	transport.blocked.Store(true)
	started := time.Now()
	l.release()
	elapsed := time.Since(started)
	t.Logf("release with a blocked transport took %v", elapsed)
	require.Less(t, elapsed, 15*time.Second, "release must stay bounded by the driver's terminate deadline")
}
