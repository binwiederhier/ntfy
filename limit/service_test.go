package limit

import (
	"fmt"
	"sync"
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
	conf := &Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}
	return newService(conf, clock.now), clock
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, _ := newTestServiceWithClock(t)
	return s
}

func TestService_Allow_Bucket(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{Kind: KindRequests, Burst: 3, Refill: 100 * time.Millisecond}
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
	s, _ := newTestServiceWithClock(t) // Noon UTC, 12 hours before the reset
	rule := Rule{Kind: KindMessages, Daily: 2}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 12*time.Hour, d.RetryAfter)
	require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))

	// Denied usage is not counted
	require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))

	// The reset starts the day over
	s.ResetUsage()
	require.Equal(t, int64(0), s.Usage("k").Count(KindMessages))
	require.True(t, s.Allow("k", rule, 1).OK)
}

func TestService_Allow_BucketAndDaily(t *testing.T) {
	// Emails: a burst of 2 refilling slowly, and a daily cap of 3
	s, clock := newTestServiceWithClock(t)
	rule := Rule{Kind: KindEmails, Burst: 2, Refill: time.Minute, Daily: 3}
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
	require.Equal(t, 11*time.Hour-time.Minute, d.RetryAfter) // The daily cap is what denies now
	require.Equal(t, int64(3), s.Usage("k").Count(KindEmails))
}

func TestService_Allow_N(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{Kind: KindBandwidth, Burst: 1000, Refill: time.Millisecond} // 1 kB burst, 1 kB/s
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

func TestService_Allow_RuleChangeKeepsTokens(t *testing.T) {
	// A tier change passes a new rule on the next call: a bigger burst does not hand back a full
	// bucket, a smaller one caps what is there
	s, _ := newTestServiceWithClock(t)
	rule := Rule{Kind: KindRequests, Burst: 10, Refill: time.Hour}
	for i := 0; i < 8; i++ {
		require.True(t, s.Allow("k", rule, 1).OK)
	}
	bigger := Rule{Kind: KindRequests, Burst: 100, Refill: time.Hour}
	require.Equal(t, int64(2), s.Tokens("k", bigger))
	require.True(t, s.Allow("k", bigger, 1).OK)
	require.True(t, s.Allow("k", bigger, 1).OK)
	require.False(t, s.Allow("k", bigger, 1).OK)
	smaller := Rule{Kind: KindRequests, Burst: 1, Refill: time.Hour}
	require.Equal(t, int64(0), s.Tokens("k", smaller))

	// A fresh key with the smaller rule starts with its burst
	require.Equal(t, int64(1), s.Tokens("fresh", smaller))
	require.True(t, s.Allow("fresh", smaller, 1).OK)
	require.False(t, s.Allow("fresh", smaller, 1).OK)
	require.False(t, s.Allow("fresh", bigger, 1).OK) // Still empty, the bigger rule refills nothing
}

func TestService_Allow_DailyChangeAppliesAtOnce(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	require.True(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 5}, 5).OK)
	require.False(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 5}, 1).OK)
	require.True(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 6}, 1).OK)  // Upgraded
	require.False(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 3}, 1).OK) // Downgraded below the count
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
	rule := Rule{Kind: KindAccount, Burst: 2} // No refill
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	clock.advance(24 * time.Hour)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, time.Duration(0), d.RetryAfter)
}

func TestService_Refund(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	rule := Rule{Kind: KindBandwidth, Burst: 3, Refill: time.Hour, Daily: 100}
	require.True(t, s.Allow("k", rule, 3).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	s.Refund("k", KindBandwidth, 1)
	require.True(t, s.Allow("k", rule, 1).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	require.Equal(t, int64(3), s.Usage("k").Count(KindBandwidth))

	// A refund cannot exceed the burst, nor make the count negative
	s.Refund("k", KindBandwidth, 100)
	require.Equal(t, int64(3), s.Tokens("k", rule))
	require.Equal(t, int64(0), s.Usage("k").Count(KindBandwidth))

	// Refunding an unknown key or kind is harmless
	s.Refund("unknown", KindBandwidth, 1)
	require.Equal(t, 1, s.Size())
}

func TestService_Tokens_DoesNotConsume(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	rule := Rule{Kind: KindAuth, Burst: 5, Refill: time.Minute}
	require.Equal(t, int64(5), s.Tokens("k", rule))
	require.Equal(t, int64(5), s.Tokens("k", rule))
	require.True(t, s.Allow("k", rule, 2).OK)
	require.Equal(t, int64(3), s.Tokens("k", rule))
	require.Equal(t, int64(0), s.Tokens("k", Rule{Kind: KindAuth})) // No bucket, no tokens
}

func TestService_Seed(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	var usage Usage
	usage.Set(KindMessages, 120)
	usage.Set(KindEmails, 7)
	s.Seed("k", usage)
	require.Equal(t, int64(120), s.Usage("k").Count(KindMessages))
	require.Equal(t, int64(7), s.Usage("k").Count(KindEmails)) // Every kind at once, not only the first
	usage.Set(KindMessages, 999)
	s.Seed("k", usage) // Known key: ignored
	require.Equal(t, int64(120), s.Usage("k").Count(KindMessages))
	require.False(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 120}, 1).OK)
	require.True(t, s.Allow("k", Rule{Kind: KindMessages, Daily: 121}, 1).OK)
}

func TestService_Prune(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{Kind: KindRequests, Burst: 1, Refill: time.Hour}
	require.True(t, s.Allow("old", rule, 1).OK)
	clock.advance(10 * time.Minute)
	require.True(t, s.Allow("fresh", rule, 1).OK)
	require.Equal(t, 2, s.Size())
	s.Prune(5 * time.Minute)
	require.Equal(t, 1, s.Size())
	require.True(t, s.Allow("old", rule, 1).OK)    // Forgotten: back with a full bucket
	require.False(t, s.Allow("fresh", rule, 1).OK) // Kept
}

func TestService_Allow_Concurrent(t *testing.T) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)})
	rule := Rule{Kind: KindRequests, Burst: 500, Refill: time.Hour, Daily: 400}
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
	require.Equal(t, int64(400), s.Usage("hot").Count(KindRequests))
	require.Equal(t, 1601, s.Size())
}

func BenchmarkAllow_HotKey(b *testing.B) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)})
	rule := Rule{Kind: KindRequests, Burst: 60, Refill: 5 * time.Second, Daily: 1 << 40}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Allow("ip:1.2.3.4", rule, 1)
	}
}

func BenchmarkAllow_ManyKeys(b *testing.B) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)})
	rule := Rule{Kind: KindRequests, Burst: 60, Refill: 5 * time.Second}
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)})
	rule := Rule{Kind: KindRequests, Burst: 60, Refill: 5 * time.Second}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Allow(fmt.Sprintf("ip:10.0.%d.%d", (i>>8)&255, i&255), rule, 1)
			i++
		}
	})
}
