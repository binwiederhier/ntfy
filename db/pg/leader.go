package pg

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tagLeader = "leader"

	tryAdvisoryLockQuery = `SELECT pg_try_advisory_lock($1)`
	currentSchemaQuery   = `SELECT current_schema()`

	// renewQuery confirms that this session still holds the lock, on a primary. Advisory locks
	// are not replicated, so after a failover the old primary (if still reachable) answers pings
	// but no longer holds our lock; a bare ping would keep us leader next to the new one.
	renewQuery = `
		SELECT NOT pg_is_in_recovery() AND EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory' AND pid = pg_backend_pid() AND classid = $1::oid AND objid = $2::oid AND granted
		)
	`
	// fenceQueryFormat makes the server drop the pinned session, and with it the lock, if it
	// goes silent (packets black-holed, VM paused). Without it, Postgres keeps an idle session
	// until TCP keepalives give up, hours with stock settings. SET takes no parameters, so the
	// milliseconds are formatted in. Requires PostgreSQL 14; older servers run without the fence.
	fenceQueryFormat = `SET idle_session_timeout = %d`

	defaultRenewInterval = 5 * time.Second
	leaderMissedRenewals = 3 // Lease duration = renewInterval x this
	leaderHoldoffFactor  = 2 // Hold-off = lease duration x this; must be > 1 so the old belief expires first
	leaderFenceFactor    = 2 // idle_session_timeout = lease duration x this; must be > 1 so a healthy leader is never fenced
)

// Leader implements singleton-job leader election via a Postgres advisory lock held on a pinned
// connection, renewed on its own loop. Callers only ask IsLeader and eventually Close.
//
// Holding the lock is not the same as believing to be the leader: IsLeader also requires a
// recent renewal (lease duration) and a completed hold-off after winning the lock. The hold-off
// outlasts the lease by construction, so the old belief always expires before the new one
// begins: a short no-leader gap, never two leaders. Renewal checks the lock itself, not just
// the connection, so a database failover deposes the old leader too.
//
// Defaults: renew 5s, lease 15s, hold-off 30s, so a crashed leader is replaced after ~35s. A
// leader that goes silent rather than crashing keeps its session, and the lock, until the
// server's idle_session_timeout (set to 30s on that session) fences it: ~65s.
//
// What this does and does not give you:
//
//   - A singleton job must finish well inside the lease or re-check IsLeader per unit of work,
//     because a deposed leader believes for up to one lease. Leadership is not a fence: a job
//     with side effects that must not run twice needs its own coordination (a claim row, or
//     SELECT ... FOR UPDATE SKIP LOCKED) or needs to be idempotent.
//   - The connection must be direct. A transaction-pooling proxy (PgBouncer in transaction
//     mode) silently breaks session advisory locks.
//   - One authoritative primary is assumed, enforced by whatever runs the database. The
//     renewal's pg_is_in_recovery check deposes a leader on a server that knows it is a
//     standby; it cannot fence a split-brain server that still believes it is the primary.
//   - The stated takeover time for a silent leader assumes the idle timeout was set on its
//     session, which needs PostgreSQL 14. The acquire path logs when that SET fails and the
//     fence then falls back to TCP keepalives, which can take hours.
type Leader struct {
	db            *sql.DB
	key           int64 // Base key; scoped to the schema on the first acquire attempt (see scopeKeyToSchema)
	keyScoped     bool
	renewInterval time.Duration
	conn          *sql.Conn          // Holds the advisory lock while this process is leader
	acquiredAt    time.Time          // When the lock was won (this tenure), for the hold-off
	renewedAt     time.Time          // Last successful renewal, for the lease duration; zero = lock not held
	cancel        context.CancelFunc // Stops the renew loop and aborts its in-flight query on Close
	closeOnce     sync.Once
	wg            sync.WaitGroup
	mu            sync.Mutex // Protects key, keyScoped, conn, acquiredAt and renewedAt
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
	return time.Since(l.renewedAt) < l.leaseDuration() && time.Since(l.acquiredAt) >= l.holdoff()
}

// Key returns the lock key as Postgres holds it, i.e. scoped to the schema (see
// scopeKeyToSchema). In pg_locks it shows as classid = high 32 bits, objid = low 32 bits.
func (l *Leader) Key() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.key
}

// Close stops competing for leadership and releases the lock. Idempotent.
func (l *Leader) Close() {
	l.closeOnce.Do(func() {
		l.cancel() // Also aborts an in-flight renewal query
		l.wg.Wait()
		if l.IsLeader() {
			log.Tag(tagLeader).Info("Lost leadership: closed (lock key %d)", l.Key())
		}
		l.release()
	})
}

func (l *Leader) leaseDuration() time.Duration {
	return leaderMissedRenewals * l.renewInterval
}

func (l *Leader) holdoff() time.Duration {
	return leaderHoldoffFactor * l.leaseDuration()
}

// runAcquireOrRenewLoop acquires or renews the lock every renewInterval until ctx is canceled
func (l *Leader) runAcquireOrRenewLoop(ctx context.Context) {
	defer l.wg.Done()
	log.Tag(tagLeader).Debug("Competing for leader lock (base key %d)", l.Key())
	defer log.Tag(tagLeader).Debug("Stopped competing for leader lock (lock key %d)", l.Key())
	ticker := time.NewTicker(l.renewInterval)
	defer ticker.Stop()
	wasLeader := false
	for {
		// An attempt may take up to a lease: a single slow renewal must not count as a dead
		// connection, and belief expires on its own if it takes longer than that anyway
		attemptCtx, cancel := context.WithTimeout(ctx, l.leaseDuration())
		l.tryAcquireOrRenew(attemptCtx)
		cancel()
		if isLeader := l.IsLeader(); isLeader != wasLeader {
			wasLeader = isLeader
			if isLeader {
				log.Tag(tagLeader).Info("Became leader (lock key %d)", l.Key())
			} else {
				log.Tag(tagLeader).Info("Lost leadership (lock key %d)", l.Key())
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// tryAcquireOrRenew renews the lock on a healthy leader (one cheap query) or retries acquiring
// it on a follower, on a pinned connection
func (l *Leader) tryAcquireOrRenew(ctx context.Context) {
	l.mu.Lock()
	conn, key := l.conn, l.key
	l.mu.Unlock()
	if conn != nil {
		// The lease is dated from when the renewal was issued, not when its result was processed:
		// a goroutine paused between the two holds stale evidence, and meanwhile the server may
		// have fenced the session and a follower may lead
		renewalStarted := time.Now()
		var held bool
		if err := conn.QueryRowContext(ctx, renewQuery, classID(key), objID(key)).Scan(&held); err != nil {
			log.Tag(tagLeader).Debug("Leader lock connection died, lock lost (lock key %d): %s", key, err.Error())
			l.release() // Re-acquire below
		} else if !held {
			log.Tag(tagLeader).Info("Leader lock no longer held by this session (database failover?) (lock key %d)", key)
			l.release() // The lock is gone, or the server is a standby now
		} else {
			l.mu.Lock()
			l.renewedAt = renewalStarted
			l.mu.Unlock()
			log.Tag(tagLeader).Trace("Renewed leader lease (lock key %d)", key)
			return
		}
	}
	newConn, err := l.db.Conn(ctx)
	if err != nil {
		log.Tag(tagLeader).Debug("Cannot get connection to compete for leader lock (lock key %d): %s", key, err.Error())
		return
	}
	if err := l.scopeKeyToSchema(ctx, newConn); err != nil {
		newConn.Close()
		log.Tag(tagLeader).Warn("Cannot scope leader lock key to schema, not competing: %s", err.Error())
		return
	}
	key = l.Key()
	var acquired bool
	if err := newConn.QueryRowContext(ctx, tryAdvisoryLockQuery, key).Scan(&acquired); err != nil || !acquired {
		newConn.Close()
		log.Tag(tagLeader).Trace("Leader lock held elsewhere (lock key %d)", key)
		return
	}
	// Fence the session before relying on it: a silent leader must be dropped server-side
	fence := fmt.Sprintf(fenceQueryFormat, (leaderFenceFactor * l.leaseDuration()).Milliseconds())
	if _, err := newConn.ExecContext(ctx, fence); err != nil {
		// Warn, not debug: this silently stretches the takeover time for a silent leader from
		// ~65s to however long TCP keepalives take, which is hours with stock settings
		log.Tag(tagLeader).Err(err).Warn("Cannot set idle_session_timeout on the leader lock session (PostgreSQL < 14?); a silent leader is then only fenced by TCP keepalives")
	}
	log.Tag(tagLeader).Debug("Acquired leader lock (lock key %d, classid %d, objid %d); leadership after the hold-off", key, classID(key), objID(key))
	l.mu.Lock()
	l.conn = newConn
	l.acquiredAt = time.Now()
	l.renewedAt = l.acquiredAt
	l.mu.Unlock()
}

// scopeKeyToSchema folds the connection's current schema into the lock key, once. Advisory
// locks are database-global, but a cluster is defined by its schema: every real node runs with
// the same search_path (public), while test runs and multi-tenant setups use distinct schemas
// and must elect independently instead of stealing each other's leadership. The schema is read
// from the first connection; every connection in the pool shares the DSN, so they agree.
func (l *Leader) scopeKeyToSchema(ctx context.Context, conn *sql.Conn) error {
	l.mu.Lock()
	scoped := l.keyScoped
	l.mu.Unlock()
	if scoped {
		return nil
	}
	// Query outside the lock: IsLeader must not stall behind a slow or dead database
	var schema sql.NullString
	if err := conn.QueryRowContext(ctx, currentSchemaQuery).Scan(&schema); err != nil {
		return err
	} else if !schema.Valid {
		return fmt.Errorf("no current schema (does search_path name an existing schema?)")
	}
	h := fnv.New64a()
	h.Write([]byte(schema.String))
	l.mu.Lock()
	l.key ^= int64(h.Sum64()) // Only the renew loop gets here, so this cannot run twice
	l.keyScoped = true
	l.mu.Unlock()
	return nil
}

// release gives up the pinned connection, which releases the lock: the session ends, and a
// session-scoped advisory lock dies with its session. The connection is discarded rather than
// returned to the pool (Close alone would return it, lock and idle timeout included).
//
// This issues no query and cannot fail, but it is bounded rather than instant: the driver sends
// a Terminate message on close, under a 5s deadline of its own, so a transport that cannot even
// accept a write delays release by up to that long. Closing the local socket also does not
// prove the server ended the session; that is what the idle timeout fence is for.
func (l *Leader) release() {
	l.mu.Lock()
	conn, key := l.conn, l.key
	l.conn = nil
	l.renewedAt = time.Time{} // Zero revokes belief; without it, IsLeader would linger a lease duration
	l.mu.Unlock()
	if conn == nil {
		return
	}
	conn.Raw(func(any) error { return driver.ErrBadConn }) // Marks it bad, so Close discards the session
	conn.Close()
	log.Tag(tagLeader).Debug("Released leader lock (lock key %d)", key)
}

// classID and objID split a 64-bit advisory lock key the way pg_locks shows it
func classID(key int64) int64 {
	return int64(uint32(uint64(key) >> 32))
}

func objID(key int64) int64 {
	return int64(uint32(uint64(key)))
}
