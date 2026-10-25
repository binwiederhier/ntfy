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
)

// Service keeps the per-key state and decides on usage. See the package comment.
type Service struct {
	conf   *Config
	now    func() time.Time // Injectable clock, for tests
	shards [shardCount]shard
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
		s.shards[i].entries = make(map[string]*entry)
	}
	return s
}

// Allow decides whether the key may use n units of the rule's kind now, and if so, uses them.
// Both the bucket (if the rule has one) and the daily cap (if it has one) must have room.
func (s *Service) Allow(key string, rule Rule, n int64) Decision {
	now := s.now()
	sh := &s.shards[shardIndex(key)]
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
	e.counts[kind] = max(0, e.counts[kind]-n)
}

// Usage returns the key's usage today, of every kind
func (s *Service) Usage(key string) Usage {
	sh := &s.shards[shardIndex(key)]
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
	sh := &s.shards[shardIndex(key)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if _, ok := sh.entries[key]; ok {
		return
	}
	e := sh.entryNoLock(key, now)
	e.counts = usage.counts
}

// ResetUsage zeroes the daily counts of every key (the daily reset)
func (s *Service) ResetUsage() {
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for _, e := range sh.entries {
			e.counts = [numKinds]int64{}
		}
		sh.mu.Unlock()
	}
}

// Prune forgets keys that have not been used for longer than idle. A key that comes back starts
// with full buckets and zero counts.
func (s *Service) Prune(idle time.Duration) {
	cutoff := s.now().Add(-idle).UnixNano()
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for key, e := range sh.entries {
			if e.seen < cutoff {
				delete(sh.entries, key)
			}
		}
		sh.mu.Unlock()
	}
}

// Limiter returns a util.Limiter view of the key's bucket for the rule, for the readers and
// writers in util that meter bytes. A negative AllowN refunds.
func (s *Service) Limiter(key string, rule Rule) util.Limiter {
	return &limiter{s: s, key: key, rule: rule}
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
