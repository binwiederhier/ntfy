package pg

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tagLeader = "leader"

	tryAdvisoryLockQuery = `SELECT pg_try_advisory_lock($1)`
	advisoryUnlockQuery  = `SELECT pg_advisory_unlock($1)`

	defaultRenewInterval = 5 * time.Second
	leaderMissedRenewals = 3
	leaderHoldoffFactor  = 2
)

// Leader implements singleton-job leader election via a Postgres advisory lock held on a
// pinned connection. The lock auto-releases when the holding connection dies, so a crashed
// leader is replaced without manual fencing; distinct keys elect independently. The Leader
// renews its lease on its own loop; callers only ask IsLeader and eventually Close.
//
// Holding the lock is not the same as believing to be the leader: IsLeader also requires a
// recent renewal (lease duration) and a completed hold-off after winning the lock. The
// hold-off outlasts the lease duration by construction, so on failover the old belief always
// expires before the new one begins: a short no-leader gap, never two leaders. Defaults:
// renew every 5s, lease duration 15s, hold-off 30s -> up to ~35s without a leader.
type Leader struct {
	db            *sql.DB
	key           int64
	renewInterval time.Duration
	conn          *sql.Conn          // holds the advisory lock while this process is leader
	acquiredAt    time.Time          // When the lock was won (this tenure), for the hold-off
	renewedAt     time.Time          // Last successful renewal, for the lease duration; zero = lock not held
	cancel        context.CancelFunc // Stops the renew loop and aborts its in-flight query on Close
	closeOnce     sync.Once
	wg            sync.WaitGroup
	mu            sync.Mutex // Protects conn, acquiredAt and renewedAt
}

// NewLeader creates a Leader competing for the lock identified by key and starts its renew
// loop. renewInterval is for tests; pass 0 for the default.
func NewLeader(db *sql.DB, key int64, renewInterval time.Duration) *Leader {
	if renewInterval <= 0 {
		renewInterval = defaultRenewInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &Leader{
		db:            db,
		key:           key,
		renewInterval: renewInterval,
		cancel:        cancel,
	}
	l.wg.Add(1)
	go l.runAcquireOrRenewLoop(ctx)
	return l
}

// IsLeader reports whether this process should act as the leader: lock held, lease renewed
// recently, hold-off elapsed (see the Leader doc comment).
func (l *Leader) IsLeader() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	leaseDuration := leaderMissedRenewals * l.renewInterval
	holdoff := leaderHoldoffFactor * leaseDuration
	return time.Since(l.renewedAt) < leaseDuration && time.Since(l.acquiredAt) >= holdoff
}

// Close stops competing for leadership and releases the lock. Idempotent.
func (l *Leader) Close() {
	l.closeOnce.Do(func() {
		l.cancel() // Also aborts an in-flight renewal query
		l.wg.Wait()
		if l.IsLeader() {
			log.Tag(tagLeader).Info("Lost leadership: closed (lock key %d)", l.key)
		}
		l.release()
	})
}

// runAcquireOrRenewLoop acquires or renews the lock every renewInterval until ctx is canceled
func (l *Leader) runAcquireOrRenewLoop(ctx context.Context) {
	defer l.wg.Done()
	ticker := time.NewTicker(l.renewInterval)
	defer ticker.Stop()
	wasLeader := false
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, l.renewInterval)
		l.tryAcquireOrRenew(attemptCtx)
		cancel()
		if isLeader := l.IsLeader(); isLeader != wasLeader {
			wasLeader = isLeader
			if isLeader {
				log.Tag(tagLeader).Info("Became leader (lock key %d)", l.key)
			} else {
				log.Tag(tagLeader).Info("Lost leadership (lock key %d)", l.key)
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// tryAcquireOrRenew renews the lock on a healthy leader (a cheap ping) or retries acquiring
// it on a follower, on a pinned connection.
func (l *Leader) tryAcquireOrRenew(ctx context.Context) {
	l.mu.Lock()
	conn := l.conn
	l.mu.Unlock()
	if conn != nil {
		if conn.PingContext(ctx) == nil {
			// Still holding the lock, connection healthy: renew the lease
			l.mu.Lock()
			l.renewedAt = time.Now()
			l.mu.Unlock()
			log.Tag(tagLeader).Trace("Renewed leader lease (lock key %d)", l.key)
			return
		}
		log.Tag(tagLeader).Debug("Leader lock connection died, lock lost (lock key %d)", l.key)
		l.release() // Connection died; the lock is already gone, re-acquire below
	}
	newConn, err := l.db.Conn(ctx)
	if err != nil {
		log.Tag(tagLeader).Debug("Cannot get connection to compete for leader lock (lock key %d): %s", l.key, err.Error())
		return
	}
	var acquired bool
	if err := newConn.QueryRowContext(ctx, tryAdvisoryLockQuery, l.key).Scan(&acquired); err != nil || !acquired {
		newConn.Close()
		log.Tag(tagLeader).Trace("Leader lock held elsewhere (lock key %d)", l.key)
		return
	}
	log.Tag(tagLeader).Debug("Acquired leader lock (lock key %d); leadership after the hold-off", l.key)
	l.mu.Lock()
	l.conn = newConn
	l.acquiredAt = time.Now()
	l.renewedAt = l.acquiredAt
	l.mu.Unlock()
}

// release unlocks the advisory lock and returns the pinned connection to the pool
func (l *Leader) release() {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.renewedAt = time.Time{} // Zero revokes belief; without it, IsLeader would linger a lease duration
	l.mu.Unlock()
	if conn != nil {
		// Unlock explicitly: sql.Conn.Close() returns the connection to the pool, so the
		// session-scoped lock would otherwise stay held
		conn.ExecContext(context.Background(), advisoryUnlockQuery, l.key)
		conn.Close()
		log.Tag(tagLeader).Debug("Released leader lock (lock key %d)", l.key)
	}
}
