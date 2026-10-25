// Package limit answers one question: may key K use N units of kind X? A key is a visitor
// identity (see package visitor), a kind is what is being counted (requests, messages, bytes, ...),
// and a Rule, passed in on every call, says how much is allowed: a token bucket, a daily cap, or
// both. Rules are not baked into the state, so a changed rule (a tier change) applies on the next
// call without rebuilding anything and without handing back a full bucket.
//
// The service knows nothing about users, tiers or HTTP. The state is in memory, sharded by key.
// The daily counts of the kinds that cost money or are billed (messages, emails, calls) are also
// written to the usage table, one row per key, day and kind, and read back, so they survive a
// restart and are the same on every node sharing the table. The hot path never waits for the
// store: counts are written and read in the background, and every failure degrades to the
// in-memory count (fail open, bounded by the local limit).
package limit

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/util"
)

const (
	tag        = "limit"
	shardCount = 64 // Keys are spread over this many independently locked maps

	// DefaultFlushInterval is how often usage is written to and read back from the store. It
	// bounds the quota overshoot in a cluster ((nodes - 1) x admit rate x interval) and how
	// long a node takes to see another's usage.
	DefaultFlushInterval = 5 * time.Second

	// missInterval is how often the rows of keys the service has just met are read back
	missInterval = 500 * time.Millisecond

	// missesMax caps the keys waiting for a read-back; beyond it a new key is served from its
	// local count only (fail open), which is what a flood of fresh keys gets anyway
	missesMax = 100000

	// flushTimeout bounds one statement against the store. Rows are stamped with the database
	// clock at the start of their statement, so a row's updated_at lags its commit by at most
	// this much; readOverlap re-reads that far behind the watermark so a slow writer is never
	// skipped (re-reads are idempotent: rows carry sums, not deltas).
	flushTimeout = 5 * time.Second
	readOverlap  = flushTimeout + time.Second

	// usageRetention is how long old days' rows are kept before PruneStore deletes them
	usageRetention = 7 * 24 * time.Hour

	// promotedRetention is how long a key with a row (one near its quota, see Rule.Threshold)
	// stays in memory however idle it is, so that going idle for a few minutes does not get a
	// fresh local count ahead of the read-back
	promotedRetention = time.Hour
)

// persistedKinds are the kinds whose daily counts are written to the store: the ones that cost
// money or are billed. The others converge through their buckets and are never persisted.
var persistedKinds = [numKinds]bool{
	KindMessages: true,
	KindEmails:   true,
	KindCalls:    true,
}

// syncLoadKinds are the persisted kinds whose first use of a key reads the key's rows before
// deciding: emails and calls cost real money and are rare, so the wait is worth it. Messages are
// the hot path and are read back in the background instead (fail open for a moment).
var syncLoadKinds = [numKinds]bool{
	KindEmails: true,
	KindCalls:  true,
}

// Service keeps the per-key state and decides on usage. See the package comment.
type Service struct {
	conf        *Config
	now         func() time.Time // Injectable clock, for tests
	shards      [shardCount]shard
	store       atomic.Pointer[Store] // Nil when nothing is persisted or shared; atomic so the hot path never takes mu
	day         string                // The usage day the counts refer to (see today)
	clockOffset time.Duration         // The store's clock minus this node's, as of the last flush; the day follows the store's clock
	pulledAt    int64                 // Store clock watermark of the last read-back; 0 before the first
	closeChan   chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup
	misses      []string      // Keys whose rows have not been read yet (see readMisses)
	missSignal  chan struct{} // Wakes the miss loop as soon as a key is queued; buffered, so a signal is never lost or waited for
	missMu      sync.Mutex    // Protects misses
	mu          sync.Mutex    // Protects day, clockOffset and pulledAt
}

// shard is one independently locked part of the key space
type shard struct {
	entries map[string]*entry
	mu      sync.Mutex // Protects entries and everything inside them
}

// entry is one key's state: a bucket and a daily count per kind, and when it was last used.
// The count is baseline plus unflushed: what the store knows (the cluster sum as of the last
// read) plus what this node counted since and has not written. For kinds that are never
// written, unflushed simply holds the day's count.
type entry struct {
	buckets   [numKinds]bucket
	baseline  [numKinds]int64 // The key's count in the store as of the last read, per kind
	unflushed [numKinds]int64 // Counted here, not yet written, per kind
	promoted  [numKinds]bool  // Whether the kind's count of this key is written to the store (see Rule.Threshold)
	loaded    bool            // Whether the key's rows were read back (or there is no store)
	missNoted bool            // Whether the key waits in misses for its read-back
	seen      int64           // Unix nanoseconds of the last use, for pruning
}

// bucket is a token bucket. It is created full: last is zero until the first use, and tokens
// then start at the rule's burst. Tokens may go negative (debt) when peers' usage is charged in.
type bucket struct {
	tokens float64
	last   int64 // Unix nanoseconds of the last refill, 0 if never used
	burst  int64 // Burst of the rule the bucket was last used with; caps refunds and debt
}

// New creates the service on the given store (nil for in-memory only) and starts its loops
func New(conf *Config, store *Store) *Service {
	s := newService(conf, time.Now, store)
	s.wg.Add(2)
	go s.runFlushLoop()
	go s.runMissLoop()
	return s
}

func newService(conf *Config, now func() time.Time, store *Store) *Service {
	if conf.FlushInterval <= 0 {
		conf.FlushInterval = DefaultFlushInterval
	}
	s := &Service{
		conf:       conf,
		now:        now,
		closeChan:  make(chan struct{}),
		missSignal: make(chan struct{}, 1),
	}
	s.store.Store(store)
	for i := range s.shards {
		s.shards[i].entries = make(map[string]*entry)
	}
	s.day = s.today()
	return s
}

// Allow decides whether the key may use n units of the rule's kind now, and if so, uses them.
// Both the bucket (if the rule has one) and the daily cap (if it has one) must have room.
func (s *Service) Allow(key string, rule Rule, n int64) Decision {
	now := s.now()
	store := s.store.Load()
	sh := &s.shards[shardIndex(key)]
	if store != nil && syncLoadKinds[rule.Kind] {
		s.maybeLoadNow(store, sh, key)
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e := sh.entryNoLock(key, now, store == nil)
	e.seen = now.UnixNano()
	b := &e.buckets[rule.Kind]
	if rule.Burst > 0 {
		b.refill(now, &rule)
		if n > rule.Burst {
			return Decision{} // Can never fit, so there is no time to wait for
		} else if b.tokens < float64(n) {
			return Decision{RetryAfter: b.wait(n, &rule)}
		}
	}
	total := e.baseline[rule.Kind] + e.unflushed[rule.Kind]
	if rule.Daily > 0 && total+n > rule.Daily {
		return Decision{RetryAfter: untilReset(now, s.conf.ResetTime)}
	}
	if rule.Burst > 0 {
		b.tokens -= float64(n)
	}
	e.unflushed[rule.Kind] += n
	if store != nil && persistedKinds[rule.Kind] {
		if !e.promoted[rule.Kind] && total+n >= rule.Threshold {
			e.promoted[rule.Kind] = true
		}
		if !e.loaded && !e.missNoted {
			e.missNoted = true
			s.noteMiss(key)
		}
	}
	return Decision{OK: true}
}

// Tokens returns the whole tokens the key's bucket for the rule's kind has right now, without
// using any. It is what a log line or a "may I try" check wants; Allow is what consumes.
func (s *Service) Tokens(key string, rule Rule) int64 {
	if rule.Burst <= 0 {
		return 0
	}
	now := s.now()
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
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
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return
	}
	b := &e.buckets[kind]
	if b.last != 0 {
		b.tokens = math.Min(b.tokens+float64(n), float64(b.burst))
	}
	e.unflushed[kind] = max(-e.baseline[kind], e.unflushed[kind]-n) // The total cannot go below zero; a negative unflushed is a refund still to be written
}

// Usage returns the key's usage today, of every kind, as this node knows it (see Lookup for the
// store's numbers)
func (s *Service) Usage(key string) Usage {
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return Usage{}
	}
	return e.usage()
}

// Lookup returns the key's usage today, reading its rows from the store first if they have not
// been read yet (a key the service never saw, or whose read-back is still pending)
func (s *Service) Lookup(key string) (Usage, error) {
	store := s.store.Load()
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	e, ok := sh.entries[key]
	if store == nil || (ok && e.loaded) {
		sh.mu.Unlock()
		return s.Usage(key), nil
	}
	sh.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	day := s.today()
	rows, err := store.readKeys(ctx, day, []string{key})
	if err != nil {
		return s.Usage(key), err
	}
	s.applyRows(day, rows, []string{key})
	return s.Usage(key), nil
}

// Prune forgets keys that have not been used for longer than idle, except keys with counts that
// are still to be written, and keys with a row, which stay for at least promotedRetention. A key
// that comes back starts with full buckets, and with its counts read back from the store if it
// has rows.
func (s *Service) Prune(idle time.Duration) {
	now := s.now()
	cutoff := now.Add(-idle).UnixNano()
	promotedCutoff := now.Add(-max(idle, promotedRetention)).UnixNano()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for key, e := range sh.entries {
			if e.pending() {
				continue
			} else if e.anyPromoted() && e.seen >= promotedCutoff {
				continue
			} else if e.seen < cutoff {
				delete(sh.entries, key)
			}
		}
		sh.mu.Unlock()
	}
}

// Forget drops the key from memory, unless it has counts still to be written; its rows stay in
// the store and are read back when it is next met. It reports whether the key was dropped.
func (s *Service) Forget(key string) bool {
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok || e.pending() {
		return false
	}
	delete(sh.entries, key)
	return true
}

// PruneStore deletes the usage rows of days older than usageRetention; in a cluster, one node
// (the leader) calls it
func (s *Service) PruneStore() error {
	store := s.store.Load()
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	return store.prune(ctx, s.dayFor(s.now().Add(-usageRetention)))
}

// Limiter returns a util.Limiter view of the key's bucket for the rule, for the readers and
// writers in util that meter bytes. A negative AllowN refunds.
func (s *Service) Limiter(key string, rule Rule) util.Limiter {
	return &limiter{s: s, key: key, rule: rule}
}

// Persistent reports whether usage is written to a store, i.e. whether a key's daily counts
// outlive its time in memory
func (s *Service) Persistent() bool {
	return s.store.Load() != nil
}

// Size returns the number of keys the service holds
func (s *Service) Size() int {
	size := 0
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		size += len(sh.entries)
		sh.mu.Unlock()
	}
	return size
}

// Close stops the loops, writes what is unflushed, and closes the store
func (s *Service) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closeChan)
		s.wg.Wait()
		store := s.store.Load()
		if store == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		s.write(ctx, store)
		err = store.Close()
	})
	return err
}

// runFlushLoop writes and reads back usage every FlushInterval until Close
func (s *Service) runFlushLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.conf.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.closeChan:
			return
		}
	}
}

// runMissLoop reads back the rows of newly met keys as soon as they are queued (and every
// missInterval regardless, in case a read failed) until Close
func (s *Service) runMissLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(missInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.missSignal:
			s.readMisses()
		case <-ticker.C:
			s.readMisses()
		case <-s.closeChan:
			return
		}
	}
}

// flush is one round: sync the clock with the store, roll the day over if it changed, write the
// unflushed counts of the promoted keys, read back what changed in the store since the last
// round, and read the rows of newly met keys. Every store error is logged and leaves the state
// as it was: counts stay unflushed, totals serve stale.
func (s *Service) flush() {
	store := s.store.Load()
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	var dbNow int64
	if store != nil {
		now, err := store.clock(ctx)
		if err != nil {
			log.Tag(tag).Err(err).Warn("Cannot read the usage store's clock")
			metrics.UsageFlushErrors.Inc()
		} else {
			dbNow = now
			s.mu.Lock()
			s.clockOffset = time.Unix(now, 0).Sub(s.now())
			s.mu.Unlock()
		}
	}
	s.maybeRollDay()
	if store == nil {
		return
	}
	s.write(ctx, store)
	if dbNow > 0 {
		s.readBack(ctx, store, dbNow)
	}
	s.readMisses()
}

// write writes the unflushed counts of the promoted keys to the store as increments, in one
// statement per chunk. Counts stay unflushed until their own row is written: a failed statement
// leaves its keys' increments where they are for the next round, and the keys before it are
// retired, so nothing is written twice.
func (s *Service) write(ctx context.Context, store *Store) {
	s.mu.Lock()
	day := s.day
	s.mu.Unlock()
	rows := s.snapshot()
	if len(rows) == 0 {
		return
	}
	written, err := store.flush(ctx, day, rows)
	s.retire(day, rows[:written])
	if err != nil {
		log.Tag(tag).Err(err).Warn("Writing usage failed, %d of %d rows written; the rest is retried", written, len(rows))
		metrics.UsageFlushErrors.Inc()
	}
	metrics.UsageRowsWritten.Add(float64(written))
}

// readBack reads the day's rows written since the last watermark (minus the overlap) and sets
// the baseline of the keys this node holds; other keys are read when they are met.
func (s *Service) readBack(ctx context.Context, store *Store, dbNow int64) {
	s.mu.Lock()
	day, since := s.day, s.pulledAt
	s.mu.Unlock()
	if since == 0 {
		// The first round (of the process, or of the day): the day's rows so far are read per key
		// as keys are met, not all at once, so the memory stays at the active set
		s.setPulledAt(dbNow)
		return
	}
	rows, err := store.readSince(ctx, day, since-int64(readOverlap.Seconds()))
	if err != nil {
		log.Tag(tag).Err(err).Warn("Reading usage back failed; totals serve stale until the next round")
		metrics.UsageFlushErrors.Inc()
		return
	}
	s.applyRows(day, rows, nil)
	s.setPulledAt(dbNow)
}

// readMisses reads the rows of the keys met since the last call and marks them loaded; a key
// without rows is loaded at zero. A failed read puts the keys back for the next call.
func (s *Service) readMisses() {
	store := s.store.Load()
	s.missMu.Lock()
	keys := s.misses
	s.misses = nil
	s.missMu.Unlock()
	if store == nil || len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	day := s.today()
	rows, err := store.readKeys(ctx, day, keys)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Reading the usage of %d new key(s) failed; they are served from local counts until the next try", len(keys))
		metrics.UsageFlushErrors.Inc()
		s.missMu.Lock()
		s.misses = append(keys, s.misses...)
		s.missMu.Unlock()
		return
	}
	s.applyRows(day, rows, keys)
}

// snapshot returns the unflushed counts of the promoted keys as rows to write
func (s *Service) snapshot() []usageRow {
	rows := make([]usageRow, 0)
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for key, e := range sh.entries {
			for kind := Kind(0); kind < numKinds; kind++ {
				if e.promoted[kind] && e.unflushed[kind] != 0 {
					rows = append(rows, usageRow{key: key, kind: kind, count: e.unflushed[kind]})
				}
			}
		}
		sh.mu.Unlock()
	}
	return rows
}

// retire moves written increments from unflushed to the baseline, so the total is unchanged and
// the increments are not written again. After a day rollover the entries were reset; the rows
// were still correct under their day.
func (s *Service) retire(day string, rows []usageRow) {
	s.mu.Lock()
	rolled := s.day != day
	s.mu.Unlock()
	if rolled {
		return
	}
	for _, r := range rows {
		sh := &s.shards[shardIndex(r.key)]
		sh.mu.Lock()
		if e, ok := sh.entries[r.key]; ok {
			e.unflushed[r.kind] -= r.count
			e.baseline[r.kind] += r.count
		}
		sh.mu.Unlock()
	}
}

// applyRows marks the given keys as loaded (creating them if needed, so a looked-up key keeps
// its rows), then sets the baseline of the keys this node holds from the store's rows; a loaded
// key without rows is at zero. Rows of another day than the current one are discarded (the day
// rolled over while reading).
func (s *Service) applyRows(day string, rows []usageRow, loadedKeys []string) {
	s.mu.Lock()
	rolled := s.day != day
	s.mu.Unlock()
	if rolled {
		return
	}
	now := s.now()
	for _, key := range loadedKeys {
		sh := &s.shards[shardIndex(key)]
		sh.mu.Lock()
		e := sh.entryNoLock(key, now, false)
		e.loaded = true
		e.missNoted = false
		sh.mu.Unlock()
	}
	for _, r := range rows {
		sh := &s.shards[shardIndex(r.key)]
		sh.mu.Lock()
		if e, ok := sh.entries[r.key]; ok {
			e.baseline[r.kind] = r.count
		}
		sh.mu.Unlock()
	}
}

// maybeRollDay starts a new usage day if the clock says so: every count starts at zero, every
// key is read back when it is next met (another node may have written the new day already),
// and the watermark starts over
func (s *Service) maybeRollDay() {
	today := s.today()
	s.mu.Lock()
	if s.day == today {
		s.mu.Unlock()
		return
	}
	s.day = today
	s.pulledAt = 0
	s.mu.Unlock()
	log.Tag(tag).Info("Usage day rolled over to %s; daily counts start over", today)
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for _, e := range sh.entries {
			e.baseline = [numKinds]int64{}
			e.unflushed = [numKinds]int64{}
			e.promoted = [numKinds]bool{}
			e.loaded = false
			e.missNoted = false
		}
		sh.mu.Unlock()
	}
	s.missMu.Lock()
	s.misses = nil
	s.missMu.Unlock()
}

// rollDayForTest forces a day rollover to the given day, as if the clock had reached it. Tests
// use it, because the day follows the store's clock, which a test cannot move.
func (s *Service) rollDayForTest(day string) {
	rows := s.snapshot()
	s.mu.Lock()
	previous := s.day
	s.mu.Unlock()
	if store := s.store.Load(); store != nil && len(rows) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		if _, err := store.flush(ctx, previous, rows); err != nil {
			log.Tag(tag).Err(err).Warn("Writing usage before the forced rollover failed")
		}
	}
	s.mu.Lock()
	s.day = ""
	s.mu.Unlock()
	s.maybeRollDay()
	s.mu.Lock()
	s.day = day
	s.mu.Unlock()
}

// setStore replaces the store; tests use it to recover from a broken one
func (s *Service) setStore(store *Store) {
	s.store.Store(store)
}

// today returns the current usage day, by the store's clock if there is one
func (s *Service) today() string {
	s.mu.Lock()
	offset := s.clockOffset
	s.mu.Unlock()
	return s.dayFor(s.now().Add(offset))
}

// dayFor returns the usage day of the given time (see dayFor in util.go)
func (s *Service) dayFor(t time.Time) string {
	return dayFor(t, s.conf.ResetTime)
}

func (s *Service) setPulledAt(dbNow int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pulledAt = dbNow
}

// noteMiss queues the key for a read-back of its rows and wakes the miss loop; beyond missesMax
// the key is served from its local count only (fail open: a flood of fresh keys must not pile
// up reads)
func (s *Service) noteMiss(key string) {
	s.missMu.Lock()
	if len(s.misses) < missesMax {
		s.misses = append(s.misses, key)
	}
	s.missMu.Unlock()
	select {
	case s.missSignal <- struct{}{}:
	default: // The loop is already awake
	}
}

// maybeLoadNow reads the key's rows before its first use of a kind that must not be decided on
// a local count alone (see syncLoadKinds). A failed read is logged, and the key is decided on
// its local count (fail open).
func (s *Service) maybeLoadNow(store *Store, sh *shard, key string) {
	sh.mu.Lock()
	e, ok := sh.entries[key]
	loaded := ok && e.loaded
	sh.mu.Unlock()
	if loaded {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	day := s.today()
	rows, err := store.readKeys(ctx, day, []string{key})
	if err != nil {
		log.Tag(tag).Err(err).Warn("Reading the usage of key %s failed; it is decided on its local count", key)
		metrics.UsageFlushErrors.Inc()
		return
	}
	s.applyRows(day, rows, []string{key})
}

// reset fills the key's bucket of the kind and zeroes its count (see limiter.Reset)
func (s *Service) reset(key string, kind Kind) {
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return
	}
	e.buckets[kind].tokens = float64(e.buckets[kind].burst)
	e.unflushed[kind] = -e.baseline[kind]
}

// entryNoLock returns the key's entry, creating it if needed. A key met without a store has
// nothing to read back, so it is loaded at once. The caller must hold sh.mu.
func (sh *shard) entryNoLock(key string, now time.Time, loaded bool) *entry {
	e, ok := sh.entries[key]
	if !ok {
		e = &entry{seen: now.UnixNano(), loaded: loaded}
		sh.entries[key] = e
	}
	return e
}

// usage returns the entry's counts: what the store knows plus what is counted here since
func (e *entry) usage() Usage {
	var u Usage
	for kind := Kind(0); kind < numKinds; kind++ {
		u.counts[kind] = e.baseline[kind] + e.unflushed[kind]
	}
	return u
}

// anyPromoted reports whether any kind of the entry is written to the store
func (e *entry) anyPromoted() bool {
	for kind := Kind(0); kind < numKinds; kind++ {
		if e.promoted[kind] {
			return true
		}
	}
	return false
}

// pending reports whether the entry has counts that are still to be written
func (e *entry) pending() bool {
	for kind := Kind(0); kind < numKinds; kind++ {
		if e.promoted[kind] && e.unflushed[kind] != 0 {
			return true
		}
	}
	return false
}

// refill adds the tokens earned since the last refill, capped at the rule's burst, and remembers
// the burst. A bucket that was never used starts full.
func (b *bucket) refill(now time.Time, rule *Rule) {
	nowNanos := now.UnixNano()
	if b.last == 0 {
		b.tokens = float64(rule.Burst)
	} else if rule.Refill > 0 {
		b.tokens += float64(nowNanos-b.last) / float64(rule.Refill)
	}
	if b.tokens > float64(rule.Burst) {
		b.tokens = float64(rule.Burst)
	}
	b.last = nowNanos
	b.burst = rule.Burst
}

// wait returns how long until the bucket has n tokens, or 0 if it never will (no refill)
func (b *bucket) wait(n int64, rule *Rule) time.Duration {
	if rule.Refill <= 0 {
		return 0
	}
	return time.Duration((float64(n) - b.tokens) * float64(rule.Refill))
}
