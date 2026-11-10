package limit

import (
	"math"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	// BucketDeltasMax bounds the buckets BucketDeltas returns per round, so a flood of fresh keys
	// cannot make one round's message to the peers unbounded (~80 bytes per bucket on the wire)
	BucketDeltasMax = 50000

	// peerBurstMax is the largest burst a peer's usage may carry (10 TB, for bandwidth buckets);
	// usage with a larger one, or a negative refill, is a bug or a bad peer, and is ignored
	peerBurstMax = 10_000_000_000_000
)

// BucketDeltas returns the tokens this node used per key and kind since the last call (refunds
// netted out), each with its bucket's burst and refill, so a peer can charge them without the
// visitor's rule, and starts over. Every key is in it, not only the ones being drained: spread over
// N nodes, a client at N times its rate drains no node's bucket on its own. A lost round undercounts,
// except for a refund of a charge sent in an earlier round, which then stays charged on the peers
// until the bucket refills; refunds almost always come in the same round as their charge.
func (s *Service) BucketDeltas() Buckets {
	deltas := make(Buckets)
	entries, dropped := 0, 0
	for _, bs := range s.buckets {
		bs.mu.Lock()
		for key, d := range bs.deltas {
			e, ok := bs.entries[key]
			if !ok {
				continue // Pruned since
			}
			for k := range d {
				if d[k] == 0 {
					continue
				} else if entries >= BucketDeltasMax {
					dropped++ // A round that would be too large: the peers undercount
					continue
				}
				entries++
				if deltas[key] == nil {
					deltas[key] = make(map[Kind]*BucketUsage)
				}
				deltas[key][Kind(k)] = e.buckets[k].usage(d[k])
			}
		}
		if len(bs.deltas) > 0 {
			bs.deltas = make(map[string]*[numKinds]float64)
		}
		bs.mu.Unlock()
	}
	if dropped > 0 {
		log.Tag(tag).Warn("Not sharing %d bucket(s) this round, more than %d were used", dropped, BucketDeltasMax)
	}
	return deltas
}

// ChargeBuckets takes what peers used (their BucketDeltas) from the keys' buckets here, down to a
// debt of one burst: an abuser spread over the nodes then gets the configured rate from the
// cluster, while a burst cannot lock a key, or the neighbors behind its NAT, out for longer than one
// burst takes to refill. A key this node has not seen gets a bucket with the peer's rule, so it is
// known here when the client comes, and when a restarting peer asks (BucketSnapshot).
func (s *Service) ChargeBuckets(buckets Buckets) {
	now := s.now()
	for key, kinds := range buckets {
		bs := s.buckets[shardIndex(key)]
		bs.mu.Lock()
		e := bs.entryNoLock(key, now)
		for kind, u := range kinds {
			if b, rule := e.peerBucketNoLock(kind, u, now); b != nil {
				b.tokens = math.Min(math.Max(b.tokens-u.Used, -float64(rule.Burst)), float64(rule.Burst))
				e.noteFull(b, rule)
			}
		}
		bs.mu.Unlock()
	}
}

// BucketSnapshot returns every bucket that is not full, as the tokens used below its burst, for a
// peer that restarts (RestoreBuckets). A full bucket is what a fresh node has anyway. What this node
// used since its last BucketDeltas is left out: the next round sends it to the peer, which would
// otherwise count it twice.
func (s *Service) BucketSnapshot() Buckets {
	now := s.now()
	snapshot := make(Buckets)
	for _, bs := range s.buckets {
		bs.mu.Lock()
		for key, e := range bs.entries {
			for k := range e.buckets {
				b := &e.buckets[k]
				if b.last == 0 || b.burst <= 0 {
					continue
				}
				e.refillNoLock(b.rule(Kind(k)), now)
				used := float64(b.burst) - b.tokens
				if d, ok := bs.deltas[key]; ok {
					used -= d[k]
				}
				if used <= 0 {
					continue
				}
				if snapshot[key] == nil {
					snapshot[key] = make(map[Kind]*BucketUsage)
				}
				snapshot[key][Kind(k)] = b.usage(used)
			}
		}
		bs.mu.Unlock()
	}
	return snapshot
}

// RestoreBuckets takes over a peer's BucketSnapshot. Every peer holds the whole cluster's buckets
// (ChargeBuckets), so a bucket keeps the most used of here and the snapshot, never the sum: a
// node that restores from two peers, or got a delta before the snapshot, does not count twice.
// It must run before the node accepts its peers' rounds: a round the snapshot contains would then
// arrive again and count twice.
func (s *Service) RestoreBuckets(snapshot Buckets) {
	now := s.now()
	for key, kinds := range snapshot {
		bs := s.buckets[shardIndex(key)]
		bs.mu.Lock()
		e := bs.entryNoLock(key, now)
		for kind, u := range kinds {
			if b, rule := e.peerBucketNoLock(kind, u, now); b != nil && float64(rule.Burst)-b.tokens < u.Used {
				b.tokens = math.Max(float64(rule.Burst)-u.Used, -float64(rule.Burst))
				e.noteFull(b, rule)
			}
		}
		bs.mu.Unlock()
	}
}

// noteDeltaNoLock adds n used tokens (negative: refunded) of the kind to what the next round sends the
// peers. The caller must hold bs.mu.
func (bs *bucketShard) noteDeltaNoLock(key string, kind Kind, n float64) {
	if n == 0 {
		return
	}
	d, ok := bs.deltas[key]
	if !ok {
		d = &[numKinds]float64{}
		bs.deltas[key] = d
	}
	d[kind] += n
}

// peerBucketNoLock refills the entry's bucket of the kind for a peer's usage and returns it with the
// rule to charge it by: the more generous of the peer's rule and the one the bucket was last used
// with, so a tier upgrade on the peer applies here too, and two rules that disagree undercount rather
// than lock the key out. When the client comes here, this node's own rule applies again. It returns
// nil for a kind this node does not know (a newer peer) or a usage without a valid bucket. The caller
// must hold the shard's lock.
func (e *bucketEntry) peerBucketNoLock(kind Kind, u *BucketUsage, now time.Time) (*bucket, *Rule) {
	if kind >= numKinds || u == nil || u.Burst <= 0 || u.Burst > peerBurstMax || u.Refill < 0 {
		return nil, nil
	}
	b := &e.buckets[kind]
	rule := &Rule{
		Kind:   kind,
		Burst:  u.Burst,
		Refill: u.Refill,
	}
	if b.last != 0 {
		rule.Burst = max(rule.Burst, b.burst)
		if b.refillTime > 0 && (rule.Refill == 0 || b.refillTime < rule.Refill) {
			rule.Refill = b.refillTime // A zero refill never refills, so any other is more generous
		}
	}
	e.refillNoLock(rule, now)
	return b, rule
}

// rule returns the rule the bucket was last used with
func (b *bucket) rule(kind Kind) *Rule {
	return &Rule{
		Kind:   kind,
		Burst:  b.burst,
		Refill: b.refillTime,
	}
}

// usage returns used tokens of the bucket, with its burst and refill, for the peers
func (b *bucket) usage(used float64) *BucketUsage {
	return &BucketUsage{
		Used:   used,
		Burst:  b.burst,
		Refill: b.refillTime,
	}
}
