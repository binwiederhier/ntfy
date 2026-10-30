// Package limit answers one question: may key K use N units of kind X? A key is a visitor
// identity (see package visitor), a kind is what is being counted (requests, messages, bytes, ...),
// and a Rule, passed in on every call, says how much is allowed: a token bucket, a daily cap, or
// both. Rules are not baked into the state, so a changed rule (a tier change) applies on the next
// call without rebuilding anything and without handing back a full bucket.
//
// The service knows nothing about users, tiers or HTTP. The state is in memory, sharded by key.
// The daily counts of the kinds that cost money or are billed (messages, emails, calls) are also
// written to the usage table, one row per key, period and kind, and read back, so they survive a
// restart and are the same on every node sharing the table. A key's "day" is its own rolling 24h
// period, starting at an offset hashed from the key (see periodOf), so the resets of all keys are
// spread over the day instead of all happening at midnight. The hot path never waits for the
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

	// usageRetention is how long a row is kept after its last write before PruneStore deletes it.
	// A row not written for a whole period is never its key's current one again; the second
	// period keeps the previous one around for a look.
	usageRetention = 2 * oneDay

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
	conf            *Config
	now             func() time.Time // Injectable clock, for tests
	shards          [shardCount]*shard
	store           atomic.Pointer[Store] // Nil when nothing is persisted or shared; atomic so the hot path takes no lock for it
	clockOffset     atomic.Int64          // The store's clock minus this node's in nanoseconds, as of the last flush; periods follow the store's clock
	testShift       atomic.Int64          // Tests only: added to the store's clock (see ShiftClockForTest)
	pulledAt        atomic.Int64          // Store clock watermark of the last read-back; 0 before the first
	syncMu          sync.Mutex            // Serializes every store read and the apply of its rows with every write and its retirement (see write)
	afterWriteHook  func()                // Tests only: runs between a write's commit and its retirement
	beforeApplyHook func()                // Tests only: runs between a read's query and applying its rows
	closeChan       chan struct{}
	closeOnce       sync.Once
	wg              sync.WaitGroup
	misses          []string      // Keys whose rows have not been read yet (see readMisses)
	missSignal      chan struct{} // Wakes the miss loop as soon as a key is queued; buffered, so a signal is never lost or waited for
	missMu          sync.Mutex    // Protects misses
}

// shard is one independently locked part of the key space
type shard struct {
	entries map[string]*entry
	mu      sync.Mutex // Protects entries and everything inside them
}

// entry is one key's state: a bucket and a daily count per kind, and when it was last used.
// The count is baseline plus unflushed: what the store knows (the cluster sum as of the last
// read) plus what this node counted since and has not written. For kinds that are never
// written, unflushed simply holds the period's count.
type entry struct {
	period    int64 // The key's usage period the counts belong to (see periodOf)
	buckets   [numKinds]bucket
	baseline  [numKinds]int64 // The key's count in the store as of the last read, per kind
	unflushed [numKinds]int64 // Counted here, not yet written, per kind
	promoted  [numKinds]bool  // Whether the kind's count of this key is written to the store (see Rule.Threshold)
	loaded    bool            // Whether the key's rows were read back (or there is no store)
	missNoted bool            // Whether the key waits in misses for its read-back
	seen      int64           // Unix nanoseconds of the last use, for pruning
	full      int64           // Unix nanoseconds from which every bucket is full again, for pruning
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
		s.shards[i] = &shard{
			entries: make(map[string]*entry),
		}
	}
	return s
}

// Allow decides whether the key may use n units of the rule's kind now, and if so, uses them.
// Both the bucket (if the rule has one) and the daily cap (if it has one) must have room.
func (s *Service) Allow(key string, rule Rule, n int64) Decision {
	now := s.now()
	clockNow := s.storeTime(now)
	store := s.store.Load()
	sh := s.shards[shardIndex(key)]
	if store != nil && syncLoadKinds[rule.Kind] {
		s.maybeLoadNow(store, sh, key)
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	period := periodOf(key, clockNow)
	e := sh.entryNoLock(key, now, period, store == nil)
	e.roll(period, store != nil)
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
		return Decision{RetryAfter: periodEnd(key, e.period).Sub(clockNow)}
	}
	if rule.Burst > 0 {
		b.tokens -= float64(n)
		e.noteFull(b, &rule)
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
	sh := s.shards[shardIndex(key)]
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
	period := s.periodNow(key)
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return
	}
	e.roll(period, s.store.Load() != nil)
	b := &e.buckets[kind]
	if b.last != 0 {
		b.tokens = math.Min(b.tokens+float64(n), float64(b.burst))
	}
	e.unflushed[kind] = max(-e.baseline[kind], e.unflushed[kind]-n) // The total cannot go below zero; a negative unflushed is a refund still to be written
}

// Usage returns the key's usage in its current period, of every kind, as this node knows it (see
// Lookup for the store's numbers). A key not used since its period ended has none.
func (s *Service) Usage(key string) Usage {
	period := s.periodNow(key)
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok || e.period < period {
		return Usage{}
	}
	return e.usage()
}

// Lookup returns the key's usage in its current period, reading its rows from the store first if
// they have not been read yet (a key the service never saw, whose read-back is still pending, or
// that was not used since its period ended)
func (s *Service) Lookup(key string) (Usage, error) {
	store := s.store.Load()
	period := s.periodNow(key)
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	e, ok := sh.entries[key]
	if store == nil || (ok && e.loaded && e.period >= period) {
		sh.mu.Unlock()
		return s.Usage(key), nil
	}
	sh.mu.Unlock()
	if err := s.readAndApply(store, []string{key}); err != nil {
		return s.Usage(key), err
	}
	return s.Usage(key), nil
}

// Prune forgets keys that have not been used for longer than idle, except keys with counts that
// are still to be written, keys with a row, which stay for at least promotedRetention, and keys
// whose buckets are not full again, and returns how many it forgot. A key that comes back starts
// with full buckets, and with its counts read back from the store if it has rows.
func (s *Service) Prune(idle time.Duration) int {
	now := s.now()
	cutoff := now.Add(-idle).UnixNano()
	promotedCutoff := now.Add(-max(idle, promotedRetention)).UnixNano()
	pruned := 0
	for _, sh := range s.shards {
		sh.mu.Lock()
		for key, e := range sh.entries {
			if e.pending() {
				continue
			} else if e.anyPromoted() && e.seen >= promotedCutoff {
				continue
			} else if e.seen < cutoff && now.UnixNano() >= e.full {
				delete(sh.entries, key)
				pruned++
			}
		}
		sh.mu.Unlock()
	}
	return pruned
}

// Touch marks the key as used now, so Prune keeps it, e.g. for a long-lived subscription
func (s *Service) Touch(key string) {
	now := s.now()
	store := s.store.Load()
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.entryNoLock(key, now, periodOf(key, s.storeTime(now)), store == nil).seen = now.UnixNano()
}

// Seen returns when the key was last used, or the zero time if the service does not know it
func (s *Service) Seen(key string) time.Time {
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return time.Time{}
	}
	return time.Unix(0, e.seen).UTC()
}

// Forget drops the key from memory, unless it has counts still to be written; its rows stay in
// the store and are read back when it is next met. It reports whether the key was dropped.
func (s *Service) Forget(key string) bool {
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok || e.pending() {
		return false
	}
	delete(sh.entries, key)
	return true
}

// PruneStore deletes the usage rows not written for longer than usageRetention; in a cluster, one
// node (the leader) calls it
func (s *Service) PruneStore() error {
	store := s.store.Load()
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	return store.prune(ctx, usageRetention)
}

// ResetAt returns when the key's daily counts start over: 24 hours after its current period
// started, by the store's clock (see periodOf)
func (s *Service) ResetAt(key string) time.Time {
	return periodEnd(key, s.periodNow(key))
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

// Persistent reports whether usage is written to a store, i.e. whether a key's daily counts
// outlive its time in memory
func (s *Service) Persistent() bool {
	return s.store.Load() != nil
}

// Size returns the number of keys the service holds
func (s *Service) Size() int {
	size := 0
	for _, sh := range s.shards {
		sh.mu.Lock()
		size += len(sh.entries)
		sh.mu.Unlock()
	}
	return size
}

// Close stops the loops and writes what is unflushed. The store's database is its owner's to close
// (the user manager's, or the shared pool), after this.
func (s *Service) Close() {
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
	})
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

// flush is one round: sync the clock with the store, write the unflushed counts of the promoted
// keys, read back what changed in the store since the last round, and read the rows of newly met
// keys. Every store error is logged and leaves the state as it was: counts stay unflushed, totals
// serve stale.
func (s *Service) flush() {
	store := s.store.Load()
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	dbNow, err := store.clock(ctx)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot read the usage store's clock")
		metrics.UsageFlushErrors.Inc()
	} else {
		s.clockOffset.Store(int64(time.Unix(dbNow, 0).Sub(s.now())))
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
//
// A write holds syncMu from its snapshot to its retirement, and so does every read of the store
// from its query to applying the rows. Otherwise a snapshot read after a commit but applied before
// the retirement counts the increments twice, and one read before a commit but applied after the
// retirement drops them. The hot path (Allow) never takes syncMu.
func (s *Service) write(ctx context.Context, store *Store) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	rows := s.snapshot()
	if len(rows) == 0 {
		return
	}
	written, err := store.flush(ctx, rows)
	if s.afterWriteHook != nil {
		s.afterWriteHook()
	}
	s.retire(rows[:written])
	if err != nil {
		log.Tag(tag).Err(err).Warn("Writing usage failed, %d of %d rows written; the rest is retried", written, len(rows))
		metrics.UsageFlushErrors.Inc()
	}
	metrics.UsageRowsWritten.Add(float64(written))
}

// readBack reads the rows written since the last watermark (minus the overlap) and sets the
// baseline of the keys this node holds; other keys are read when they are met.
func (s *Service) readBack(ctx context.Context, store *Store, dbNow int64) {
	since := s.pulledAt.Load()
	if since == 0 {
		// The first round: the rows so far are read per key as keys are met, not all at once, so
		// the memory stays at the active set
		s.pulledAt.Store(dbNow)
		return
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	rows, err := store.readSince(ctx, since-int64(readOverlap.Seconds()))
	if err != nil {
		log.Tag(tag).Err(err).Warn("Reading usage back failed; totals serve stale until the next round")
		metrics.UsageFlushErrors.Inc()
		return
	}
	s.applyRows(rows, nil)
	s.pulledAt.Store(dbNow)
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
	if err := s.readAndApply(store, keys); err != nil {
		log.Tag(tag).Err(err).Warn("Reading the usage of %d new key(s) failed; they are served from local counts until the next try", len(keys))
		metrics.UsageFlushErrors.Inc()
		s.missMu.Lock()
		s.misses = append(keys, s.misses...)
		s.missMu.Unlock()
	}
}

// snapshot returns the unflushed counts of the promoted keys as rows to write, each under the
// period it was counted in (for a key idle since its period ended, the old one)
func (s *Service) snapshot() []usageRow {
	rows := make([]usageRow, 0)
	for _, sh := range s.shards {
		sh.mu.Lock()
		for key, e := range sh.entries {
			for kind := Kind(0); kind < numKinds; kind++ {
				if e.promoted[kind] && e.unflushed[kind] != 0 {
					rows = append(rows, usageRow{
						key:    key,
						period: e.period,
						kind:   kind,
						count:  e.unflushed[kind],
					})
				}
			}
		}
		sh.mu.Unlock()
	}
	return rows
}

// retire moves written increments from unflushed to the baseline, so the total is unchanged and
// the increments are not written again. An entry that rolled over to a new period meanwhile was
// reset; the row was still correct under its period.
func (s *Service) retire(rows []usageRow) {
	for _, r := range rows {
		sh := s.shards[shardIndex(r.key)]
		sh.mu.Lock()
		if e, ok := sh.entries[r.key]; ok && e.period == r.period {
			e.unflushed[r.kind] -= r.count
			e.baseline[r.kind] += r.count
		}
		sh.mu.Unlock()
	}
}

// applyRows sets the baselines of the keys this node holds from the store's rows of each entry's
// period, and marks the given keys as loaded in their current period, creating them if needed so
// a looked-up key keeps its rows; a loaded key without rows is at zero. Each key's loaded flag and
// baselines change under one lock, so no decision sees a key loaded without its counts. Rows of
// other periods are ignored. The caller must hold syncMu.
func (s *Service) applyRows(rows []usageRow, loadedKeys []string) {
	byKey := make(map[string][]usageRow, len(loadedKeys))
	for _, r := range rows {
		byKey[r.key] = append(byKey[r.key], r)
	}
	loading := make(map[string]bool, len(loadedKeys))
	for _, key := range loadedKeys {
		loading[key] = true
		if _, ok := byKey[key]; !ok {
			byKey[key] = nil
		}
	}
	now := s.now()
	clockNow := s.storeTime(now)
	for key, keyRows := range byKey {
		sh := s.shards[shardIndex(key)]
		sh.mu.Lock()
		e, ok := sh.entries[key]
		if !ok && loading[key] {
			e = sh.entryNoLock(key, now, periodOf(key, clockNow), false)
			ok = true
		}
		if ok {
			if loading[key] {
				e.roll(periodOf(key, clockNow), true)
			}
			for _, r := range keyRows {
				if r.period == e.period {
					e.baseline[r.kind] = r.count
				}
			}
			if loading[key] {
				e.loaded = true
				e.missNoted = false
			}
		}
		sh.mu.Unlock()
	}
}

// setStore replaces the store; tests use it to recover from a broken one
func (s *Service) setStore(store *Store) {
	s.store.Store(store)
}

// ShiftClockForTest moves the clock the periods follow forward by d (the buckets keep the real
// one). Tests use it, because the periods follow the store's clock, which a test cannot move.
func (s *Service) ShiftClockForTest(d time.Duration) {
	s.testShift.Add(int64(d))
}

// storeTime returns the store's time at the node's time now (see clockOffset)
func (s *Service) storeTime(now time.Time) time.Time {
	return now.Add(time.Duration(s.clockOffset.Load() + s.testShift.Load()))
}

// periodNow returns the key's current usage period, by the store's clock
func (s *Service) periodNow(key string) int64 {
	return periodOf(key, s.storeTime(s.now()))
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
	period := s.periodNow(key)
	sh.mu.Lock()
	e, ok := sh.entries[key]
	loaded := ok && e.loaded && e.period >= period
	sh.mu.Unlock()
	if loaded {
		return
	}
	if err := s.readAndApply(store, []string{key}); err != nil {
		log.Tag(tag).Err(err).Warn("Reading the usage of key %s failed; it is decided on its local count", key)
		metrics.UsageFlushErrors.Inc()
	}
}

// readAndApply reads the given keys' rows and applies them, as one step with respect to writes
// (see write)
func (s *Service) readAndApply(store *Store, keys []string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	rows, err := store.readKeys(ctx, keys)
	if err != nil {
		return err
	}
	if s.beforeApplyHook != nil {
		s.beforeApplyHook()
	}
	s.applyRows(rows, keys)
	return nil
}

// reset fills the key's bucket of the kind and zeroes its count (see limiter.Reset)
func (s *Service) reset(key string, kind Kind) {
	period := s.periodNow(key)
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return
	}
	e.roll(period, s.store.Load() != nil)
	e.buckets[kind].tokens = float64(e.buckets[kind].burst)
	e.unflushed[kind] = -e.baseline[kind]
}

// entryNoLock returns the key's entry, creating it in the given period if needed. A key met
// without a store has nothing to read back, so it is loaded at once. The caller must hold sh.mu.
func (sh *shard) entryNoLock(key string, now time.Time, period int64, loaded bool) *entry {
	e, ok := sh.entries[key]
	if !ok {
		e = &entry{
			period: period,
			seen:   now.UnixNano(),
			loaded: loaded,
		}
		sh.entries[key] = e
	}
	return e
}

// roll starts the entry's daily counts over if the given period is a later one than the entry's
// (never back: a clock offset jittering at the boundary must not reset twice). The buckets keep
// their tokens. With a store, the key's rows of the new period are read back when it is next used
// (another node may have counted already). What the old period counted and was not written yet
// is dropped: at most one flush interval of usage, of a period that is over and that nothing
// enforces or reports on anymore; an idle key's counts are written under their own period (see
// snapshot). The caller must hold the shard's lock.
func (e *entry) roll(period int64, persistent bool) {
	if period <= e.period {
		return
	}
	e.period = period
	e.baseline = [numKinds]int64{}
	e.unflushed = [numKinds]int64{}
	e.promoted = [numKinds]bool{}
	e.loaded = !persistent
	e.missNoted = false
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

// noteFull moves the time from which every bucket is full again to when this bucket is full, if
// that is later. A bucket without refill never fills, so it does not hold the key.
func (e *entry) noteFull(b *bucket, rule *Rule) {
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
