package limit

import (
	"math"
	"sync"
	"time"
)

// bucketShard is one independently locked part of the token buckets
type bucketShard struct {
	entries map[string]*bucketEntry
	deltas  map[string]*[numKinds]float64 // Tokens used here per kind since the last BucketDeltas, for the peers; empty unless Config.TrackDeltas
	mu      sync.Mutex                    // Protects entries, deltas and everything inside them
}

// bucketEntry is one key's token buckets
type bucketEntry struct {
	buckets [numKinds]bucket
	seen    int64 // Last use, unix nanoseconds
	full    int64 // When every bucket is full again, unix nanoseconds
}

// bucket is a token bucket. It is created full: last is zero until the first use, and tokens
// then start at the rule's burst. Tokens may go negative (debt) when a burst shrinks.
type bucket struct {
	tokens     float64
	last       int64         // Unix nanoseconds of the last refill, 0 if never used
	burst      int64         // Burst of the rule the bucket was last used with; caps refunds and debt
	refillTime time.Duration // Refill of the rule the bucket was last used with, to charge peers' usage without a rule
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
	b := e.refillNoLock(rule, now)
	if n > rule.Burst {
		return b.decision(Decision{}, rule) // Can never fit, so there is no time to wait for
	} else if b.tokens < float64(n) {
		return b.decision(Decision{RetryAfter: b.wait(n, rule)}, rule)
	}
	b.tokens -= float64(n)
	e.noteFull(b, rule)
	if s.trackDeltas {
		bs.noteDeltaNoLock(key, rule.Kind, float64(n))
	}
	return b.decision(Decision{OK: true}, rule)
}

// refundTokens hands n tokens back to the key's bucket of the kind, up to its burst
func (s *Service) refundTokens(key string, kind Kind, n int64) {
	bs := s.buckets[shardIndex(key)]
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if e, ok := bs.entries[key]; ok {
		b := &e.buckets[kind]
		if b.last != 0 {
			before := b.tokens
			b.tokens = math.Min(b.tokens+float64(n), float64(b.burst))
			if s.trackDeltas {
				bs.noteDeltaNoLock(key, kind, before-b.tokens)
			}
		}
	}
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

// settledNoLock returns true if forgetting the entry changes nothing: every bucket is full again,
// and a forgotten key comes back with full buckets. The caller must hold the shard's lock.
func (e *bucketEntry) settledNoLock(now time.Time) bool {
	return now.UnixNano() >= e.full
}

// refillNoLock refills the entry's bucket for the rule and notes when it is full again: the rule
// may refill slower than the one the bucket was last charged with, also on a denied call. The
// caller must hold the shard's lock.
func (e *bucketEntry) refillNoLock(rule *Rule, now time.Time) *bucket {
	b := &e.buckets[rule.Kind]
	b.refill(now, rule)
	e.noteFull(b, rule)
	return b
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
	b.refillTime = rule.Refill
}

// decision adds the bucket's quota to d: the whole tokens left, and the time until the next one,
// within which the client can use no more than those (the "t" of the RateLimit header)
func (b *bucket) decision(d Decision, rule *Rule) Decision {
	d.Quota = rule.Burst
	d.Window = time.Duration(rule.Burst) * rule.Refill
	d.Remaining = max(0, int64(math.Floor(b.tokens)))
	d.Reset = b.wait(d.Remaining+1, rule)
	return d
}

// wait returns how long until the bucket has n tokens, or 0 if it never will (no refill)
func (b *bucket) wait(n int64, rule *Rule) time.Duration {
	if rule.Refill <= 0 {
		return 0
	}
	return time.Duration((float64(n) - b.tokens) * float64(rule.Refill))
}
