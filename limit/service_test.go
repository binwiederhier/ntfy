package limit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		require.True(t, s.Allow("k", Rule{Kind: KindTopics}, 1).OK)
	}
	require.Equal(t, int64(1000), s.Usage("k").Count(KindTopics))
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
	s.Prune(5 * time.Minute)
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
	s.Prune(5 * time.Minute)
	require.Equal(t, 1, s.Size())
	require.False(t, s.Allow("key", rule, 1).OK)
	clock.advance(time.Hour)
	s.Prune(5 * time.Minute)
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
	s.Prune(time.Minute)
	require.Equal(t, 0, s.Size())
	require.Equal(t, int64(4), s.Usage("k").Count(KindMessages))
	require.Len(t, s.counters[shardIndex("k")].counters, 1)
	clock.advance(time.Hour)
	s.Prune(time.Minute)
	require.Empty(t, s.counters[shardIndex("k")].counters)
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
	s.Prune(5 * time.Minute)
	require.Equal(t, 1, s.Size()) // Touched, so not idle
	require.Equal(t, clock.now(), s.Seen("k"))
	clock.advance(10 * time.Minute)
	s.Prune(5 * time.Minute)
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
			s.Prune(time.Hour)
		}
	}()
	close(start)
	wg.Wait()
	require.Zero(t, admitted.Load())
	require.Equal(t, int64(1), s.Usage("hot").Count(rule.Kind))
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
		dbNow, err := store.clock(context.Background())
		require.Nil(t, err)
		_, err = store.write(context.Background(), []usageRow{
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
		})
		require.Nil(t, err)
		s := startTestService(t, newStore(), time.Now)
		require.Equal(t, Usage{}, s.Usage("user:u_1")) // Stale: the previous period
		require.Equal(t, int64(4), s.Usage("user:u_2").Count(KindMessages))
	})
}

func TestService_Persist_TwoNodesConverge(t *testing.T) {
	// Two nodes on one store, one with its clock 10 minutes ahead: periods follow the database
	// clock, so both agree on the key's period, and after a sync on its count
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), func() time.Time {
			return time.Now().Add(10 * time.Minute)
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
		store.writeChunk = func(ctx context.Context, rows []usageRow) error {
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

func TestService_Persist_StartupWithoutStore(t *testing.T) {
	// A node that cannot load at startup serves from memory, and adds the store's counts once it
	// can load them
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		dbNow, err := store.clock(context.Background())
		require.Nil(t, err)
		_, err = store.write(context.Background(), []usageRow{
			{
				key:         "k",
				periodStart: periodStart("k", time.Unix(dbNow, 0)),
				counts:      [numDaily]int64{3, 0, 0},
			},
		})
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
		dbNow, err := store.clock(context.Background())
		require.Nil(t, err)
		rule := Rule{
			Kind: KindMessages,
		}

		// A new period's counts replace the row of an old one, they do not add to it
		start := periodStart("a", time.Unix(dbNow, 0))
		_, err = store.write(context.Background(), []usageRow{
			{
				key:         "a",
				periodStart: start - daySeconds,
				counts:      [numDaily]int64{100, 0, 0},
			},
		})
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
		})
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
