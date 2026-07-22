package pg

import (
	"context"
	"database/sql"
	"sync"
)

const (
	tryAdvisoryLockQuery = `SELECT pg_try_advisory_lock($1)`
	advisoryUnlockQuery  = `SELECT pg_advisory_unlock($1)`
)

// Leader implements singleton-job leader election via a Postgres advisory lock held on a pinned
// connection. The lock auto-releases if the holding connection dies, so a crashed leader is
// replaced without manual fencing or lease bookkeeping. Multiple independent leaderships can
// coexist by using distinct lock keys.
type Leader struct {
	db   *sql.DB
	key  int64
	mu   sync.Mutex
	conn *sql.Conn // holds the advisory lock while this process is leader
	held bool
}

// NewLeader creates a Leader competing for the advisory lock identified by key. It does not
// attempt to acquire the lock; call TryAcquire periodically.
func NewLeader(db *sql.DB, key int64) *Leader {
	return &Leader{db: db, key: key}
}

// TryAcquire attempts to grab (or confirm) the advisory lock on a pinned connection, without
// blocking. It returns whether this process is the leader after the attempt, i.e. it acquired
// the lock now or still holds it. It is meant to be called periodically; on a healthy leader it
// is a cheap ping, on a follower it retries the lock.
func (l *Leader) TryAcquire(ctx context.Context) bool {
	l.mu.Lock()
	conn, held := l.conn, l.held
	l.mu.Unlock()
	if held {
		if conn != nil && conn.PingContext(ctx) == nil {
			return true // Still leader, connection healthy
		}
		l.Release() // Connection died; the lock is already gone, re-acquire below
	}
	newConn, err := l.db.Conn(ctx)
	if err != nil {
		return false
	}
	var acquired bool
	if err := newConn.QueryRowContext(ctx, tryAdvisoryLockQuery, l.key).Scan(&acquired); err != nil || !acquired {
		newConn.Close()
		return false
	}
	l.mu.Lock()
	l.conn = newConn
	l.held = true
	l.mu.Unlock()
	return true
}

// Release unlocks the advisory lock and returns the pinned connection to the pool.
func (l *Leader) Release() {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.held = false
	l.mu.Unlock()
	if conn != nil {
		// Unlock explicitly before returning the connection to the pool: sql.Conn.Close() returns
		// the physical connection to the pool rather than closing it, so the session-scoped lock
		// would otherwise stay held.
		conn.ExecContext(context.Background(), advisoryUnlockQuery, l.key)
		conn.Close()
	}
}

// IsLeader reports whether this process currently holds the advisory lock.
func (l *Leader) IsLeader() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}
