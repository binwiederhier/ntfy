// Package limit answers one question: may key K use N units of kind X? A key is a visitor
// identity (see package visitor), a kind is what is being counted (requests, messages, bytes, ...),
// and a Rule, passed in on every call, says how much is allowed: a token bucket, a daily cap, or
// both. Rules are not baked into the state, so a changed rule (a tier change) applies on the next
// call without rebuilding anything and without handing back a full bucket.
//
// The service knows nothing about users, tiers or HTTP. All state is in memory, sharded by key.
package limit

import (
	"math"
	"sync"
	"time"

	"heckel.io/ntfy/v2/util"
)

const (
	shardCount = 64 // Keys are spread over this many independently locked maps

	// settledIdle is how long a settled key (see Prune) must go unused before it is forgotten, so
	// a key in steady use is not dropped and recreated over and over
	settledIdle = time.Minute
)

// Service keeps the per-key state and decides on usage. See the package comment.
type Service struct {
	conf   *Config
	now    func() time.Time // Injectable clock, for tests
	shards [shardCount]*shard
}

// shard is one independently locked part of the key space
type shard struct {
	entries map[string]*entry
	mu      sync.Mutex // Protects entries and everything inside them
}

// entry is one key's state: a bucket and a daily count per kind, and when it was last used
type entry struct {
	buckets [numKinds]bucket
	counts  [numKinds]int64 // Units used today, per kind
	seen    int64           // Unix nanoseconds of the last use, for pruning
	full    int64           // Unix nanoseconds from which every bucket is full again, for pruning
	capped  uint32          // Kinds (bits) used under a daily cap or seeded, whose counts forgetting would lose
}

// bucket is a token bucket. It is created full: last is zero until the first use, and tokens
// then start at the rule's burst. Tokens may go negative (debt) when peers' usage is charged in.
type bucket struct {
	tokens float64
	last   int64 // Unix nanoseconds of the last refill, 0 if never used
	burst  int64 // Burst of the rule the bucket was last used with; caps refunds and debt
}

// New creates the service
func New(conf *Config) *Service {
	return newService(conf, time.Now)
}

func newService(conf *Config, now func() time.Time) *Service {
	s := &Service{
		conf: conf,
		now:  now,
	}
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
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e := sh.entryNoLock(key, now)
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
	if rule.Daily > 0 && e.counts[rule.Kind]+n > rule.Daily {
		return Decision{RetryAfter: untilReset(now, s.conf.ResetTime)}
	}
	if rule.Burst > 0 {
		b.tokens -= float64(n)
		e.noteFull(b, &rule)
	}
	if rule.Daily > 0 {
		e.capped |= 1 << rule.Kind
	}
	e.counts[rule.Kind] += n
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
	e.noteFull(b, &rule) // The rule may refill slower than the one the bucket was charged with
	return int64(math.Max(0, math.Floor(b.tokens)))
}

// Refund hands n units of the kind back to the key, e.g. for bytes that were reserved but not
// sent. The bucket cannot exceed its burst, and the daily count cannot go below zero.
func (s *Service) Refund(key string, kind Kind, n int64) {
	sh := s.shards[shardIndex(key)]
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
	e.counts[kind] = max(0, e.counts[kind]-n)
}

// Usage returns the key's usage today, of every kind
func (s *Service) Usage(key string) Usage {
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return Usage{}
	}
	return Usage{counts: e.counts}
}

// Seed sets the key's daily counts, but only if the key is unknown to the service. It carries
// counts persisted elsewhere (the user's stats) into a fresh key.
func (s *Service) Seed(key string, usage Usage) {
	now := s.now()
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if _, ok := sh.entries[key]; ok {
		return
	}
	e := sh.entryNoLock(key, now)
	e.counts = usage.counts
	for kind, count := range usage.counts {
		if count > 0 {
			e.capped |= 1 << kind
		}
	}
}

// ResetUsage zeroes the daily counts of every key (the daily reset)
func (s *Service) ResetUsage() {
	for _, sh := range s.shards {
		sh.mu.Lock()
		for _, e := range sh.entries {
			e.counts = [numKinds]int64{}
		}
		sh.mu.Unlock()
	}
}

// Prune forgets keys that have not been used for longer than idle, and settled keys that have
// not been used for settledIdle, and returns how many. A key that comes back starts with full
// buckets and zero counts, so forgetting a settled key (every bucket full again, no counts today)
// changes nothing; idle is only the backstop for keys that never settle (e.g. no refill).
func (s *Service) Prune(idle time.Duration) int {
	now := s.now().UnixNano()
	cutoff := now - int64(idle)
	settledCutoff := now - int64(settledIdle)
	pruned := 0
	for _, sh := range s.shards {
		sh.mu.Lock()
		for key, e := range sh.entries {
			if e.seen < cutoff || (e.seen < settledCutoff && e.settled(now)) {
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
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.entryNoLock(key, now).seen = now.UnixNano()
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

// Limiter returns a util.Limiter view of the key's bucket for the rule, for the readers and
// writers in util that meter bytes. A negative AllowN refunds.
func (s *Service) Limiter(key string, rule Rule) util.Limiter {
	return &limiter{
		s:    s,
		key:  key,
		rule: rule,
	}
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

// reset fills the key's bucket of the kind and zeroes its count (see limiter.Reset)
func (s *Service) reset(key string, kind Kind) {
	sh := s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.entries[key]
	if !ok {
		return
	}
	e.buckets[kind].tokens = float64(e.buckets[kind].burst)
	e.counts[kind] = 0
}

// entryNoLock returns the key's entry, creating it if needed. The caller must hold sh.mu.
func (sh *shard) entryNoLock(key string, now time.Time) *entry {
	e, ok := sh.entries[key]
	if !ok {
		e = &entry{seen: now.UnixNano()}
		sh.entries[key] = e
	}
	return e
}

// noteFull moves the time from which every bucket is full again to when this bucket is full,
// if that is later. It only ever moves forward, so it may be late but is never early. The
// bucket must have been refilled with the rule.
func (e *entry) noteFull(b *bucket, rule *Rule) {
	missing := float64(b.burst) - b.tokens
	if missing <= 0 {
		return
	}
	full := int64(math.MaxInt64) // A bucket without refill is never full again
	if rule.Refill > 0 {
		full = b.last + int64(math.Ceil(missing*float64(rule.Refill)))
	}
	if full > e.full {
		e.full = full
	}
}

// settled returns true if forgetting the entry changes nothing: every bucket is full again, and
// no kind under a daily cap has counts today. Counts of uncapped kinds are only a tally.
func (e *entry) settled(now int64) bool {
	if now < e.full {
		return false
	}
	for kind, count := range e.counts {
		if count > 0 && e.capped&(1<<kind) != 0 {
			return false
		}
	}
	return true
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
