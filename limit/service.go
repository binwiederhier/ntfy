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
//
// Periods, updated_at and the read-back watermark use the node's clock, so the nodes must
// keep their clocks in sync (NTP); a skew below a second or so is harmless, a skew beyond
// readOverlap may skip rows in the read-back. The database's clock is never used.
package limit

import (
	"context"
	"math"
	"sync"
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

	// statementTimeout bounds each store call of a sync. A row's updated_at is the writing
	// node's time before its statement, so it may commit up to this much later; reads re-read
	// readOverlap behind the watermark so such a row (or one stamped by a node clock a little
	// behind ours) is not skipped (rows are totals, so a re-read is harmless).
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

	// settledIdle is how long a key whose buckets are full again must go unused before Prune
	// forgets its buckets, so a key in steady use is not dropped and recreated over and over
	settledIdle = time.Minute
)

// Service decides on usage from memory; see the package comment
type Service struct {
	now         func() time.Time          // Injectable clock, for tests
	counters    [shardCount]*counterShard // Daily counts, kept until their period ends
	buckets     [shardCount]*bucketShard  // Token buckets, per node, forgotten when settled (see Prune)
	store       *Store                    // Nil: memory only
	watermark   int64                     // Time of the last read, unix seconds; the sync worker's only
	prunedAt    int64                     // Time of the last prune, unix seconds; the sync worker's only
	interval    time.Duration             // How often the worker syncs (syncInterval); tests change it
	trackDeltas bool                      // Whether takeTokens and refundTokens record deltas (see TrackBucketDeltas)
	epoch       int64                     // When the service was created, unix nanoseconds; orders this node's bucket rounds across restarts
	round       uint64                    // The last BucketDeltas round; protected by roundMu
	roundMu     sync.Mutex                // Serializes BucketDeltas and BucketSnapshot, so a snapshot contains whole rounds
	peers       map[string]*peerRound     // The last round taken from each peer (ChargeBuckets, RestoreBuckets); protected by peersMu
	peersMu     sync.Mutex
	ctx         context.Context // Done on Close; the sync worker's store calls derive from it, so Close never waits on a stuck one
	cancel      context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

// New creates the service; the store shares the daily counts and keeps them across restarts, nil
// for memory only
func New(store *Store) *Service {
	return newService(store, time.Now)
}

func newService(store *Store, now func() time.Time) *Service {
	s := &Service{
		now:      now,
		store:    store,
		interval: syncInterval,
		epoch:    now().UnixNano(),
		peers:    make(map[string]*peerRound),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	for i := 0; i < shardCount; i++ {
		s.counters[i] = &counterShard{
			counters: make(map[string]*counter),
		}
		s.buckets[i] = &bucketShard{
			entries: make(map[string]*bucketEntry),
			deltas:  make(map[string]*[numKinds]float64),
		}
	}
	return s
}

// Allow decides whether the key may use n units of the rule's kind now, and if so, uses them.
// Both the bucket (if the rule has one) and the daily cap (if it has one) must have room. The
// bucket is charged first and handed back if the daily cap denies, so the two locks are taken
// one after the other, never nested.
func (s *Service) Allow(key string, rule Rule, n int64) Decision {
	d := s.allow(key, rule, n)
	d.Kind = rule.Kind
	return d
}

// allow is Allow, without naming the decision's kind
func (s *Service) allow(key string, rule Rule, n int64) Decision {
	now := s.now()
	d := s.takeTokens(key, &rule, n, now)
	if !d.OK {
		return d
	}
	i := dailyIndex(rule.Kind)
	if i < 0 {
		return d
	}
	start := periodStart(key, now)
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
		return dailyDecision(false, rule.Daily, total, end.Sub(now))
	}
	c.unflushed[i] += n
	if rule.Threshold == 0 || total+n >= rule.Threshold {
		c.promoted = true
	}
	end := time.Unix(c.periodStart+daySeconds, 0)
	cs.mu.Unlock()
	if rule.Daily <= 0 {
		return d
	}
	return dailyDecision(true, rule.Daily, total+n, end.Sub(now))
}

// dailyDecision describes the daily cap after used units, with reset until the period ends
func dailyDecision(ok bool, daily, used int64, reset time.Duration) Decision {
	d := Decision{
		OK:        ok,
		Quota:     daily,
		Window:    oneDay,
		Remaining: max(0, daily-used),
		Reset:     reset,
	}
	if !ok {
		d.RetryAfter = reset
	}
	return d
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
	b := e.refillNoLock(&rule, now)
	return int64(math.Max(0, math.Floor(b.tokens)))
}

// Check decides like Allow whether the key's bucket for the rule has one unit now, without using
// it. Like Tokens, a rule without a bucket admits nothing. It is for "may I try" checks that charge
// only on failure, and still need to tell the client how long to wait.
func (s *Service) Check(key string, rule Rule) Decision {
	d := s.check(key, rule)
	d.Kind = rule.Kind
	return d
}

// check is Check, without naming the decision's kind
func (s *Service) check(key string, rule Rule) Decision {
	if rule.Burst <= 0 {
		return Decision{}
	}
	now := s.now()
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	e, ok := bs.entries[key]
	if !ok {
		return Decision{OK: true} // A fresh bucket is full
	}
	b := e.refillNoLock(&rule, now)
	if b.tokens < 1 {
		return Decision{RetryAfter: b.wait(1, &rule)}
	}
	return Decision{OK: true}
}

// Refund hands n units of the kind back to the key, e.g. for bytes that were reserved but not
// sent. The bucket cannot exceed its burst, and the daily count cannot go below zero.
func (s *Service) Refund(key string, kind Kind, n int64) {
	s.refundTokens(key, kind, n)
	i := dailyIndex(kind)
	if i < 0 {
		return
	}
	start := periodStart(key, s.now())
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok && c.periodStart >= start {
		c.unflushed[i] = max(-c.base[i], c.unflushed[i]-n) // A negative unflushed is a refund still to be written
	}
}

// Usage returns the key's daily counts in its current period, from memory: all nodes' as of the
// last sync, plus this node's since. The other kinds have only buckets (see Tokens).
func (s *Service) Usage(key string) Usage {
	start := periodStart(key, s.now())
	var u Usage
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok && c.periodStart >= start {
		for i := range c.base {
			u.counts[i] = c.base[i] + c.unflushed[i]
		}
	}
	return u
}

// ResetAt returns when the key's daily counts start over: the end of its current period
func (s *Service) ResetAt(key string) time.Time {
	return periodEnd(key, s.now())
}

// Prune forgets the buckets of keys that are settled (see bucketEntry.settledNoLock) and have not
// been used for settledIdle, and the daily counts of periods that have ended. It returns how many
// keys' buckets it forgot.
func (s *Service) Prune() int {
	now := s.now()
	cutoff := now.Add(-settledIdle).UnixNano()
	pruned := 0
	for _, bs := range s.buckets {
		bs.mu.Lock()
		for key, e := range bs.entries {
			if e.seen < cutoff && e.settledNoLock(now) {
				delete(bs.entries, key)
				pruned++
			}
		}
		bs.mu.Unlock()
	}
	for _, cs := range s.counters {
		cs.mu.Lock()
		for key, c := range cs.counters {
			if c.periodStart < periodStart(key, now) {
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

// reset fills the key's bucket of the kind and zeroes its daily count (see limiter.Reset)
func (s *Service) reset(key string, kind Kind) {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	if e, ok := bs.entries[key]; ok {
		e.buckets[kind].tokens = float64(e.buckets[kind].burst)
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
