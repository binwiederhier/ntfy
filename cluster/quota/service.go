// Package quota tracks per-visitor usage counters across the cluster. Nodes count in memory
// on the hot path and periodically flush increments to a shared per-day table, then read back
// the cluster-wide sums (eventually consistent, bounded by the flush interval). The server
// layers these totals ON TOP of the unchanged per-node limiters: a request must pass both the
// local limiter and the cluster-wide daily limit. On database errors the tracker fails open:
// deltas are retained for retry and totals serve stale, so enforcement degrades to the local
// limiters instead of blocking traffic.
package quota

import (
	"context"
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
	"heckel.io/ntfy/v2/log"
)

const (
	tag            = "quota"
	schemaStoreKey = "visitor_usage" // Store name in the shared schema_version table (see db/schema)
	schemaVersion  = 1

	// DefaultFlushInterval is the cadence for pushing counter deltas and pulling cluster
	// totals. It bounds quota overshoot (~nodes x interval x per-node rate) and the latency
	// until one node notices another node's consumption. Constant by design (all nodes must
	// roughly agree, and it is not worth a config knob).
	DefaultFlushInterval = 30 * time.Second

	// retention is how long old per-day usage rows are kept before the leader prunes them
	retention = 7 * 24 * time.Hour

	// flushTimeout bounds one usage upsert. Rows are stamped with the database clock at the
	// start of their statement, so a row's updated_at lags its commit by at most this much;
	// pullOverlap re-reads that far behind the watermark so a slow writer is never skipped.
	flushTimeout = 5 * time.Second
	pullOverlap  = flushTimeout + time.Second
)

const (
	createTable = `
		CREATE TABLE IF NOT EXISTS visitor_usage (
			key TEXT NOT NULL,
			day TEXT NOT NULL,
			requests BIGINT NOT NULL DEFAULT 0,
			messages BIGINT NOT NULL DEFAULT 0,
			emails BIGINT NOT NULL DEFAULT 0,
			calls BIGINT NOT NULL DEFAULT 0,
			bandwidth_bytes BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (key, day)
		);
		CREATE INDEX IF NOT EXISTS idx_visitor_usage_updated_at ON visitor_usage (updated_at);
	`
	// Timestamps come from the database clock, never from a node's: the watermark below compares
	// them, and node clocks neither agree with each other nor with the database
	upsertUsageQuery = `
		INSERT INTO visitor_usage (key, day, requests, messages, emails, calls, bandwidth_bytes, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, EXTRACT(EPOCH FROM now())::BIGINT)
		ON CONFLICT (key, day) DO UPDATE SET
			requests = visitor_usage.requests + EXCLUDED.requests,
			messages = visitor_usage.messages + EXCLUDED.messages,
			emails = visitor_usage.emails + EXCLUDED.emails,
			calls = visitor_usage.calls + EXCLUDED.calls,
			bandwidth_bytes = visitor_usage.bandwidth_bytes + EXCLUDED.bandwidth_bytes,
			updated_at = EXCLUDED.updated_at
	`
	selectChangedUsageQuery  = `SELECT key, requests, messages, emails, calls, bandwidth_bytes FROM visitor_usage WHERE day = $1 AND updated_at >= $2`
	selectDatabaseClockQuery = `SELECT EXTRACT(EPOCH FROM now())::BIGINT`
	pruneUsageQuery          = `DELETE FROM visitor_usage WHERE day < $1`
)

// Config is the tracker configuration. FlushInterval defaults to DefaultFlushInterval;
// StatsResetTime carries the server's visitor-stats-reset-time (wall clock; the usage day
// rolls over at that time). PeerUsageFunc, if set, is called after each pull with the usage
// other nodes consumed for a key since the last pull (used to burn down local token buckets);
// it is called without the tracker lock held.
type Config struct {
	FlushInterval  time.Duration
	StatsResetTime time.Time
	PeerUsageFunc  func(key Key, peerDelta Counters)
}

// Tracker tracks per-visitor usage counters cluster-wide. See the package comment.
type Tracker struct {
	conf      *Config
	pool      *db.DB
	now       func() time.Time // Injectable clock, for tests
	day       string           // Usage day the entries below refer to
	entries   map[Key]*entry   // Per-key state; one entry per key seen anywhere in the cluster today
	pulledAt  int64            // updated_at watermark for pulling changed usage rows
	closing   bool             // Set in Close; the final pull skips PeerUsageFunc (the server is shutting down and may hold its own locks)
	closeChan chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	mu        sync.Mutex // Protects day, entries, pulledAt, closing
}

// New creates a tracker on the given (shared) database pool and starts its flush loop
func New(conf *Config, pool *db.DB) (*Tracker, error) {
	if conf.FlushInterval <= 0 {
		conf.FlushInterval = DefaultFlushInterval
	}
	t := newTracker(conf, pool)
	if err := schema.Migrate(pool.Primary(), schema.Postgres, schemaStoreKey, schemaVersion, schema.AsMigrateFunc(createTable), nil); err != nil {
		return nil, err
	}
	// Warm the totals cache so a freshly booted node enforces existing usage right away
	if err := t.flushAndPull(); err != nil {
		return nil, err
	}
	t.wg.Add(1)
	go t.runFlushLoop()
	return t, nil
}

// entry is the per-key state. One entry per key replaces the four parallel maps this used to
// keep (deltas, totals, flushed, peerSeen): every node holds state for every key with usage
// anywhere in the cluster that day, so the per-key footprint is a scaling property.
//
// baseline is the database sum as of the last pull, so the cluster total is baseline plus the
// local increments not yet written, and usage consumed by peers since the last pull is
// whatever the database grew beyond the baseline.
type entry struct {
	unflushed Counters
	baseline  Counters
}

// newTracker builds a tracker with its in-memory state initialized (no database work)
func newTracker(conf *Config, pool *db.DB) *Tracker {
	t := &Tracker{
		conf:      conf,
		pool:      pool,
		now:       time.Now,
		entries:   make(map[Key]*entry),
		closeChan: make(chan struct{}),
	}
	t.day = t.currentDay()
	return t
}

// Inc records local usage for the given visitor key. In-memory only, safe for the hot path;
// the flush loop persists it as an increment.
func (t *Tracker) Inc(key Key, delta Counters) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entryFor(key).unflushed.Add(delta)
}

// entryFor returns the key's entry, creating it if needed. The caller must hold t.mu.
func (t *Tracker) entryFor(key Key) *entry {
	e, ok := t.entries[key]
	if !ok {
		e = &entry{}
		t.entries[key] = e
	}
	return e
}

// Totals returns the visitor's cluster-wide usage for the current day, including local
// increments that have not been flushed yet. In-memory only, safe for the hot path.
func (t *Tracker) Totals(key Key) Counters {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[key]
	if !ok {
		return Counters{}
	}
	total := e.baseline
	total.Add(e.unflushed)
	return total
}

// Prune deletes usage rows older than the retention. Only the cluster leader calls this
// (like the other singleton database jobs).
func (t *Tracker) Prune() error {
	cutoffDay := dayFor(t.now().Add(-retention), t.conf.StatsResetTime)
	_, err := t.pool.Exec(pruneUsageQuery, cutoffDay)
	return err
}

// Close stops the flush loop and flushes remaining deltas; safe to call more than once
func (t *Tracker) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closing = true
		t.mu.Unlock()
		close(t.closeChan)
		t.wg.Wait()
		err = t.flushAndPull()
	})
	return err
}

// runFlushLoop periodically pushes deltas and pulls totals until Close
func (t *Tracker) runFlushLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.conf.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := t.flushAndPull(); err != nil {
				log.Tag(tag).Err(err).Warn("Usage tracker flush failed")
			}
		case <-t.closeChan:
			return
		}
	}
}

// flushAndPull pushes pending deltas, handles day rollover, pulls changed cluster totals,
// and reports peer consumption. Increments stay in unflushed until their row is written, so
// Totals never dips while a flush is in flight, and a failed write simply leaves its key's
// increments where they are for the next flush (fail open: totals serve stale until then).
func (t *Tracker) flushAndPull() error {
	// Snapshot the pending increments and roll the day over if needed. Increments are always
	// flushed under the day they were counted in; a new day starts with empty entries.
	t.mu.Lock()
	day := t.day
	pending := make(map[Key]Counters, len(t.entries))
	for key, e := range t.entries {
		if !e.unflushed.zero() {
			pending[key] = e.unflushed
		}
	}
	if newDay := t.currentDay(); newDay != day {
		t.day = newDay
		t.entries = make(map[Key]*entry)
		t.pulledAt = 0
	}
	t.mu.Unlock()

	// Push the snapshot as increments, one row at a time. Each written row moves from unflushed
	// to the baseline right away, so a later failure in the same flush cannot write it twice;
	// the keys after the failure keep their increments and are retried on the next flush.
	for key, c := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		_, err := t.pool.ExecContext(ctx, upsertUsageQuery, string(key), day, c.Requests, c.Messages, c.Emails, c.Calls, c.BandwidthBytes)
		cancel()
		if err != nil {
			return err
		}
		t.mu.Lock()
		if t.day == day { // After a rollover the old day's entries are gone; the row is still correct
			e := t.entryFor(key)
			e.unflushed.sub(c) // Only the snapshot: increments counted since stay pending
			e.baseline.Add(c)
		}
		t.mu.Unlock()
	}
	return t.pull()
}

// pull reads usage rows changed since the last pull, resets the local totals to the database
// sums plus any still-unflushed local increments, and reports peer consumption deltas via
// the configured PeerUsageFunc (outside the tracker lock).
func (t *Tracker) pull() error {
	t.mu.Lock()
	day, since := t.day, t.pulledAt
	t.mu.Unlock()
	// The watermark is the database clock, read before the rows: everything committed after
	// this read has a later updated_at (minus at most flushTimeout, covered by pullOverlap)
	var dbNow int64
	if err := t.pool.QueryRow(selectDatabaseClockQuery).Scan(&dbNow); err != nil {
		return err
	}
	rows, err := t.pool.Query(selectChangedUsageQuery, day, since) // Deliberately the primary: the pull must see this node's own just-flushed rows (replicas may lag)
	if err != nil {
		return err
	}
	defer rows.Close()
	sums := make(map[Key]*Counters)
	for rows.Next() {
		var key string
		c := &Counters{}
		if err := rows.Scan(&key, &c.Requests, &c.Messages, &c.Emails, &c.Calls, &c.BandwidthBytes); err != nil {
			return err
		}
		sums[Key(key)] = c
	}
	if err := rows.Err(); err != nil {
		return err
	}
	peerDeltas := t.apply(day, sums, dbNow)
	for key, delta := range peerDeltas {
		t.conf.PeerUsageFunc(key, delta)
	}
	return nil
}

// apply folds the database sums read by pull into the in-memory state and returns the usage
// peers consumed since the last pull. Split out so it can be exercised without a database.
func (t *Tracker) apply(day string, sums map[Key]*Counters, dbNow int64) map[Key]Counters {
	var peerDeltas map[Key]Counters
	t.mu.Lock()
	if t.day != day {
		t.mu.Unlock()
		return nil // Rolled over while reading; discard
	}
	for key, sum := range sums {
		e := t.entryFor(key)
		peer := *sum
		peer.sub(e.baseline) // Everything the database grew beyond our baseline came from peers
		e.baseline = *sum
		if !peer.zero() && t.conf.PeerUsageFunc != nil && !t.closing {
			if peerDeltas == nil {
				peerDeltas = make(map[Key]Counters)
			}
			peerDeltas[key] = peer
		}
	}
	t.pulledAt = dbNow - int64(pullOverlap.Seconds()) // Re-reads are idempotent (sums, not deltas)
	t.mu.Unlock()
	return peerDeltas
}

// currentDay returns the usage day for the current time
func (t *Tracker) currentDay() string {
	return dayFor(t.now(), t.conf.StatsResetTime)
}
