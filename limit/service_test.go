package limit

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
)

// testClock is a clock the tests move by hand
type testClock struct {
	t  time.Time
	mu sync.Mutex
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestServiceWithClock(t *testing.T) (*Service, *testClock) {
	t.Helper()
	clock := &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	return newService(&Config{}, clock.now), clock
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, _ := newTestServiceWithClock(t)
	return s
}

func TestService_Allow_Bucket(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  3,
		Refill: 100 * time.Millisecond,
	}
	for i := 0; i < 3; i++ {
		require.True(t, s.Allow("k", rule, 1).OK)
	}
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 100*time.Millisecond, d.RetryAfter)
	clock.advance(50 * time.Millisecond)
	d = s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 50*time.Millisecond, d.RetryAfter)
	clock.advance(50 * time.Millisecond)
	require.True(t, s.Allow("k", rule, 1).OK)
	require.False(t, s.Allow("k", rule, 1).OK)

	// A full minute refills the bucket, but never past the burst
	clock.advance(time.Minute)
	require.Equal(t, int64(3), s.Tokens("k", rule))
	for i := 0; i < 3; i++ {
		require.True(t, s.Allow("k", rule, 1).OK)
	}
	require.False(t, s.Allow("k", rule, 1).OK)

	// Other keys are independent
	require.True(t, s.Allow("other", rule, 1).OK)
}

func TestService_Allow_Daily(t *testing.T) {
	s, _ := newTestServiceWithClock(t) // Noon UTC; "k" resets at 03:48:26 UTC
	rule := Rule{
		Kind:  KindMessages,
		Daily: 2,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 15*time.Hour+48*time.Minute+26*time.Second, d.RetryAfter)

	// Denied usage is not counted
	require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))
}

func TestService_Allow_BucketAndDaily(t *testing.T) {
	// Emails: a burst of 2 refilling slowly, and a daily cap of 3
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindEmails,
		Burst:  2,
		Refill: time.Minute,
		Daily:  3,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, time.Minute, d.RetryAfter) // The bucket is what denies
	clock.advance(time.Minute)
	require.True(t, s.Allow("k", rule, 1).OK)
	clock.advance(time.Hour)
	d = s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 14*time.Hour+47*time.Minute+26*time.Second, d.RetryAfter) // The daily cap is what denies now, until 03:48:26 UTC
	require.Equal(t, int64(3), s.Usage("k").Count(KindEmails))
}

func TestService_Allow_QuotaBucket(t *testing.T) {
	// The decision describes the bucket: whole tokens left, and the time to the next token
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  3,
		Refill: 100 * time.Millisecond,
	}
	expected := Decision{
		OK:        true,
		Kind:      KindRequests,
		Quota:     3,
		Window:    300 * time.Millisecond,
		Remaining: 2,
		Reset:     100 * time.Millisecond,
	}
	require.Equal(t, expected, s.Allow("k", rule, 1))
	s.Allow("k", rule, 1)
	expected.Remaining = 0
	require.Equal(t, expected, s.Allow("k", rule, 1))
	clock.advance(40 * time.Millisecond)
	expected = Decision{
		RetryAfter: 60 * time.Millisecond,
		Kind:       KindRequests,
		Quota:      3,
		Window:     300 * time.Millisecond,
		Remaining:  0,
		Reset:      60 * time.Millisecond, // Matches RetryAfter
	}
	require.Equal(t, expected, s.Allow("k", rule, 1))
}

func TestService_Allow_QuotaDaily(t *testing.T) {
	// The decision describes the daily cap: units left, and the time until the period ends
	s, _ := newTestServiceWithClock(t) // Noon UTC; "k" resets at 03:48:26 UTC
	rule := Rule{
		Kind:  KindMessages,
		Daily: 2,
	}
	reset := 15*time.Hour + 48*time.Minute + 26*time.Second
	expected := Decision{
		OK:        true,
		Kind:      KindMessages,
		Quota:     2,
		Window:    24 * time.Hour,
		Remaining: 1,
		Reset:     reset,
	}
	require.Equal(t, expected, s.Allow("k", rule, 1))
	expected.Remaining = 0
	require.Equal(t, expected, s.Allow("k", rule, 1))
	expected.OK = false
	expected.RetryAfter = reset
	require.Equal(t, expected, s.Allow("k", rule, 1))
}

func TestService_Allow_DailyDenialKeepsTokens(t *testing.T) {
	// A call the daily cap denies does not spend the bucket's tokens
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindEmails,
		Burst:  5,
		Refill: time.Hour,
		Daily:  2,
	}
	require.True(t, s.Allow("k", rule, 2).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	require.Equal(t, int64(3), s.Tokens("k", rule))
}

func TestService_Allow_N(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Millisecond,
	} // 1 kB burst, 1 kB/s
	require.True(t, s.Allow("k", rule, 600).OK)
	d := s.Allow("k", rule, 600)
	require.False(t, d.OK)
	require.Equal(t, 200*time.Millisecond, d.RetryAfter)
	clock.advance(200 * time.Millisecond)
	require.True(t, s.Allow("k", rule, 600).OK)
	require.Equal(t, int64(1200), s.Usage("k").Count(KindBandwidth))

	// More than the burst can never be allowed, so there is no retry time
	d = s.Allow("k", rule, 1001)
	require.False(t, d.OK)
	require.Equal(t, time.Duration(0), d.RetryAfter)
}

func TestService_Allow_RuleChangeKeepsSpent(t *testing.T) {
	// A tier change passes a new rule on the next call: what was spent stays spent, so a bigger
	// burst adds its difference and a smaller one takes it away (down to a debt of one burst)
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  10,
		Refill: time.Hour,
	}
	for i := 0; i < 8; i++ {
		require.True(t, s.Allow("k", rule, 1).OK)
	}
	bigger := Rule{
		Kind:   KindRequests,
		Burst:  100,
		Refill: time.Hour,
	}
	require.Equal(t, int64(92), s.Tokens("k", bigger)) // 8 spent
	require.True(t, s.Allow("k", bigger, 2).OK)        // 10 spent
	smaller := Rule{
		Kind:   KindRequests,
		Burst:  5,
		Refill: time.Hour,
	}
	require.Equal(t, int64(0), s.Tokens("k", smaller)) // 10 spent of 5: in debt
	require.False(t, s.Allow("k", smaller, 1).OK)
	require.Equal(t, int64(90), s.Tokens("k", bigger)) // Back up: still 10 spent

	// A fresh key with the smaller rule starts with its burst
	require.Equal(t, int64(5), s.Tokens("fresh", smaller))
	for i := 0; i < 5; i++ {
		require.True(t, s.Allow("fresh", smaller, 1).OK)
	}
	require.False(t, s.Allow("fresh", smaller, 1).OK)
	require.Equal(t, int64(95), s.Tokens("fresh", bigger)) // 5 spent
}

func TestService_Allow_DailyChangeAppliesAtOnce(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	require.True(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 5,
	}, 5).OK)
	require.False(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 5,
	}, 1).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 6,
	}, 1).OK) // Upgraded
	require.False(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 3,
	}, 1).OK) // Downgraded below the count
	require.Equal(t, int64(6), s.Usage("k").Count(KindMessages))
}

func TestService_Allow_Unlimited(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	for i := 0; i < 1000; i++ {
		require.True(t, s.Allow("k", Rule{Kind: KindBandwidth}, 1).OK)
	}
	require.Equal(t, int64(1000), s.Usage("k").Count(KindBandwidth))
}

func TestService_Allow_NeverRefills(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:  KindAccount,
		Burst: 2,
	} // No refill
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	clock.advance(24 * time.Hour)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, time.Duration(0), d.RetryAfter)
}

func TestService_Refund(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindMessages,
		Burst:  3,
		Refill: time.Hour,
		Daily:  100,
	}
	require.True(t, s.Allow("k", rule, 3).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	s.Refund("k", KindMessages, 1)
	require.True(t, s.Allow("k", rule, 1).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	require.Equal(t, int64(3), s.Usage("k").Count(KindMessages))

	// A refund cannot exceed the burst, nor make the count negative
	s.Refund("k", KindMessages, 100)
	require.Equal(t, int64(3), s.Tokens("k", rule))
	require.Equal(t, int64(0), s.Usage("k").Count(KindMessages))

	// Refunding an unknown key or kind is harmless
	s.Refund("unknown", KindBandwidth, 1)
	require.Equal(t, 1, s.Size())
}

func TestService_Tokens_DoesNotConsume(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindAuth,
		Burst:  5,
		Refill: time.Minute,
	}
	require.Equal(t, int64(5), s.Tokens("k", rule))
	require.Equal(t, int64(5), s.Tokens("k", rule))
	require.True(t, s.Allow("k", rule, 2).OK)
	require.Equal(t, int64(3), s.Tokens("k", rule))
	require.Equal(t, int64(0), s.Tokens("k", Rule{Kind: KindAuth})) // No bucket, no tokens
}

func TestService_Check(t *testing.T) {
	// Check answers like Allow would for one unit, including the wait, but uses nothing
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindAuth,
		Burst:  2,
		Refill: time.Minute,
	}
	require.Equal(t, Decision{OK: true, Kind: KindAuth}, s.Check("k", rule)) // A fresh bucket is full
	require.True(t, s.Allow("k", rule, 2).OK)
	require.Equal(t, Decision{RetryAfter: time.Minute, Kind: KindAuth}, s.Check("k", rule))
	clock.advance(20 * time.Second)
	require.Equal(t, Decision{RetryAfter: 40 * time.Second, Kind: KindAuth}, s.Check("k", rule))
	require.Equal(t, int64(0), s.Tokens("k", rule)) // Checking did not take anything
	clock.advance(40 * time.Second)
	require.Equal(t, Decision{OK: true, Kind: KindAuth}, s.Check("k", rule))
	require.Equal(t, int64(1), s.Tokens("k", rule))
	require.Equal(t, Decision{Kind: KindAuth}, s.Check("k", Rule{Kind: KindAuth})) // No bucket admits nothing, as Tokens says 0
}

func TestService_Prune(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  1,
		Refill: time.Minute,
	}
	require.True(t, s.Allow("old", rule, 1).OK)
	clock.advance(10 * time.Minute)
	require.True(t, s.Allow("fresh", rule, 1).OK)
	require.Equal(t, 2, s.Size())
	s.Prune()
	require.Equal(t, 1, s.Size())
	require.True(t, s.Allow("old", rule, 1).OK)    // Forgotten: back with a full bucket
	require.False(t, s.Allow("fresh", rule, 1).OK) // Kept
}

func TestService_Prune_KeepsKeyUntilBucketsRefill(t *testing.T) {
	// A forgotten key comes back with full buckets, so an idle key is only forgotten once its
	// buckets are full again; otherwise a slow bucket (e.g. one email every few hours) would
	// hand out a fresh burst after every prune
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindEmails,
		Burst:  1,
		Refill: time.Hour,
	}
	require.True(t, s.Allow("key", rule, 1).OK)
	clock.advance(10 * time.Minute)
	s.Prune()
	require.Equal(t, 1, s.Size())
	require.False(t, s.Allow("key", rule, 1).OK)
	clock.advance(time.Hour)
	s.Prune()
	require.Equal(t, 0, s.Size())
	require.True(t, s.Allow("key", rule, 1).OK)
}

func TestService_Prune_DropsEndedPeriods(t *testing.T) {
	// Daily counts are kept until their period ends, however idle the key is
	s, clock := newTestServiceWithClock(t) // Noon UTC; "k" resets at 03:48:26 UTC
	rule := Rule{
		Kind:  KindMessages,
		Daily: 10,
	}
	require.True(t, s.Allow("k", rule, 4).OK)
	clock.advance(15 * time.Hour)
	s.Prune()
	require.Equal(t, 0, s.Size())
	require.Equal(t, int64(4), s.Usage("k").Count(KindMessages))
	require.Len(t, s.counters[shardIndex("k")].counters, 1)
	clock.advance(time.Hour)
	s.Prune()
	require.Empty(t, s.counters[shardIndex("k")].counters)
}

func TestService_Prune_ForgetsSettledKeyAfterAMinute(t *testing.T) {
	// A one-shot key (e.g. one of a flood of IPs) is forgotten once its buckets are full again and
	// it went unused for settledIdle, not after a day
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  10,
		Refill: time.Second,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	clock.advance(settledIdle / 2)
	require.Zero(t, s.Prune())
	require.Equal(t, 1, s.Size()) // Full again, but used too recently
	clock.advance(settledIdle)
	require.Equal(t, 1, s.Prune())
	require.Equal(t, 0, s.Size())
	require.Equal(t, rule.Burst, s.Tokens("k", rule))
}

func TestService_Prune_NoRefillDoesNotHoldKey(t *testing.T) {
	// A bucket without refill is never full again, so it does not hold the key: like any settled
	// key, it is forgotten after settledIdle and comes back with a full bucket
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:  KindAccount,
		Burst: 1,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	clock.advance(2 * settledIdle)
	require.Equal(t, 1, s.Prune())
	require.True(t, s.Allow("k", rule, 1).OK)
}

func TestService_Prune_ForgettingIsARefillAtPruneTime(t *testing.T) {
	// Forgetting a settled key is the same as refilling its buckets when it is forgotten: a
	// wall clock stepped back afterwards finds the same tokens either way
	clock := &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	pruned, kept := newService(&Config{}, clock.now), newService(&Config{}, clock.now)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  10,
		Refill: time.Second,
	}
	for _, s := range []*Service{pruned, kept} {
		require.True(t, s.Allow("k", rule, 10).OK)
	}
	clock.advance(time.Hour)
	require.Equal(t, 1, pruned.Prune())
	require.Equal(t, rule.Burst, kept.Tokens("k", rule))
	clock.advance(-59 * time.Minute) // 5 s after the bucket was emptied
	for _, s := range []*Service{pruned, kept} {
		require.Equal(t, rule.Burst, s.Tokens("k", rule))
		require.True(t, s.Allow("k", rule, 10).OK)
		require.False(t, s.Allow("k", rule, 1).OK)
	}
}

func TestService_Prune_DeniedSlowerRuleHoldsKey(t *testing.T) {
	// A bucket charged with a fast rule and then denied with a slower one (a tier downgrade, a
	// config change) is full again only at the slower rate: the key must be kept until then, or
	// it comes back with a full bucket
	s, clock := newTestServiceWithClock(t)
	fast := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Second,
	}
	slow := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Hour,
	}
	require.True(t, s.Allow("k", fast, 2).OK)
	clock.advance(time.Second)
	require.False(t, s.Allow("k", slow, 2).OK)
	clock.advance(2 * settledIdle)
	require.Zero(t, s.Prune())
	require.Equal(t, 1, s.Size())
	require.False(t, s.Allow("k", slow, 1).OK)
}

func TestService_Prune_TokensWithSlowerRuleHoldsKey(t *testing.T) {
	// Tokens refills too, so a slower rule read there moves the time the bucket is full again
	s, clock := newTestServiceWithClock(t)
	fast := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Second,
	}
	slow := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Hour,
	}
	require.True(t, s.Allow("k", fast, 2).OK)
	clock.advance(time.Second)
	require.Zero(t, s.Tokens("k", slow))
	clock.advance(2 * settledIdle)
	require.Zero(t, s.Prune())
	require.Zero(t, s.Tokens("k", slow))
}

func TestService_Prune_CheckWithSlowerRuleHoldsKey(t *testing.T) {
	// Check refills too, so a slower rule read there moves the time the bucket is full again
	s, clock := newTestServiceWithClock(t)
	fast := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Second,
	}
	slow := Rule{
		Kind:   KindRequests,
		Burst:  2,
		Refill: time.Hour,
	}
	require.True(t, s.Allow("k", fast, 2).OK)
	clock.advance(time.Second)
	require.False(t, s.Check("k", slow).OK)
	clock.advance(2 * settledIdle)
	require.Zero(t, s.Prune())
	require.Zero(t, s.Tokens("k", slow))
}

func TestService_Prune_ForgottenKeyDecidesLikeAFreshOne(t *testing.T) {
	// A key that is forgotten and used again gets the decision of a never seen key: a full bucket
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  3,
		Refill: time.Second,
	}
	require.True(t, s.Allow("k", rule, 3).OK)
	clock.advance(2 * settledIdle)
	require.Equal(t, 1, s.Prune())
	require.Equal(t, s.Allow("fresh", rule, 1), s.Allow("k", rule, 1))
	d := s.Allow("k", rule, 1)
	require.True(t, d.OK)
	require.Equal(t, int64(3), d.Quota)
	require.Equal(t, 3*time.Second, d.Window)
	require.Equal(t, int64(1), d.Remaining)
	require.Equal(t, s.Check("fresh", rule), s.Check("k", rule))
}

func TestService_Prune_BandwidthTallyHoldsKeyUntilPeriodEnd(t *testing.T) {
	// A big bandwidth bucket refills a small download in milliseconds, but the bytes stay in the
	// key's usage until its period ends, so the key is kept until then
	s, clock := newTestServiceWithClock(t)
	burst := int64(15 * 1024 * 1024 * 1024)
	rule := Rule{
		Kind:   KindBandwidth,
		Burst:  burst,
		Refill: oneDay / time.Duration(burst),
	}
	require.True(t, s.Allow("k", rule, 1024*1024).OK)
	clock.advance(2 * settledIdle)
	require.Zero(t, s.Prune())
	require.Equal(t, int64(1024*1024), s.Usage("k").Count(KindBandwidth))
	require.Equal(t, int64(1024*1024), s.Limiter("k", rule).Value())
	clock.t = s.ResetAt("k")
	require.Equal(t, 1, s.Prune())
	require.Equal(t, Usage{}, s.Usage("k"))
}

func TestService_Prune_LimiterViewAcrossPrune(t *testing.T) {
	// A limiter view (e.g. a reader metering an upload) refers to the key, not to its entry: bytes
	// it charged keep the entry, a revert finds them, and after a prune it charges a fresh entry
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Millisecond,
	}
	l := s.Limiter("k", rule)
	require.True(t, l.AllowN(100))
	clock.advance(2 * settledIdle)
	require.Zero(t, s.Prune())
	require.True(t, l.AllowN(-100)) // Revert: the upload failed
	require.Zero(t, l.Value())
	require.Equal(t, 1, s.Prune())
	require.True(t, l.AllowN(50))
	require.Equal(t, int64(50), l.Value())
	require.Equal(t, rule.Burst-50, s.Tokens("k", rule))
}

func TestService_Allow_Concurrent(t *testing.T) {
	s := New(&Config{})
	rule := Rule{
		Kind:   KindMessages,
		Burst:  500,
		Refill: time.Hour,
		Daily:  400,
	}
	var wg sync.WaitGroup
	var admitted sync.Map
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			n := 0
			for i := 0; i < 100; i++ {
				if s.Allow("hot", rule, 1).OK {
					n++
				}
				s.Allow(fmt.Sprintf("cold-%d-%d", g, i), rule, 1)
			}
			admitted.Store(g, n)
		}(g)
	}
	wg.Wait()
	total := 0
	admitted.Range(func(_, v any) bool {
		total += v.(int)
		return true
	})
	require.Equal(t, 400, total) // The daily cap, exactly once
	require.Equal(t, int64(400), s.Usage("hot").Count(KindMessages))
	require.Equal(t, 1601, s.Size())
}

func BenchmarkAllow_HotKey(b *testing.B) {
	s := New(&Config{})
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
		Daily:  1 << 40,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Allow("ip:1.2.3.4", rule, 1)
	}
}

func BenchmarkAllow_ManyKeys(b *testing.B) {
	s := New(&Config{})
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	keys := make([]string, 100000)
	for i := range keys {
		keys[i] = fmt.Sprintf("ip:10.%d.%d.%d", i>>16, (i>>8)&255, i&255)
		s.Allow(keys[i], rule, 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Allow(keys[i%len(keys)], rule, 1)
	}
}

func BenchmarkAllow_Parallel(b *testing.B) {
	s := New(&Config{})
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Allow(fmt.Sprintf("ip:10.0.%d.%d", (i>>8)&255, i&255), rule, 1)
			i++
		}
	})
}

func TestService_TouchAndSeen(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	require.True(t, s.Seen("k").IsZero())
	s.Touch("k")
	require.Equal(t, clock.now(), s.Seen("k"))
	clock.advance(10 * time.Minute)
	s.Touch("k")
	s.Prune()
	require.Equal(t, 1, s.Size()) // Touched, so not idle
	require.Equal(t, clock.now(), s.Seen("k"))
	clock.advance(10 * time.Minute)
	s.Prune()
	require.True(t, s.Seen("k").IsZero())
}

func TestService_FirebaseKind(t *testing.T) {
	kind, ok := ParseKind("firebase")
	require.True(t, ok)
	require.Equal(t, KindFirebase, kind)
}

// Fails if Allow stops debiting bucket tokens; weighted concurrent calls must share one burst.
func TestService_Allow_ConcurrentBucketOnly(t *testing.T) {
	s := newTestService(t)
	rule := Rule{
		Kind:  KindBandwidth,
		Burst: 97,
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int64
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i := 0; i < 64; i++ {
				n := int64(1 + g%7)
				if s.Allow("hot", rule, n).OK {
					admitted.Add(n)
				}
			}
		}(g)
	}
	close(start)
	wg.Wait()
	require.Equal(t, rule.Burst, admitted.Load())
	require.Equal(t, admitted.Load(), s.Usage("hot").Count(rule.Kind))
	require.Zero(t, s.Tokens("hot", rule))
}

// Fails if Allow no longer refreshes seen; pruning must retain an active exhausted key while deleting idle keys.
func TestService_Prune_ConcurrentAllow(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:  KindRequests,
		Burst: 1,
	}
	require.True(t, s.Allow("hot", rule, 1).OK)
	for i := 0; i < 128; i++ {
		require.True(t, s.Allow(fmt.Sprintf("idle-%d", i), rule, 1).OK)
	}
	clock.advance(2 * time.Hour)
	require.False(t, s.Allow("hot", rule, 1).OK)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int64
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				if s.Allow("hot", rule, 1).OK {
					admitted.Add(1)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 100; i++ {
			s.Prune()
		}
	}()
	close(start)
	wg.Wait()
	require.Zero(t, admitted.Load())
	require.Zero(t, s.Tokens("hot", rule))
	require.Equal(t, 1, s.Size())
}

// Fails if refill is disabled; nanosecond and century intervals must earn exactly one token per interval.
func TestService_Allow_ExtremeRefill(t *testing.T) {
	for _, interval := range []time.Duration{time.Nanosecond, 100 * 365 * 24 * time.Hour} {
		t.Run(interval.String(), func(t *testing.T) {
			s, clock := newTestServiceWithClock(t)
			rule := Rule{
				Kind:   KindRequests,
				Burst:  2,
				Refill: interval,
			}
			require.True(t, s.Allow("hot", rule, 2).OK)
			d := s.Allow("hot", rule, 1)
			require.False(t, d.OK)
			require.Equal(t, interval, d.RetryAfter)
			clock.advance(interval)
			require.True(t, s.Allow("hot", rule, 1).OK)
			require.False(t, s.Allow("hot", rule, 1).OK)
		})
	}
}

func TestService_Allow_ClockGoingBackDoesNotCharge(t *testing.T) {
	// A call that carries an older timestamp than the bucket's last refill (a goroutine paused
	// between reading the clock and taking the shard lock, or a wall clock set back) must not
	// take tokens away, and must not move the refill point back either (that would credit the
	// same time twice)
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  3,
		Refill: time.Second,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	clock.advance(-time.Second)
	require.True(t, s.Allow("k", rule, 1).OK) // The third token is still there
	require.False(t, s.Allow("k", rule, 1).OK)
	clock.advance(time.Second) // Back to where the bucket was last refilled: no time has passed
	require.False(t, s.Allow("k", rule, 1).OK)
	clock.advance(time.Second)
	require.True(t, s.Allow("k", rule, 1).OK)
}

func TestService_PeriodRollover(t *testing.T) {
	// Each key's daily counts start over at the end of its own 24h period, lazily on the next
	// use, while its buckets keep their tokens: a rollover never hands back a bucket
	s, clock := newTestServiceWithClock(t) // Noon UTC; "k" resets at 03:48:26 UTC, "ip:1.2.3.4" at 17:14:44 UTC
	rule := Rule{
		Kind:   KindEmails,
		Burst:  3,
		Refill: time.Hour,
		Daily:  3,
	}
	require.True(t, s.Allow("k", rule, 3).OK)
	require.True(t, s.Allow("ip:1.2.3.4", rule, 3).OK)
	clock.advance(5*time.Hour + 14*time.Minute + 44*time.Second) // 17:14:44 UTC, both buckets full again
	require.Equal(t, int64(3), s.Usage("k").Count(KindEmails))
	require.Equal(t, int64(0), s.Usage("ip:1.2.3.4").Count(KindEmails))
	require.True(t, s.Allow("ip:1.2.3.4", rule, 3).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 10*time.Hour+33*time.Minute+42*time.Second, d.RetryAfter) // Until 03:48:26 UTC
	clock.advance(d.RetryAfter)
	require.Equal(t, int64(0), s.Usage("k").Count(KindEmails))
	require.Equal(t, int64(3), s.Usage("ip:1.2.3.4").Count(KindEmails))

	// The bucket does not refill at the rollover
	require.True(t, s.Allow("k", rule, 3).OK)
	d = s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, time.Hour, d.RetryAfter)
	require.Equal(t, int64(3), s.Usage("k").Count(KindEmails))
}

func TestService_ResetAt(t *testing.T) {
	s, clock := newTestServiceWithClock(t) // Noon UTC
	require.Equal(t, time.Date(2026, 10, 26, 3, 48, 26, 0, time.UTC), s.ResetAt("k"))
	require.Equal(t, time.Date(2026, 10, 25, 17, 14, 44, 0, time.UTC), s.ResetAt("ip:1.2.3.4"))
	clock.advance(6 * time.Hour) // 18:00 UTC
	require.Equal(t, time.Date(2026, 10, 26, 17, 14, 44, 0, time.UTC), s.ResetAt("ip:1.2.3.4"))

	// Retry-After of a daily cap points at the same time
	rule := Rule{
		Kind:  KindMessages,
		Daily: 1,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, s.ResetAt("k").Sub(clock.now()), d.RetryAfter)
}

// startTestService starts a service on the store with the given clock, whose worker never ticks
// on its own: the tests sync by hand. It is closed when the test ends.
func startTestService(t *testing.T, store *Store, now func() time.Time) *Service {
	t.Helper()
	s := newService(&Config{Store: store}, now)
	s.interval = time.Hour
	require.Nil(t, s.Start(context.Background()))
	t.Cleanup(s.Close)
	return s
}

// usageRowOf returns the key's row in the store, or a zero row
func usageRowOf(t *testing.T, store *Store, key string) usageRow {
	t.Helper()
	return readAll(t, store, 0)[key]
}

func TestService_Persist_SurvivesRestart(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		promoted := Rule{
			Kind:  KindMessages,
			Daily: 10,
		}
		belowThreshold := Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 25,
		}
		s := startTestService(t, newStore(), time.Now)
		for i := 0; i < 7; i++ {
			require.True(t, s.Allow("user:u_1", promoted, 1).OK)
		}
		require.True(t, s.Allow("ip:1.2.3.4", belowThreshold, 24).OK)
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindCalls}, 1).OK)
		s.Close() // Writes what is unflushed

		// A restarted node continues where the old one stopped; a key below the threshold starts over
		s = startTestService(t, newStore(), time.Now)
		require.Equal(t, int64(7), s.Usage("user:u_1").Count(KindMessages))
		require.Equal(t, int64(1), s.Usage("user:u_1").Count(KindCalls))
		require.Equal(t, int64(0), s.Usage("ip:1.2.3.4").Count(KindMessages))
		for i := 0; i < 3; i++ {
			require.True(t, s.Allow("user:u_1", promoted, 1).OK)
		}
		require.False(t, s.Allow("user:u_1", promoted, 1).OK)
	})
}

func TestService_Persist_NextPeriodStartsAtZero(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		dbNow := time.Now().Unix()
		_, err := store.write(context.Background(), []usageRow{
			{
				key:         "user:u_1",
				periodStart: periodStart("user:u_1", time.Unix(dbNow, 0)) - daySeconds,
				counts:      [numDaily]int64{100, 5, 1},
			},
			{
				key:         "user:u_2",
				periodStart: periodStart("user:u_2", time.Unix(dbNow, 0)),
				counts:      [numDaily]int64{4, 0, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		s := startTestService(t, newStore(), time.Now)
		require.Equal(t, Usage{}, s.Usage("user:u_1")) // Stale: the previous period
		require.Equal(t, int64(4), s.Usage("user:u_2").Count(KindMessages))
	})
}

func TestService_Persist_UpdatedAtIsTheNodesTime(t *testing.T) {
	// Rows are stamped with the writing node's clock, so the read-back watermark (also the node's
	// clock) never depends on the database's clock
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		ahead := time.Now().Add(time.Hour)
		s := startTestService(t, newStore(), func() time.Time {
			return ahead
		})
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages}, 1).OK)
		s.sync()
		var updatedAt int64
		require.Nil(t, s.conf.Store.db.QueryRow(`SELECT updated_at FROM usage WHERE key = 'user:u_1'`).Scan(&updatedAt))
		require.Equal(t, ahead.Unix(), updatedAt)
	})
}

func TestService_Persist_TwoNodesConverge(t *testing.T) {
	// Two nodes on one store, one with its clock 2 s ahead (synced within the read overlap): both
	// agree on the key's period, and after a sync on its count
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), func() time.Time {
			return time.Now().Add(2 * time.Second)
		})
		rule := Rule{
			Kind:  KindMessages,
			Daily: 6,
		}
		require.True(t, a.Allow("k", rule, 3).OK)
		require.True(t, b.Allow("k", rule, 2).OK)
		a.sync()
		b.sync()
		a.sync()
		require.Equal(t, int64(5), a.Usage("k").Count(KindMessages))
		require.Equal(t, int64(5), b.Usage("k").Count(KindMessages))
		require.Equal(t, a.ResetAt("k"), b.ResetAt("k"))
		require.True(t, a.Allow("k", rule, 1).OK)
		a.sync()
		b.sync()
		require.False(t, b.Allow("k", rule, 1).OK)
		require.Equal(t, int64(6), b.Usage("k").Count(KindMessages))
		require.Equal(t, int64(6), usageRowOf(t, a.conf.Store, "k").counts[0])
	})
}

func TestService_Persist_ThresholdPromotes(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := startTestService(t, newStore(), time.Now)
		rule := Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 25,
		}
		require.True(t, s.Allow("ip:1.2.3.4", rule, 24).OK)
		s.sync()
		require.Equal(t, usageRow{}, usageRowOf(t, s.conf.Store, "ip:1.2.3.4"))
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
		s.sync()
		require.Equal(t, int64(25), usageRowOf(t, s.conf.Store, "ip:1.2.3.4").counts[0])
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
		s.sync()
		require.Equal(t, int64(26), usageRowOf(t, s.conf.Store, "ip:1.2.3.4").counts[0])
	})
}

func TestService_Persist_ReadBackPromotes(t *testing.T) {
	// A node that reads another node's row for a key it counted below the threshold promotes it
	// and writes its own counts
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		rule := Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 25,
		}
		require.True(t, a.Allow("ip:1.2.3.4", rule, 10).OK)
		require.True(t, b.Allow("ip:1.2.3.4", rule, 30).OK)
		b.sync()
		a.sync() // Reads b's row
		require.Equal(t, int64(40), a.Usage("ip:1.2.3.4").Count(KindMessages))
		a.sync() // Writes its own 10
		require.Equal(t, int64(40), usageRowOf(t, a.conf.Store, "ip:1.2.3.4").counts[0])
		b.sync()
		require.Equal(t, int64(40), b.Usage("ip:1.2.3.4").Count(KindMessages))
	})
}

func TestService_Persist_ReadBackKeyIsNotUsedHere(t *testing.T) {
	// A key known only from another node's row has its count here, but no buckets in use: it is
	// not a visitor of this node (Size, Seen), and it is forgotten once its period ends
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		require.True(t, a.Allow("user:u_1", Rule{Kind: KindMessages}, 3).OK)
		a.sync()
		b.sync()
		require.Equal(t, int64(3), b.Usage("user:u_1").Count(KindMessages))
		require.Equal(t, 0, b.Size())
		require.Equal(t, time.Time{}, b.Seen("user:u_1"))
		require.Equal(t, benchRequestRule.Burst, b.Tokens("user:u_1", benchRequestRule))
		require.True(t, b.Allow("user:u_1", benchRequestRule, 1).OK)
		require.Equal(t, 1, b.Size())

		// Once the period ended and the buckets are idle and full, the key is gone
		clock.advance(25 * time.Hour)
		b.Prune()
		require.Equal(t, 0, b.Size())
		require.Equal(t, Usage{}, b.Usage("user:u_1"))
	})
}

func TestService_Persist_WriteFailureRetries(t *testing.T) {
	// A failed write keeps the counts unflushed for the next round: nothing lost, nothing twice
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		s := startTestService(t, store, time.Now)
		rule := Rule{
			Kind:  KindEmails,
			Daily: 100,
		}
		require.True(t, s.Allow("k", rule, 30).OK)
		writeChunk := store.writeChunk
		store.writeChunk = func(ctx context.Context, rows []usageRow, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		require.Equal(t, int64(30), s.Usage("k").Count(KindEmails))
		require.Equal(t, usageRow{}, usageRowOf(t, store, "k"))
		store.writeChunk = writeChunk
		require.True(t, s.Allow("k", rule, 1).OK)
		s.sync()
		s.sync()
		require.Equal(t, int64(31), s.Usage("k").Count(KindEmails))
		require.Equal(t, int64(31), usageRowOf(t, store, "k").counts[1])
	})
}

func TestService_Persist_TimeoutAfterCommitIsNotCountedTwice(t *testing.T) {
	// On Postgres, a write that times out may have committed (e.g. the commit was acknowledged
	// after the deadline): it is counted as written, so a retry does not count it twice. If it
	// did not commit, its counts are lost, which fails open like a store outage.
	d := dbtest.CreateTestPostgres(t)
	_, err := user.NewPostgresManager(d, &user.Config{}) // Creates the usage table
	require.Nil(t, err)
	store := NewPostgresStore(d)
	writeChunk := store.writeChunk
	store.writeChunk = func(ctx context.Context, rows []usageRow, now int64) error {
		if err := writeChunk(ctx, rows, now); err != nil {
			return err
		}
		return fmt.Errorf("%w: %w", errMaybeWritten, context.DeadlineExceeded) // Committed, but the answer came too late
	}
	s := startTestService(t, store, time.Now)
	require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages}, 26).OK)
	s.sync()
	store.writeChunk = writeChunk
	s.sync()
	require.Equal(t, int64(26), usageRowOf(t, store, "user:u_1").counts[0])
	require.Equal(t, int64(26), s.Usage("user:u_1").Count(KindMessages))
}

func TestService_Persist_StartupWithoutStore(t *testing.T) {
	// A node that cannot load at startup serves from memory, and adds the store's counts once it
	// can load them
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		dbNow := time.Now().Unix()
		_, err := store.write(context.Background(), []usageRow{
			{
				key:         "k",
				periodStart: periodStart("k", time.Unix(dbNow, 0)),
				counts:      [numDaily]int64{3, 0, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		_, err = store.db.Exec(`ALTER TABLE usage RENAME TO usage_down`)
		require.Nil(t, err)
		s := newService(&Config{Store: store}, time.Now)
		s.interval = 50 * time.Millisecond
		t.Cleanup(s.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		started := time.Now()
		require.Nil(t, s.Start(ctx))
		require.Less(t, time.Since(started), 5*time.Second)
		rule := Rule{
			Kind:  KindMessages,
			Daily: 10,
		}
		require.True(t, s.Allow("k", rule, 2).OK)
		require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))
		_, err = store.db.Exec(`ALTER TABLE usage_down RENAME TO usage`)
		require.Nil(t, err)
		require.Eventually(t, func() bool {
			return s.Usage("k").Count(KindMessages) == 5
		}, 5*time.Second, 20*time.Millisecond)
		require.Eventually(t, func() bool {
			return usageRowOf(t, store, "k").counts[0] == 5
		}, 5*time.Second, 20*time.Millisecond)
	})
}

func TestService_Persist_LeftoverAfterRollover(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		s := startTestService(t, store, time.Now)
		dbNow := time.Now().Unix()
		rule := Rule{
			Kind: KindMessages,
		}

		// A new period's counts replace the row of an old one, they do not add to it
		start := periodStart("a", time.Unix(dbNow, 0))
		_, err := store.write(context.Background(), []usageRow{
			{
				key:         "a",
				periodStart: start - daySeconds,
				counts:      [numDaily]int64{100, 0, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		require.True(t, s.Allow("a", rule, 2).OK)
		s.sync()
		require.Equal(t, usageRow{
			key:         "a",
			periodStart: start,
			counts:      [numDaily]int64{2, 0, 0},
		}, usageRowOf(t, store, "a"))
		require.Equal(t, int64(2), s.Usage("a").Count(KindMessages))

		// Another node rolled first: a leftover of the old period lands in the new one, and the
		// row keeps the new period
		start = periodStart("b", time.Unix(dbNow, 0))
		require.True(t, s.Allow("b", rule, 3).OK)
		_, err = store.write(context.Background(), []usageRow{
			{
				key:         "b",
				periodStart: start + daySeconds,
				counts:      [numDaily]int64{4, 0, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		s.sync()
		require.Equal(t, usageRow{
			key:         "b",
			periodStart: start + daySeconds,
			counts:      [numDaily]int64{7, 0, 0},
		}, usageRowOf(t, store, "b"))
	})
}

func TestService_Allow_DenyRacesRollover(t *testing.T) {
	// A daily denial computes its retry time from the counter it decided on, while other calls
	// roll the counter over; run with -race
	var ticks atomic.Int64
	start := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	s := newService(&Config{}, func() time.Time {
		return start.Add(time.Duration(ticks.Add(1)) * time.Hour)
	})
	rule := Rule{
		Kind:  KindMessages,
		Daily: 1,
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3000; i++ {
				if d := s.Allow("k", rule, 1); !d.OK {
					require.Greater(t, d.RetryAfter, time.Duration(0))
				}
			}
		}()
	}
	wg.Wait()
}

func TestService_Close_CancelsStalledLoad(t *testing.T) {
	// Close does not wait for a startup load stuck on the database
	store := NewSQLiteStore(newTestUserDB(t))
	pool := store.db.Primary()
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(context.Background()) // Holds the only connection: the load waits for it
	require.Nil(t, err)
	defer conn.Close()
	s := newService(&Config{Store: store}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Nil(t, s.Start(ctx))
	require.Eventually(t, func() bool {
		return pool.Stats().WaitCount > 0
	}, time.Second, time.Millisecond)
	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waits for the stalled load")
	}
}

// Benchmarks of a node at production scale: 400k keys that only made requests (subscribers,
// polls) and 50k that published, 5k of them past the threshold. One op of BenchmarkService_Publish
// is what a publish costs: the request bucket and the daily message count.
const (
	benchIdleKeys      = 400_000
	benchPublisherKeys = 50_000
	benchPromotedKeys  = 5_000
	benchFloodKeys     = 500_000
)

var (
	benchRequestRule = Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	benchMessageRule = Rule{
		Kind:      KindMessages,
		Daily:     1_000_000,
		Threshold: 25,
	}
)

func newBenchService(b *testing.B) *Service {
	b.Helper()
	s := newService(&Config{}, time.Now)
	for i := 0; i < benchIdleKeys; i++ {
		s.Allow(fmt.Sprintf("ip:10.%d.%d.%d", i>>16, (i>>8)&255, i&255), benchRequestRule, 1)
	}
	for i := 0; i < benchPublisherKeys; i++ {
		key := fmt.Sprintf("ip:172.%d.%d.%d", i>>16, (i>>8)&255, i&255)
		s.Allow(key, benchRequestRule, 1)
		n := int64(1)
		if i < benchPromotedKeys {
			n = 30
		}
		s.Allow(key, benchMessageRule, n)
	}
	return s
}

func BenchmarkService_Publish(b *testing.B) {
	s := newBenchService(b)
	keys := make([]string, benchPublisherKeys)
	for i := range keys {
		keys[i] = fmt.Sprintf("ip:172.%d.%d.%d", i>>16, (i>>8)&255, i&255)
	}
	var next atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := keys[next.Add(1)%int64(len(keys))]
			s.Allow(key, benchRequestRule, 1)
			s.Allow(key, benchMessageRule, 1)
		}
	})
}

func BenchmarkService_SyncScan(b *testing.B) {
	s := newBenchService(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rows := s.unflushedRows(); len(rows) != benchPromotedKeys {
			b.Fatalf("expected %d rows, got %d", benchPromotedKeys, len(rows))
		}
	}
}

func BenchmarkService_Memory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		s := newBenchService(b)
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(benchIdleKeys+benchPublisherKeys), "B/key")
		runtime.KeepAlive(s)
	}
}

func BenchmarkService_MemoryFlood(b *testing.B) {
	// A flood of one-shot keys (one request each, as from new IPs): what the buckets hold right
	// after it, and after a prune a little later
	for i := 0; i < b.N; i++ {
		var base, flooded, pruned runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&base)
		clock := &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
		s := newService(&Config{}, clock.now)
		for j := 0; j < benchFloodKeys; j++ {
			key := fmt.Sprintf("ip:10.%d.%d.%d", j>>16, (j>>8)&255, j&255)
			s.Touch(key)
			s.Allow(key, benchRequestRule, 1)
		}
		runtime.GC()
		runtime.ReadMemStats(&flooded)
		keys := s.Size()
		clock.advance(2 * settledIdle)
		s.Prune()
		runtime.GC()
		runtime.ReadMemStats(&pruned)
		b.ReportMetric(float64(keys), "keys")
		b.ReportMetric(float64(flooded.HeapAlloc-base.HeapAlloc)/float64(benchFloodKeys), "B/key")
		b.ReportMetric(float64(flooded.HeapAlloc-base.HeapAlloc)/1e6, "MB-flooded")
		b.ReportMetric(float64(s.Size()), "keys-pruned")
		b.ReportMetric(float64(int64(pruned.HeapAlloc)-int64(base.HeapAlloc))/1e6, "MB-pruned")
		runtime.KeepAlive(s)
	}
}
