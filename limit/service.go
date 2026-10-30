// Package limit answers one question: may key K use N units of kind X? A key is a visitor
// identity (see package visitor), a kind is what is being counted (requests, messages, bytes, ...),
// and a Rule, passed in on every call, says how much is allowed: a token bucket, a daily cap, or
// both. Rules are not baked into the state, so a changed rule (a tier change) applies on the next
// call without rebuilding anything and without handing back a full bucket.
//
// The service knows nothing about users, tiers or HTTP. All state is in memory, sharded by key:
// token buckets per node, and daily counts (messages, emails, calls) per key and 24h period. A
// key's period starts at an offset hashed from the key (see periodStart), so the resets of all
// keys are spread over the day, lazily, on the key's next use.
package limit

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/util"
)

const (
	shardCount = 64 // Keys are spread over this many independently locked maps
)

// Service decides on usage from memory; see the package comment
type Service struct {
	conf        *Config
	now         func() time.Time          // Injectable clock, for tests
	counters    [shardCount]*counterShard // Daily counts, kept until their period ends
	buckets     [shardCount]*bucketShard  // Token buckets, per node, evicted when idle and full
	clockOffset atomic.Int64              // Database clock minus ours, nanoseconds
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
		conf: conf,
		now:  now,
	}
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
		cs.mu.Unlock()
		if rule.Burst > 0 {
			s.refundTokens(key, rule.Kind, n)
		}
		return Decision{RetryAfter: time.Unix(c.periodStart+daySeconds, 0).Sub(dbNow)}
	}
	c.unflushed[i] += n
	cs.mu.Unlock()
	return Decision{OK: true}
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
	i := dailyIndex(kind)
	if i < 0 {
		return
	}
	start := periodStart(key, s.dbTime(s.now()))
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.counters[key]; ok && c.periodStart >= start {
		c.unflushed[i] = max(-c.base[i], c.unflushed[i]-n) // A negative unflushed is a refund still to be written
	}
}

// Usage returns the key's daily counts in its current period, from memory
func (s *Service) Usage(key string) Usage {
	start := periodStart(key, s.dbTime(s.now()))
	cs := s.counters[shardIndex(key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.counters[key]
	if !ok || c.periodStart < start {
		return Usage{}
	}
	var u Usage
	for i := 0; i < numDaily; i++ {
		u.counts[i] = c.base[i] + c.unflushed[i]
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

// takeTokens marks the key as used and takes n tokens from its bucket for the rule, if the rule
// has one and it has room
func (s *Service) takeTokens(key string, rule *Rule, n int64, now time.Time) Decision {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	e := bs.entryNoLock(key, now)
	e.seen = now.UnixNano()
	if rule.Burst <= 0 {
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
