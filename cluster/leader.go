package cluster

import (
	"context"
	"database/sql"
	"sync"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/metrics"
)

// leaderLockKey is the Postgres advisory-lock key ("ntfy") for the singleton-job leader.
const leaderLockKey = int64(0x6e746679)

const (
	tryAdvisoryLockQuery = `SELECT pg_try_advisory_lock($1)`
	advisoryUnlockQuery  = `SELECT pg_advisory_unlock($1)`
)

// leader implements singleton-job leader election via a Postgres advisory lock held on a pinned
// connection. The lock auto-releases if the holding connection dies, so a crashed leader is
// replaced without manual fencing or lease bookkeeping. This is a single-job soft leader, never a
// cluster-wide coordinator: the message fan-out path does not depend on it.
type leader struct {
	pool *db.DB
	mu   sync.Mutex
	conn *sql.Conn // holds the advisory lock while this node is leader
	held bool
}

func newLeader(pool *db.DB) *leader {
	return &leader{pool: pool}
}

// tryAcquire attempts to grab (or confirm) the advisory lock on a pinned connection. It is called
// periodically; on a healthy leader it is a cheap ping, on a follower it retries the lock.
func (l *leader) tryAcquire(ctx context.Context) {
	l.mu.Lock()
	conn, held := l.conn, l.held
	l.mu.Unlock()
	if held {
		if conn != nil && conn.PingContext(ctx) == nil {
			return // Still leader, connection healthy
		}
		l.release() // Connection died; the lock is already gone, re-acquire below
	}
	newConn, err := l.pool.Primary().Conn(ctx)
	if err != nil {
		return
	}
	var acquired bool
	if err := newConn.QueryRowContext(ctx, tryAdvisoryLockQuery, leaderLockKey).Scan(&acquired); err != nil || !acquired {
		newConn.Close()
		return
	}
	l.mu.Lock()
	l.conn = newConn
	l.held = true
	l.mu.Unlock()
	metrics.ClusterLeader.Set(1)
}

// release unlocks the advisory lock and returns the pinned connection to the pool.
func (l *leader) release() {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.held = false
	l.mu.Unlock()
	metrics.ClusterLeader.Set(0)
	if conn != nil {
		// Unlock explicitly before returning the connection to the pool: sql.Conn.Close() returns
		// the physical connection to the pool rather than closing it, so the session-scoped lock
		// would otherwise stay held.
		conn.ExecContext(context.Background(), advisoryUnlockQuery, leaderLockKey)
		conn.Close()
	}
}

// isLeader reports whether this node currently holds singleton-job leadership.
func (l *leader) isLeader() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}
