package limit

import (
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestService_BucketDeltas(t *testing.T) {
	// Every bucket's usage since the last call is shared, for every kind and every key, also a
	// client far below its burst: spread over N nodes, a client at N times its rate drains none
	// of their buckets on its own. Refunds net out, and every call starts over.
	s := newTestService(t)
	s.TrackBucketDeltas()
	requests := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	bandwidth := Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Minute,
	}
	auth := Rule{
		Kind:   KindAuth,
		Burst:  30,
		Refill: 2 * time.Second,
	}
	require.True(t, s.Allow("ip:1.2.3.4", requests, 1).OK)
	require.True(t, s.Allow("ip:1.2.3.4", requests, 1).OK)
	require.True(t, s.Allow("ip:1.2.3.4", bandwidth, 300).OK)
	s.Refund("ip:1.2.3.4", KindBandwidth, 100)
	require.True(t, s.Allow("user:u_abc", auth, 1).OK)
	require.Equal(t, SharedBuckets{
		"ip:1.2.3.4": {
			KindRequests: {
				Used:   2,
				Burst:  60,
				Refill: 5 * time.Second,
			},
			KindBandwidth: {
				Used:   200,
				Burst:  1000,
				Refill: time.Minute,
			},
		},
		"user:u_abc": {
			KindAuth: {
				Used:   1,
				Burst:  30,
				Refill: 2 * time.Second,
			},
		},
	}, s.BucketDeltas().Buckets)
	require.Empty(t, s.BucketDeltas().Buckets)

	// Only what was used since the last call
	require.True(t, s.Allow("ip:1.2.3.4", requests, 1).OK)
	require.Equal(t, 1.0, s.BucketDeltas().Buckets["ip:1.2.3.4"][KindRequests].Used)
}

func TestService_BucketDeltas_NotCountedWithoutSharing(t *testing.T) {
	// Without a cluster nothing takes the deltas, so they are not kept
	s := newTestService(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	for i := 0; i < 40; i++ {
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
	}
	s.Refund("ip:1.2.3.4", KindRequests, 10)
	require.Empty(t, s.BucketDeltas().Buckets)
}

func TestService_ChargeBuckets(t *testing.T) {
	// What the peers used comes off this node's bucket, down to a debt of one burst, so a burst
	// spread over several nodes cannot lock a key (or the NAT behind it) out for long
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	require.True(t, s.Allow("ip:1.2.3.4", rule, 10).OK)
	s.ChargeBuckets("peer", requestUsage("ip:1.2.3.4", 30))
	require.Equal(t, int64(20), s.Tokens("ip:1.2.3.4", rule))

	s.ChargeBuckets("peer", requestUsage("ip:1.2.3.4", 1000))
	require.Equal(t, int64(0), s.Tokens("ip:1.2.3.4", rule))
	require.False(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
	clock.advance(5*time.Minute + 5*time.Second) // 61 tokens: the debt of 60, then one
	require.Equal(t, int64(1), s.Tokens("ip:1.2.3.4", rule))

	// A peer's refund hands tokens back, up to the burst
	s.ChargeBuckets("peer", requestUsage("ip:1.2.3.4", -10))
	require.Equal(t, int64(11), s.Tokens("ip:1.2.3.4", rule))
}

func TestService_ChargeBuckets_UnknownKey(t *testing.T) {
	// A key only a peer has seen is charged too, with the peer's rule: this node then knows it
	// when the client comes here, and can hand it on to a node that restarts (BucketSnapshot)
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	s.ChargeBuckets("peer", requestUsage("ip:5.6.7.8", 50))
	require.Equal(t, int64(10), s.Tokens("ip:5.6.7.8", rule))
	clock.advance(10 * time.Second)
	require.Equal(t, int64(12), s.Tokens("ip:5.6.7.8", rule))
}

func TestService_ChargeBuckets_NotShared(t *testing.T) {
	// Peers' usage is theirs to share: charging it here does not make it this node's
	s := newTestService(t)
	s.TrackBucketDeltas()
	s.ChargeBuckets("peer", requestUsage("ip:1.2.3.4", 50))
	require.Empty(t, s.BucketDeltas().Buckets)
}

func TestService_BucketSnapshot_Restore(t *testing.T) {
	// A restarted node takes over the buckets a peer holds: every bucket that is not full, as the
	// tokens used below its burst. Full buckets are what a fresh node has anyway.
	a, clockA := newTestServiceWithClock(t)
	requests := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	emails := Rule{
		Kind:   KindEmails,
		Burst:  5,
		Refill: oneDay / 5,
	}
	require.True(t, a.Allow("ip:1.2.3.4", requests, 45).OK)
	require.True(t, a.Allow("ip:1.2.3.4", emails, 3).OK)
	require.True(t, a.Allow("ip:5.6.7.8", requests, 1).OK)
	clockA.advance(5 * time.Second) // ip:5.6.7.8 is full again
	snapshot := a.BucketSnapshot()
	require.Equal(t, SharedBuckets{
		"ip:1.2.3.4": {
			KindRequests: {
				Used:   44,
				Burst:  60,
				Refill: 5 * time.Second,
			},
			KindEmails: {
				Used:   3,
				Burst:  5,
				Refill: oneDay / 5,
			},
		},
	}, roundUsed(snapshot.Buckets))

	b := newTestService(t)
	b.RestoreBuckets("a", snapshot)
	require.Equal(t, int64(16), b.Tokens("ip:1.2.3.4", requests))
	require.Equal(t, int64(2), b.Tokens("ip:1.2.3.4", emails))
	require.Equal(t, int64(60), b.Tokens("ip:5.6.7.8", requests))

	// Every peer holds the whole cluster's buckets, so a restore takes the most used, never the sum
	b.RestoreBuckets("a", snapshot)
	require.Equal(t, int64(16), b.Tokens("ip:1.2.3.4", requests))
	require.True(t, b.Allow("ip:1.2.3.4", requests, 10).OK)
	b.RestoreBuckets("a", snapshot)
	require.Equal(t, int64(6), b.Tokens("ip:1.2.3.4", requests))
}

// requestUsage is a round of a peer's usage of the key's request bucket (burst 60, one token per 5 s)
func requestUsage(key string, used float64) *BucketRound {
	return testRound(SharedBuckets{
		key: {
			KindRequests: {
				Used:   used,
				Burst:  60,
				Refill: 5 * time.Second,
			},
		},
	})
}

// testRounds numbers the rounds of testRound
var testRounds atomic.Uint64

// testRound is the next round of the test peer, with the given buckets
func testRound(buckets SharedBuckets) *BucketRound {
	return &BucketRound{
		Epoch:   1,
		Round:   testRounds.Add(1),
		Buckets: buckets,
	}
}

// roundUsed rounds the used tokens, which a snapshot taken between refills has fractions of
func roundUsed(buckets SharedBuckets) SharedBuckets {
	for _, kinds := range buckets {
		for _, u := range kinds {
			u.Used = math.Round(u.Used)
		}
	}
	return buckets
}

func TestService_BucketSnapshot_LeavesOutUnsent(t *testing.T) {
	// What this node used since its last BucketDeltas is not in its snapshot: the next round sends
	// it to the restarted peer, which would otherwise count it twice
	a := newTestService(t)
	a.TrackBucketDeltas()
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: time.Hour,
	}
	require.True(t, a.Allow("ip:1.2.3.4", rule, 10).OK)
	a.BucketDeltas() // Sent
	require.True(t, a.Allow("ip:1.2.3.4", rule, 4).OK)
	snapshot := a.BucketSnapshot()
	require.Equal(t, 10.0, math.Round(snapshot.Buckets["ip:1.2.3.4"][KindRequests].Used))

	b := newTestService(t)
	b.RestoreBuckets("a", snapshot)
	b.ChargeBuckets("a", a.BucketDeltas())
	require.Equal(t, int64(46), b.Tokens("ip:1.2.3.4", rule))
}

func TestService_ChargeBuckets_GenerousRule(t *testing.T) {
	// A bucket known here only from a peer follows the peer's rule when it changes (a tier upgrade
	// there); of two rules, the more generous one applies, which undercounts rather than locks out
	s := newTestService(t)
	slow := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: time.Hour,
	}
	fast := Rule{
		Kind:   KindRequests,
		Burst:  1000,
		Refill: time.Second,
	}
	s.ChargeBuckets("peer", testRound(SharedBuckets{
		"user:u_abc": {
			KindRequests: {
				Used:   50,
				Burst:  slow.Burst,
				Refill: slow.Refill,
			},
		},
	}))
	s.ChargeBuckets("peer", testRound(SharedBuckets{
		"user:u_abc": {
			KindRequests: {
				Used:   100,
				Burst:  fast.Burst,
				Refill: fast.Refill,
			},
		},
	}))
	require.Equal(t, int64(850), s.Tokens("user:u_abc", fast))
}

func TestService_ChargeBuckets_InvalidRule(t *testing.T) {
	// A peer's usage with a rule no node would use (a bug, or a bad peer) is ignored rather than
	// creating a bucket that locks the key out
	s := newTestService(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	s.ChargeBuckets("peer", testRound(SharedBuckets{
		"ip:1.2.3.4": {
			KindRequests: {
				Used:   1e15,
				Burst:  1e15,
				Refill: 5 * time.Second,
			},
		},
		"ip:5.6.7.8": {
			KindRequests: {
				Used:   50,
				Burst:  60,
				Refill: -time.Second,
			},
		},
	}))
	require.Equal(t, int64(60), s.Tokens("ip:1.2.3.4", rule))
	require.Equal(t, int64(60), s.Tokens("ip:5.6.7.8", rule))
}

func TestService_ChargeBuckets_UnknownKind(t *testing.T) {
	// A newer peer's kind is skipped, and the rest of its round still counts
	s := newTestService(t)
	var buckets SharedBuckets
	require.Nil(t, json.Unmarshal([]byte(`{"ip:1.2.3.4":{"requests":{"u":50,"b":60,"r":5000000000},"teleports":{"u":3,"b":5,"r":1000000000}}}`), &buckets))
	s.ChargeBuckets("peer", testRound(buckets))
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	require.Equal(t, int64(10), s.Tokens("ip:1.2.3.4", rule))
}

func TestService_ChargeBuckets_LateRoundSkipped(t *testing.T) {
	// Rounds are sent concurrently, so one can overtake another. A round older than the last one
	// taken from its node is skipped: a refund that overtook its charge must not be clipped at the
	// full bucket and then followed by the charge.
	a := newTestService(t)
	a.TrackBucketDeltas()
	b := newTestService(t)
	rule := Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Hour,
	}
	require.True(t, a.Allow("ip:1.2.3.4", rule, 600).OK)
	charge := a.BucketDeltas()
	a.Refund("ip:1.2.3.4", KindBandwidth, 600)
	refund := a.BucketDeltas()
	b.ChargeBuckets("a", refund)
	b.ChargeBuckets("a", charge) // Late
	require.Equal(t, int64(1000), b.Tokens("ip:1.2.3.4", rule))
}

func TestService_RestoreBuckets_SkipsRoundsInSnapshot(t *testing.T) {
	// A round taken before the snapshot is in the snapshot, also if it is still on its way and
	// arrives after the restarted node took the snapshot over; only later rounds are charged
	a := newTestService(t)
	a.TrackBucketDeltas()
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: time.Hour,
	}
	require.True(t, a.Allow("ip:1.2.3.4", rule, 10).OK)
	inFlight := a.BucketDeltas()
	snapshot := a.BucketSnapshot()
	b := newTestService(t)
	b.RestoreBuckets("a", snapshot)
	b.ChargeBuckets("a", inFlight)
	require.Equal(t, int64(50), b.Tokens("ip:1.2.3.4", rule))

	require.True(t, a.Allow("ip:1.2.3.4", rule, 4).OK)
	b.ChargeBuckets("a", a.BucketDeltas())
	require.Equal(t, int64(46), b.Tokens("ip:1.2.3.4", rule))
}

func TestService_ChargeBuckets_RestartedPeer(t *testing.T) {
	// A peer that restarted numbers its rounds from one again, in a later epoch; they count
	a, clock := newTestServiceWithClock(t)
	a.TrackBucketDeltas()
	b := newTestService(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: time.Hour,
	}
	for i := 0; i < 3; i++ {
		require.True(t, a.Allow("ip:1.2.3.4", rule, 1).OK)
		b.ChargeBuckets("a", a.BucketDeltas())
	}
	clock.advance(time.Second)
	restarted := newService(nil, clock.now)
	restarted.TrackBucketDeltas()
	require.True(t, restarted.Allow("ip:1.2.3.4", rule, 5).OK)
	b.ChargeBuckets("a", restarted.BucketDeltas())
	require.Equal(t, int64(52), b.Tokens("ip:1.2.3.4", rule))
}

func TestService_BucketDeltas_EntryCap(t *testing.T) {
	// A round is capped by its buckets, not its keys: a key with several kinds weighs more on the wire
	s := newTestService(t)
	s.TrackBucketDeltas()
	requests := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	topics := Rule{
		Kind:   KindTopics,
		Burst:  100,
		Refill: time.Minute,
	}
	for i := 0; i < 30000; i++ {
		key := fmt.Sprintf("ip:10.%d.%d.%d", i>>16&255, i>>8&255, i&255)
		require.True(t, s.Allow(key, requests, 1).OK)
		require.True(t, s.Allow(key, topics, 1).OK)
	}
	entries := 0
	for _, kinds := range s.BucketDeltas().Buckets {
		entries += len(kinds)
	}
	require.Equal(t, BucketDeltasMax, entries)
}
