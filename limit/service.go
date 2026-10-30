// Package limit answers one question: may key K use N units of kind X? A key is a visitor
// identity (see package visitor), a kind is what is being counted (requests, messages, bytes, ...),
// and a Rule, passed in on every call, says how much is allowed: a token bucket, a daily cap, or
// both. Rules are not baked into the state, so a changed rule (a tier change) applies on the next
// call without rebuilding anything and without handing back a full bucket.
//
// The service knows nothing about users, tiers or HTTP. All decisions are made from memory,
// sharded by key: token buckets per node, and daily counts (messages, emails, calls) per key and
// 24h period. A key's period starts at an offset hashed from the key (see periodStart), so the
// resets of all keys are spread over the day, lazily, on the key's next use.
//
// With a store, a sync worker writes what this node admitted to the shared usage table every
// syncInterval and reads back what changed, so a key's count is base (the row as last read: all
// nodes) plus unflushed (admitted here since the last write). At startup the service loads the
// current rows. Store failures never reach the request path: counts stay unflushed and are
// written once the store is back (fail open, per-node limits meanwhile).
package limit

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

const (
	tag        = "limit"
	shardCount = 64 // Keys are spread over this many independently locked maps

	// syncInterval is how often a node writes its unflushed counts and reads the others'. A count
	// reaches the other nodes within about two intervals in healthy operation (one to be written,
	// one to be read), which bounds the overshoot in a cluster to about (nodes - 1) x what a key
	// is admitted in that time.
	syncInterval = 5 * time.Second

	// statementTimeout bounds each store call of a sync. A row's updated_at is the database
	// time its statement started, so it may commit up to this much later; reads re-read
	// readOverlap behind the watermark so such a row is not skipped (rows are totals, so a
	// re-read is harmless).
	statementTimeout = 5 * time.Second
	readOverlap      = statementTimeout + time.Second

	// loadWait is how long Start waits for the startup load before serving from memory, while
	// the load goes on in the background; loadTimeout bounds one load attempt
	loadWait    = 30 * time.Second
	loadTimeout = 2 * time.Minute

	// pruneInterval is how often a node deletes rows not written for pruneAfter; a row older
	// than a period is never current again
	pruneInterval = time.Hour
	pruneAfter    = 25 * time.Hour
)

// Service decides on usage from memory; see the package comment
type Service struct {
	conf        *Config
	now         func() time.Time          // Injectable clock, for tests
	counters    [shardCount]*counterShard // Daily counts, kept until their period ends
	buckets     [shardCount]*bucketShard  // Token buckets, per node, evicted when idle and full
	store       *Store                    // Nil: memory only
	watermark   int64                     // Database time of the last read; the sync worker's only
	prunedAt    int64                     // Database time of the last prune; the sync worker's only
	clockOffset atomic.Int64              // Database clock minus ours, nanoseconds
	interval    time.Duration             // How often the worker syncs (syncInterval); tests change it
	ctx         context.Context           // Done on Close; the sync worker's store calls derive from it, so Close never waits on a stuck one
	cancel      context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

// counterShard is one independently locked part of the daily counters
type counterShard struct {
	counters map[string]*counter
	mu       sync.Mutex // Protects counters and everything inside them
}

// bucketShard is one independently locked part of the token buckets
type bucketShard struct {
	entries map[string]*bucketEntry
	mu      sync.Mutex // Protects entries and everything inside them
}

// New creates the service
func New(conf *Config) *Service {
	return newService(conf, time.Now)
}

func newService(conf *Config, now func() time.Time) *Service {
	s := &Service{
		conf:     conf,
		now:      now,
		store:    conf.Store,
		interval: syncInterval,
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	for i := 0; i < shardCount; i++ {
		s.counters[i] = &counterShard{
			counters: make(map[string]*counter),
		}
		s.buckets[i] = &bucketShard{
			entries: make(map[string]*bucketEntry),
		}
	}
	return s
}

// Allow decides whether the key may use n units of the rule's kind now, and if so, uses them.
// Both the bucket (if the rule has one) and the daily cap (if it has one) must have room. The
// bucket is charged first and handed back if the daily cap denies, so the two locks are taken
// one after the other, never nested.
func (s *Service) Allow(key string, rule Rule, n int64) Decision {
	now := s.now()
	if d := s.takeTokens(key, &rule, n, now); !d.OK {
		return d
	}
	i := dailyIndex(rule.Kind)
	if i < 0 {
		return Decision{OK: true}
	}
	dbNow := s.dbTime(now)
	start := periodStart(key, dbNow)
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	c := cs.counterNoLock(key, start)
	total := c.base[i] + c.unflushed[i]
	if rule.Daily > 0 && total+n > rule.Daily {
		end := time.Unix(c.periodStart+daySeconds, 0) // Read under the lock: another call may roll the counter over
		cs.mu.Unlock()
		if rule.Burst > 0 {
			s.refundTokens(key, rule.Kind, n)
		}
		return Decision{RetryAfter: end.Sub(dbNow)}
	}
	c.unflushed[i] += n
	if rule.Threshold == 0 || total+n >= rule.Threshold {
		c.promoted = true
	}
	cs.mu.Unlock()
	return Decision{OK: true}
}

// Start loads the current counts from the store, if there is one, and starts the sync worker.
// It returns once the load is done, or after loadWait (or when ctx is done), whichever is first;
// the load then goes on in the background while the service decides from memory.
func (s *Service) Start(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	loaded := make(chan struct{})
	s.wg.Add(1)
	go s.runSync(loaded)
	select {
	case <-loaded:
	case <-time.After(loadWait):
		log.Tag(tag).Warn("Loading usage takes longer than %v; serving from memory, loading in the background", loadWait)
	case <-ctx.Done():
		log.Tag(tag).Warn("Loading usage was not done in time; serving from memory, loading in the background")
	}
	return nil
}

// Close stops the sync worker, cancelling its store call (a stuck load or sync), and writes what
// is unflushed. The store's database is its owner's to close (the user manager's, or the shared
// pool), after this.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
		if s.store == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
		defer cancel()
		s.write(ctx)
	})
}

// Tokens returns the whole tokens the key's bucket for the rule's kind has right now, without
// using any. It is what a log line or a "may I try" check wants; Allow is what consumes.
func (s *Service) Tokens(key string, rule Rule) int64 {
	if rule.Burst <= 0 {
		return 0
	}
	now := s.now()
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	e, ok := bs.entries[key]
	if !ok {
		return rule.Burst // A fresh bucket is full; no need to create it to say so
	}
	b := &e.buckets[rule.Kind]
	b.refill(now, &rule)
	return int64(math.Max(0, math.Floor(b.tokens)))
}

// Refund hands n units of the kind back to the key, e.g. for bytes that were reserved but not
// sent. The bucket cannot exceed its burst, and the daily count cannot go below zero.
func (s *Service) Refund(key string, kind Kind, n int64) {
	s.refundTokens(key, kind, n)
	start := periodStart(key, s.dbTime(s.now()))
	i := dailyIndex(kind)
	if i < 0 {
		bs := s.buckets[shardIndex(key)]
		bs.mu.Lock()
		defer bs.mu.Unlock()
		if e, ok := bs.entries[key]; ok && e.periodStart >= start {
			e.counts[kind] = max(0, e.counts[kind]-n)
		}
		return
	}
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok && c.periodStart >= start {
		c.unflushed[i] = max(-c.base[i], c.unflushed[i]-n) // A negative unflushed is a refund still to be written
	}
}

// Usage returns the key's counts in its current period, from memory: the daily counts (all
// nodes' as of the last sync, plus this node's since), and this node's units of the other kinds
func (s *Service) Usage(key string) Usage {
	start := periodStart(key, s.dbTime(s.now()))
	var u Usage
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	if e, ok := bs.entries[key]; ok && e.periodStart >= start {
		u.counts = e.counts
	}
	bs.mu.Unlock()
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok && c.periodStart >= start {
		for i, kind := range dailyKinds {
			u.counts[kind] = c.base[i] + c.unflushed[i]
		}
	}
	return u
}

// ResetAt returns when the key's daily counts start over: the end of its current period
func (s *Service) ResetAt(key string) time.Time {
	return periodEnd(key, s.dbTime(s.now()))
}

// Prune forgets the buckets of keys that have not been used for longer than idle and are full
// again (a forgotten key comes back with full buckets), and the daily counts of periods that
// have ended. It returns how many keys' buckets it forgot.
func (s *Service) Prune(idle time.Duration) int {
	now := s.now()
	cutoff := now.Add(-idle).UnixNano()
	pruned := 0
	for _, bs := range s.buckets {
		bs.mu.Lock()
		for key, e := range bs.entries {
			if e.seen < cutoff && now.UnixNano() >= e.full {
				delete(bs.entries, key)
				pruned++
			}
		}
		bs.mu.Unlock()
	}
	dbNow := s.dbTime(now)
	for _, cs := range s.counters {
		cs.mu.Lock()
		for key, c := range cs.counters {
			if c.periodStart < periodStart(key, dbNow) {
				delete(cs.counters, key)
			}
		}
		cs.mu.Unlock()
	}
	return pruned
}

// Touch marks the key as used now, so Prune keeps it, e.g. for a long-lived subscription
func (s *Service) Touch(key string) {
	now := s.now()
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.entryNoLock(key, now).seen = now.UnixNano()
}

// Seen returns when the key was last used, or the zero time if the service does not know it
func (s *Service) Seen(key string) time.Time {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	e, ok := bs.entries[key]
	if !ok {
		return time.Time{}
	}
	return time.Unix(0, e.seen).UTC()
}

// Limiter returns a util.Limiter view of the key's bucket for the rule, for the readers and
// writers in util that meter bytes. A negative AllowN refunds.
func (s *Service) Limiter(key string, rule Rule) util.Limiter {
	return &limiter{
		s:    s,
		key:  key,
		rule: rule,
	}
}

// Size returns the number of keys the service holds buckets for, i.e. the keys used recently
func (s *Service) Size() int {
	size := 0
	for _, bs := range s.buckets {
		bs.mu.Lock()
		size += len(bs.entries)
		bs.mu.Unlock()
	}
	return size
}

// runSync loads the current counts, retrying until it succeeds, then syncs every interval until
// Close. It is the only caller of the store, so writes and reads never interleave.
func (s *Service) runSync(loaded chan struct{}) {
	defer s.wg.Done()
	for !s.load() {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(s.interval):
		}
	}
	close(loaded)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sync()
		case <-s.ctx.Done():
			return
		}
	}
}

// load reads every row written in the last day and keeps those of their key's current period,
// and reports whether it succeeded
func (s *Service) load() bool {
	ctx, cancel := context.WithTimeout(s.ctx, loadTimeout)
	defer cancel()
	started := time.Now()
	dbNow, err := s.syncClock(ctx)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot load usage; retrying in %v", s.interval)
		return false
	}
	n, err := s.read(ctx, dbNow-daySeconds)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot load usage; retrying in %v", s.interval)
		return false
	}
	s.watermark, s.prunedAt = dbNow, dbNow
	log.Tag(tag).Info("Loaded usage of %d key(s) in %v", n, time.Since(started).Round(time.Millisecond))
	return true
}

// sync is one round of the sync worker: measure the database clock, write the unflushed counts
// of the promoted keys, read back the rows changed since the last round, and prune once in a
// while. Every error is logged and leaves the state as it was, for the next round.
func (s *Service) sync() {
	ctx, cancel := context.WithTimeout(s.ctx, statementTimeout)
	dbNow, err := s.syncClock(ctx)
	cancel()
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot read the database clock; usage is not synced")
		return
	}
	ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
	s.write(ctx)
	cancel()
	ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
	_, err = s.read(ctx, s.watermark-int64(readOverlap/time.Second))
	cancel()
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot read usage; counts of other nodes are stale until the next round")
	} else {
		s.watermark = dbNow
	}
	if dbNow-s.prunedAt >= int64(pruneInterval/time.Second) {
		s.prunedAt = dbNow
		ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
		deleted, err := s.store.prune(ctx, dbNow-int64(pruneAfter/time.Second))
		cancel()
		if err != nil {
			log.Tag(tag).Err(err).Warn("Cannot prune usage")
		} else {
			log.Tag(tag).Debug("Pruned %d usage row(s)", deleted)
		}
	}
}

// syncClock reads the database clock and remembers its offset from ours
func (s *Service) syncClock(ctx context.Context) (int64, error) {
	dbNow, err := s.store.clock(ctx)
	if err != nil {
		return 0, err
	}
	s.clockOffset.Store(int64(time.Unix(dbNow, 0).Sub(s.now())))
	return dbNow, nil
}

// write writes the unflushed counts of the promoted keys. A written row moves its counts from
// unflushed to base, so the total does not change and nothing is written twice; the counts of a
// failed chunk stay unflushed for the next round.
func (s *Service) write(ctx context.Context) {
	rows := s.unflushedRows()
	if len(rows) == 0 {
		return
	}
	written, err := s.store.write(ctx, rows)
	for i := range rows[:written] {
		s.retire(&rows[i])
	}
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write usage, %d of %d key(s) written; the rest is retried", written, len(rows))
	}
}

// unflushedRows returns the unflushed counts of the promoted keys, each under its counter's period
func (s *Service) unflushedRows() []usageRow {
	rows := make([]usageRow, 0)
	for _, cs := range s.counters {
		cs.mu.Lock()
		for key, c := range cs.counters {
			if c.promoted && c.unflushed != [numDaily]int64{} {
				rows = append(rows, usageRow{
					key:         key,
					periodStart: c.periodStart,
					counts:      c.unflushed,
				})
			}
		}
		cs.mu.Unlock()
	}
	return rows
}

// retire moves a written row's counts from unflushed to base. A counter that rolled over since
// was started over; its new counts are not the row's.
func (s *Service) retire(r *usageRow) {
	cs := s.counters[shardIndex(r.key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.counters[r.key]
	if !ok || c.periodStart != r.periodStart {
		return
	}
	for i := 0; i < numDaily; i++ {
		c.unflushed[i] -= r.counts[i]
		c.base[i] += r.counts[i]
	}
}

// read sets the base of every key with a row of its current period written since the given
// database time, creating counters as needed; a key with a row is promoted, so this node's
// counts of it are written too. It returns how many rows it applied.
func (s *Service) read(ctx context.Context, since int64) (int, error) {
	dbNow := s.dbTime(s.now())
	applied := 0
	err := s.store.readSince(ctx, since, func(r *usageRow) {
		start := periodStart(r.key, dbNow)
		if r.periodStart != start {
			return // Stale, or a period this node's clock has not reached yet (the next read has it)
		}
		cs := s.counters[shardIndex(r.key)]
		cs.mu.Lock()
		c := cs.counterNoLock(r.key, start)
		if c.periodStart == start {
			c.base = r.counts
			c.promoted = true
			applied++
		}
		cs.mu.Unlock()
	})
	return applied, err
}

// takeTokens marks the key as used and takes n tokens from its bucket for the rule, if the rule
// has one and it has room
func (s *Service) takeTokens(key string, rule *Rule, n int64, now time.Time) Decision {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	e := bs.entryNoLock(key, now)
	e.seen = now.UnixNano()
	if rule.Burst <= 0 {
		e.countNoLock(key, rule.Kind, n, s.dbTime(now))
		return Decision{OK: true}
	}
	b := &e.buckets[rule.Kind]
	b.refill(now, rule)
	if n > rule.Burst {
		return Decision{} // Can never fit, so there is no time to wait for
	} else if b.tokens < float64(n) {
		return Decision{RetryAfter: b.wait(n, rule)}
	}
	b.tokens -= float64(n)
	e.noteFull(b, rule)
	e.countNoLock(key, rule.Kind, n, s.dbTime(now))
	return Decision{OK: true}
}

// refundTokens hands n tokens back to the key's bucket of the kind, up to its burst
func (s *Service) refundTokens(key string, kind Kind, n int64) {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if e, ok := bs.entries[key]; ok {
		b := &e.buckets[kind]
		if b.last != 0 {
			b.tokens = math.Min(b.tokens+float64(n), float64(b.burst))
		}
	}
}

// reset fills the key's bucket of the kind and zeroes its daily count (see limiter.Reset)
func (s *Service) reset(key string, kind Kind) {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	if e, ok := bs.entries[key]; ok {
		e.buckets[kind].tokens = float64(e.buckets[kind].burst)
		e.counts[kind] = 0
	}
	bs.mu.Unlock()
	i := dailyIndex(kind)
	if i < 0 {
		return
	}
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok {
		c.unflushed[i] = -c.base[i]
	}
}

// dbTime returns the database's time at the node's time now (see clockOffset)
func (s *Service) dbTime(now time.Time) time.Time {
	return now.Add(time.Duration(s.clockOffset.Load()))
}

// counterNoLock returns the key's counter for the period starting at start, creating it or
// starting it over (counts to zero, unpromoted) if its period has ended. A counter of a later
// period is kept: the clock offset may jitter at the boundary. The caller must hold cs.mu.
func (cs *counterShard) counterNoLock(key string, start int64) *counter {
	c, ok := cs.counters[key]
	if !ok {
		c = &counter{periodStart: start}
		cs.counters[key] = c
	} else if c.periodStart < start {
		*c = counter{periodStart: start}
	}
	return c
}

// entryNoLock returns the key's bucket entry, creating it if needed. The caller must hold bs.mu.
func (bs *bucketShard) entryNoLock(key string, now time.Time) *bucketEntry {
	e, ok := bs.entries[key]
	if !ok {
		e = &bucketEntry{seen: now.UnixNano()}
		bs.entries[key] = e
	}
	return e
}

// countNoLock adds n units of a kind without a counter to the entry's counts of the current
// period, starting them over if the period has ended. The caller must hold the shard's lock.
func (e *bucketEntry) countNoLock(key string, kind Kind, n int64, dbNow time.Time) {
	if dailyIndex(kind) >= 0 {
		return // Counted in the key's counter
	}
	if start := periodStart(key, dbNow); e.periodStart < start {
		e.counts = [numKinds]int64{}
		e.periodStart = start
	}
	e.counts[kind] += n
}

// noteFull moves the time from which every bucket is full again to when this bucket is full, if
// that is later. A bucket without refill never fills, so it does not hold the key.
func (e *bucketEntry) noteFull(b *bucket, rule *Rule) {
	missing := float64(rule.Burst) - b.tokens
	if missing <= 0 || rule.Refill <= 0 {
		return
	}
	full := b.last + int64(math.Ceil(missing*float64(rule.Refill)))
	if full > e.full {
		e.full = full
	}
}

// refill adds the tokens earned since the last refill, capped at the rule's burst, and remembers
// the burst. A bucket that was never used starts full.
func (b *bucket) refill(now time.Time, rule *Rule) {
	nowNanos := now.UnixNano()
	if b.last == 0 {
		b.tokens = float64(rule.Burst)
	} else {
		if rule.Burst != b.burst {
			// A changed burst (tier change) keeps what was spent: shift the tokens by the
			// difference, down to a debt of one burst
			b.tokens = math.Max(b.tokens+float64(rule.Burst-b.burst), -float64(rule.Burst))
		}
		// A timestamp older than the last refill (a call that read the clock before another one
		// took the lock, or a wall clock set back) neither charges nor moves the refill point back
		if rule.Refill > 0 && nowNanos > b.last {
			b.tokens += float64(nowNanos-b.last) / float64(rule.Refill)
		}
	}
	if b.tokens > float64(rule.Burst) {
		b.tokens = float64(rule.Burst)
	}
	if nowNanos > b.last {
		b.last = nowNanos
	}
	b.burst = rule.Burst
}

// wait returns how long until the bucket has n tokens, or 0 if it never will (no refill)
func (b *bucket) wait(n int64, rule *Rule) time.Duration {
	if rule.Refill <= 0 {
		return 0
	}
	return time.Duration((float64(n) - b.tokens) * float64(rule.Refill))
}
