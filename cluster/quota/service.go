// Package quota tracks per-visitor usage counters across the cluster. Nodes count in memory
// on the hot path and periodically flush increments to a shared per-day table, then read back
// the cluster-wide sums (eventually consistent, bounded by the flush interval). The server
// layers these totals ON TOP of the unchanged per-node limiters: a request must pass both the
// local limiter and the cluster-wide daily limit. On database errors the tracker fails open:
// deltas are retained for retry and totals serve stale, so enforcement degrades to the local
// limiters instead of blocking traffic.
package quota

import (
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
	upsertUsageQuery = `
		INSERT INTO visitor_usage (key, day, requests, messages, emails, calls, bandwidth_bytes, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (key, day) DO UPDATE SET
			requests = visitor_usage.requests + EXCLUDED.requests,
			messages = visitor_usage.messages + EXCLUDED.messages,
			emails = visitor_usage.emails + EXCLUDED.emails,
			calls = visitor_usage.calls + EXCLUDED.calls,
			bandwidth_bytes = visitor_usage.bandwidth_bytes + EXCLUDED.bandwidth_bytes,
			updated_at = EXCLUDED.updated_at
	`
	selectChangedUsageQuery = `SELECT key, requests, messages, emails, calls, bandwidth_bytes FROM visitor_usage WHERE day = $1 AND updated_at >= $2`
	pruneUsageQuery         = `DELETE FROM visitor_usage WHERE day < $1`
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
	now       func() time.Time  // Injectable clock, for tests
	day       string            // Usage day the maps below refer to
	deltas    map[Key]*Counters // Local increments not yet flushed to the database
	totals    map[Key]*Counters // Last known cluster totals, including unflushed local increments
	flushed   map[Key]*Counters // Local increments already flushed to the database this day (peer-delta bookkeeping)
	peerSeen  map[Key]*Counters // Peer consumption already reported via PeerUsageFunc this day
	pulledAt  int64             // updated_at watermark for pulling changed usage rows
	closeChan chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	mu        sync.Mutex // Protects day, deltas, totals, flushed, peerSeen, pulledAt
}

// New creates a tracker on the given (shared) database pool and starts its flush loop
func New(conf *Config, pool *db.DB) (*Tracker, error) {
	if conf.FlushInterval <= 0 {
		conf.FlushInterval = DefaultFlushInterval
	}
	t := &Tracker{
		conf:      conf,
		pool:      pool,
		now:       time.Now,
		deltas:    make(map[Key]*Counters),
		totals:    make(map[Key]*Counters),
		flushed:   make(map[Key]*Counters),
		peerSeen:  make(map[Key]*Counters),
		closeChan: make(chan struct{}),
	}
	t.day = t.currentDay()
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

// Inc records local usage for the given visitor key. In-memory only, safe for the hot path;
// the flush loop persists it as an increment.
func (t *Tracker) Inc(key Key, delta Counters) {
	t.mu.Lock()
	defer t.mu.Unlock()
	addTo(t.deltas, key, delta)
	addTo(t.totals, key, delta)
}

// Totals returns the visitor's cluster-wide usage for the current day, including local
// increments that have not been flushed yet. In-memory only, safe for the hot path.
func (t *Tracker) Totals(key Key) Counters {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c, ok := t.totals[key]; ok {
		return *c
	}
	return Counters{}
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
// and reports peer consumption. On error, deltas are retained and retried on the next
// flush; totals serve stale until then (fail open).
func (t *Tracker) flushAndPull() error {
	// Take the pending deltas and roll the day over if needed. Pending deltas are always
	// flushed under the day they were counted in; the fast-moving maps restart empty.
	t.mu.Lock()
	day := t.day
	pending := t.deltas
	t.deltas = make(map[Key]*Counters)
	if newDay := t.currentDay(); newDay != day {
		t.day = newDay
		t.totals = make(map[Key]*Counters)
		t.flushed = make(map[Key]*Counters)
		t.peerSeen = make(map[Key]*Counters)
		t.pulledAt = 0
	}
	t.mu.Unlock()

	// Push deltas as increments
	now := t.now().Unix()
	for key, c := range pending {
		if c.zero() {
			continue
		}
		if _, err := t.pool.Exec(upsertUsageQuery, string(key), day, c.Requests, c.Messages, c.Emails, c.Calls, c.BandwidthBytes, now); err != nil {
			// Put the deltas back for retry; counts must not be lost on a database hiccup
			t.mu.Lock()
			if t.day == day {
				for k, cc := range pending {
					addTo(t.deltas, k, *cc)
				}
			}
			t.mu.Unlock()
			return err
		}
	}
	t.mu.Lock()
	if t.day == day {
		for k, cc := range pending {
			addTo(t.flushed, k, *cc)
		}
	}
	t.mu.Unlock()
	return t.pull()
}

// pull reads usage rows changed since the last pull, resets the local totals to the database
// sums plus any still-unflushed local increments, and reports peer consumption deltas via
// the configured PeerUsageFunc (outside the tracker lock).
func (t *Tracker) pull() error {
	t.mu.Lock()
	day, since := t.day, t.pulledAt
	t.mu.Unlock()
	pullTime := t.now().Unix()
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
	var peerDeltas map[Key]Counters
	t.mu.Lock()
	if t.day != day {
		t.mu.Unlock()
		return nil // Rolled over while reading; discard
	}
	for key, sum := range sums {
		// Cluster totals: database sums plus local increments that are not in them yet
		total := *sum
		if d, ok := t.deltas[key]; ok {
			total.add(*d)
		}
		t.totals[key] = &total
		// Peer consumption: everything in the database that this node did not flush itself,
		// reported as a delta against what was already reported
		peer := *sum
		if f, ok := t.flushed[key]; ok {
			peer.sub(*f)
		}
		if seen, ok := t.peerSeen[key]; ok {
			peer.sub(*seen)
		}
		if !peer.zero() {
			addTo(t.peerSeen, key, peer)
			if t.conf.PeerUsageFunc != nil {
				if peerDeltas == nil {
					peerDeltas = make(map[Key]Counters)
				}
				peerDeltas[key] = peer
			}
		}
	}
	t.pulledAt = pullTime - 1 // Overlap one second to never miss same-second writers; re-reads are idempotent
	t.mu.Unlock()
	for key, delta := range peerDeltas {
		t.conf.PeerUsageFunc(key, delta)
	}
	return nil
}

// currentDay returns the usage day for the current time
func (t *Tracker) currentDay() string {
	return dayFor(t.now(), t.conf.StatsResetTime)
}

// addTo adds delta to the counters map entry for key, creating it if needed
func addTo(m map[Key]*Counters, key Key, delta Counters) {
	if c, ok := m[key]; ok {
		c.add(delta)
	} else {
		c := delta
		m[key] = &c
	}
}
