package limit

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
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
	conf := &Config{
		ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
		FlushInterval: time.Hour,
	}
	return newService(conf, clock.now, nil), clock
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
	s, _ := newTestServiceWithClock(t) // Noon UTC, 12 hours before the reset
	rule := Rule{
		Kind:  KindMessages,
		Daily: 2,
	}
	require.True(t, s.Allow("k", rule, 1).OK)
	require.True(t, s.Allow("k", rule, 1).OK)
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 12*time.Hour, d.RetryAfter)
	require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))

	// Denied usage is not counted
	require.Equal(t, int64(2), s.Usage("k").Count(KindMessages))

	// The next day starts over (see TestService_DayRollover)
	s.RollDayForTest("2099-01-01")
	require.Equal(t, int64(0), s.Usage("k").Count(KindMessages))
	require.True(t, s.Allow("k", rule, 1).OK)
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
	require.Equal(t, 11*time.Hour-time.Minute, d.RetryAfter) // The daily cap is what denies now
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
		Kind:   KindBandwidth,
		Burst:  3,
		Refill: time.Hour,
		Daily:  100,
	}
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

func TestService_Allow_Concurrent(t *testing.T) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
	rule := Rule{
		Kind:   KindRequests,
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
	require.Equal(t, int64(400), s.Usage("hot").Count(KindRequests))
	require.Equal(t, 1601, s.Size())
}

func BenchmarkAllow_HotKey(b *testing.B) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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

// The persistence tests below run on both backends (see forEachBackend in store_test.go). They
// drive the flush and the miss read-back by hand (the loops are off: a long interval), and
// treat two services on one store as two nodes.

func newPersistentTestService(t *testing.T, store *Store, clock *testClock) *Service {
	t.Helper()
	conf := &Config{
		ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
		FlushInterval: time.Hour,
	}
	s := newService(conf, clock.now, store)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestService_Persist_FlushAndReadBack(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		a := newPersistentTestService(t, newStore(), clock)
		b := newPersistentTestService(t, newStore(), clock)
		rule := Rule{
			Kind:  KindMessages,
			Daily: 100,
		}
		require.True(t, a.Allow("user:u_1", rule, 3).OK)
		require.Equal(t, int64(3), a.Usage("user:u_1").Count(KindMessages))
		a.flush()

		// B never saw the key: a lookup reads the row
		usage, err := b.Lookup("user:u_1")
		require.Nil(t, err)
		require.Equal(t, int64(3), usage.Count(KindMessages))
		require.True(t, b.Allow("user:u_1", rule, 2).OK)
		b.flush()

		// A learns B's usage at its next flush, and enforces the cluster total
		a.flush()
		require.Equal(t, int64(5), a.Usage("user:u_1").Count(KindMessages))
		require.False(t, a.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 5,
		}, 1).OK)
		require.True(t, a.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 6,
		}, 1).OK)
		a.flush()
		b.flush()
		require.Equal(t, int64(6), b.Usage("user:u_1").Count(KindMessages))
	})
}

func TestService_Persist_Threshold(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		store := newStore()
		s := newPersistentTestService(t, store, clock)
		rule := Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 3,
		} // 1% of the quota, rounded up
		require.True(t, s.Allow("ip:1.2.3.4", rule, 2).OK)
		s.flush()
		rows, err := store.readKeys(context.Background(), s.today(), []string{"ip:1.2.3.4"})
		require.Nil(t, err)
		require.Empty(t, rows) // Below the threshold: no row

		// The third message promotes the key; the row then carries everything, not just the delta since
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
		s.flush()
		rows, err = store.readKeys(context.Background(), s.today(), []string{"ip:1.2.3.4"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"ip:1.2.3.4/messages": 3}, rowsByKeyKind(rows))

		// Bucket kinds are never persisted, whatever the threshold
		require.True(t, s.Allow("ip:1.2.3.4", Rule{
			Kind:   KindRequests,
			Burst:  100,
			Refill: time.Second,
		}, 50).OK)
		s.flush()
		rows, err = store.readKeys(context.Background(), s.today(), []string{"ip:1.2.3.4"})
		require.Nil(t, err)
		require.Len(t, rows, 1)
	})
}

func TestService_Persist_FlushErrorRetainsDeltas(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		good := newStore()
		broken := newStore()
		closed, err := sql.Open("sqlite3", ":memory:")
		require.Nil(t, err)
		require.Nil(t, closed.Close())
		broken.db = db.New(&db.Host{DB: closed}, nil) // Every statement fails from here on
		s := newPersistentTestService(t, broken, clock)
		rule := Rule{
			Kind:  KindMessages,
			Daily: 3,
		}
		require.True(t, s.Allow("user:u_1", rule, 3).OK)
		s.flush() // Fails; fail open: the counts stay, enforced locally
		require.Equal(t, int64(3), s.Usage("user:u_1").Count(KindMessages))
		require.False(t, s.Allow("user:u_1", rule, 1).OK)

		// Recovery: the retained deltas are written once, not twice
		s.setStore(good)
		s.flush()
		s.flush()
		rows, err := good.readKeys(context.Background(), s.today(), []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 3}, rowsByKeyKind(rows))
		require.Equal(t, int64(3), s.Usage("user:u_1").Count(KindMessages))
	})
}

func TestService_Persist_DayFromDatabaseClock(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		// This node's clock is years off; the usage day still follows the database's clock
		clock := &testClock{t: time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)}
		s := newPersistentTestService(t, newStore(), clock)
		s.flush()
		require.Equal(t, time.Now().UTC().Format("2006-01-02"), s.today())
	})
}

func TestService_DayRollover(t *testing.T) {
	// In memory (no store), the day follows the local clock: at the reset time the counts start
	// over at the next flush, with no reset job
	s, clock := newTestServiceWithClock(t) // Noon UTC
	rule := Rule{
		Kind:  KindMessages,
		Daily: 3,
	}
	require.True(t, s.Allow("k", rule, 3).OK)
	require.False(t, s.Allow("k", rule, 1).OK)
	clock.advance(11 * time.Hour)
	s.flush()
	require.False(t, s.Allow("k", rule, 1).OK) // Still the same day
	clock.advance(2 * time.Hour)
	s.flush()
	require.Equal(t, int64(0), s.Usage("k").Count(KindMessages))
	require.True(t, s.Allow("k", rule, 1).OK)
	require.Equal(t, int64(1), s.Usage("k").Count(KindMessages))
}

func TestService_Persist_DayRolloverKeepsYesterdaysRows(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		store := newStore()
		s := newPersistentTestService(t, store, clock)
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 4).OK)
		s.flush()
		today := s.today()
		s.RollDayForTest("2099-01-01")
		require.Equal(t, int64(0), s.Usage("user:u_1").Count(KindMessages))
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 1).OK)
		s.flush()
		rows, err := store.readKeys(context.Background(), today, []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 4}, rowsByKeyKind(rows))
		rows, err = store.readKeys(context.Background(), "2099-01-01", []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 1}, rowsByKeyKind(rows))
	})
}

func TestService_Persist_EvictedKeyReadBack(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		rule := Rule{
			Kind:  KindMessages,
			Daily: 10,
		}
		require.True(t, s.Allow("user:u_1", rule, 5).OK)
		s.flush()
		clock.advance(promotedRetention + 10*time.Minute) // A key with a row stays longer (see TestService_Prune_KeepsPromotedKeysLonger)
		s.Prune(5 * time.Minute)
		require.Equal(t, 0, s.Size())

		// The key comes back: admitted at once (fail open), its row read back in the background
		require.True(t, s.Allow("user:u_1", rule, 1).OK)
		require.Equal(t, int64(1), s.Usage("user:u_1").Count(KindMessages))
		s.readMisses()
		require.Equal(t, int64(6), s.Usage("user:u_1").Count(KindMessages))
		require.False(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 6,
		}, 1).OK)

		// Below the threshold there is no row, so the count resumes at zero, as designed
		below := Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 3,
		}
		require.True(t, s.Allow("ip:1.2.3.4", below, 2).OK)
		s.flush()
		clock.advance(10 * time.Minute)
		s.Prune(5 * time.Minute)
		require.True(t, s.Allow("ip:1.2.3.4", below, 1).OK)
		s.readMisses()
		require.Equal(t, int64(1), s.Usage("ip:1.2.3.4").Count(KindMessages))
	})
}

func TestService_Persist_Prune_KeepsUnflushed(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 5).OK)
		clock.advance(promotedRetention + 10*time.Minute)
		s.Prune(5 * time.Minute)
		require.Equal(t, 1, s.Size()) // Not flushed yet, so not evicted
		s.flush()
		s.Prune(5 * time.Minute)
		require.Equal(t, 0, s.Size())
	})
}

func TestService_Persist_CloseFlushes_CrashLoses(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		store := newStore()
		rule := Rule{
			Kind:  KindMessages,
			Daily: 10,
		}
		graceful := newPersistentTestService(t, store, clock)
		require.True(t, graceful.Allow("user:u_1", rule, 5).OK)
		require.Nil(t, graceful.Close())
		crashed := newPersistentTestService(t, store, clock)
		require.True(t, crashed.Allow("user:u_2", rule, 5).OK) // Never flushed, never closed

		restarted := newPersistentTestService(t, newStore(), clock)
		usage, err := restarted.Lookup("user:u_1")
		require.Nil(t, err)
		require.Equal(t, int64(5), usage.Count(KindMessages))
		usage, err = restarted.Lookup("user:u_2")
		require.Nil(t, err)
		require.Equal(t, int64(0), usage.Count(KindMessages))
	})
}

func TestService_Persist_PruneStore(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		store := newStore()
		s := newPersistentTestService(t, store, clock)
		old := clock.now().Add(-usageRetention - 24*time.Hour).UTC().Format("2006-01-02")
		_, err := store.flush(context.Background(), old, []usageRow{{
			key:   "user:u_1",
			kind:  KindMessages,
			count: 1,
		}})
		require.Nil(t, err)
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 1).OK)
		s.flush()
		require.Nil(t, s.PruneStore())
		rows, err := store.readKeys(context.Background(), old, []string{"user:u_1"})
		require.Nil(t, err)
		require.Empty(t, rows)
		rows, err = store.readKeys(context.Background(), s.today(), []string{"user:u_1"})
		require.Nil(t, err)
		require.Len(t, rows, 1)
	})
}

func TestService_Persist_Loops(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		// With the loops on, two services converge on their own
		conf := &Config{
			ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
			FlushInterval: 100 * time.Millisecond,
		}
		a := New(conf, newStore())
		b := New(conf, newStore())
		t.Cleanup(func() {
			a.Close()
			b.Close()
		})
		rule := Rule{
			Kind:  KindMessages,
			Daily: 100,
		}
		require.True(t, a.Allow("user:u_1", rule, 3).OK)
		require.True(t, b.Allow("user:u_1", rule, 4).OK)
		require.Eventually(t, func() bool {
			return a.Usage("user:u_1").Count(KindMessages) == 7 && b.Usage("user:u_1").Count(KindMessages) == 7
		}, 5*time.Second, 20*time.Millisecond)
	})
}

func BenchmarkFlush(b *testing.B) {
	// 10,000 promoted keys with a delta each, flushed to SQLite in one go
	store, err := NewSQLiteStore(filepath.Join(b.TempDir(), "usage.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	s := New(&Config{
		ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
		FlushInterval: time.Hour,
	}, store)
	defer s.Close()
	rule := Rule{
		Kind:  KindMessages,
		Daily: 1 << 40,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for k := 0; k < 10000; k++ {
			s.Allow(fmt.Sprintf("ip:10.%d.%d.%d", k>>16, (k>>8)&255, k&255), rule, 1)
		}
		b.StartTimer()
		s.flush()
	}
}

func TestService_Persist_MoneyKindsLoadSynchronously(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		// Emails and calls cost money: the first use of a key reads its row before deciding, so an
		// evicted or restarted key cannot send another burst. Messages stay fail open (read in the
		// background), as the hot path never waits for the store.
		clock := &testClock{t: time.Now()}
		a := newPersistentTestService(t, newStore(), clock)
		b := newPersistentTestService(t, newStore(), clock)
		emails := Rule{
			Kind:   KindEmails,
			Burst:  10,
			Refill: time.Hour,
			Daily:  5,
		}
		calls := Rule{
			Kind:  KindCalls,
			Daily: 2,
		}
		messages := Rule{
			Kind:  KindMessages,
			Daily: 3,
		}
		require.True(t, a.Allow("user:u_1", emails, 5).OK)
		require.True(t, a.Allow("user:u_1", calls, 2).OK)
		require.True(t, a.Allow("user:u_1", messages, 3).OK)
		a.flush()
		require.False(t, b.Allow("user:u_1", emails, 1).OK) // No readMisses: the row was read on the spot
		require.False(t, b.Allow("user:u_1", calls, 1).OK)
		require.True(t, b.Allow("user:u_2", messages, 1).OK)  // Unknown key, read in the background
		require.False(t, b.Allow("user:u_1", messages, 1).OK) // Loaded by the email check above, so the message count is known too
	})
}

func TestService_Prune_KeepsPromotedKeysLonger(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		// A key with a row is one near its quota; it stays in memory for promotedRetention, so an
		// abuser cannot get a fresh local count by idling a few minutes
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		require.True(t, s.Allow("ip:heavy", Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 3,
		}, 3).OK)
		require.True(t, s.Allow("ip:light", Rule{
			Kind:      KindMessages,
			Daily:     250,
			Threshold: 3,
		}, 1).OK)
		s.flush()
		clock.advance(10 * time.Minute)
		s.Prune(5 * time.Minute)
		require.Equal(t, 1, s.Size())
		require.Equal(t, int64(3), s.Usage("ip:heavy").Count(KindMessages))
		clock.advance(promotedRetention)
		s.Prune(5 * time.Minute)
		require.Equal(t, 0, s.Size())
	})
}

func TestService_Forget(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		rule := Rule{
			Kind:  KindMessages,
			Daily: 10,
		}
		require.True(t, s.Allow("user:u_1", rule, 5).OK)
		require.False(t, s.Forget("user:u_1")) // Not written yet: kept
		require.Equal(t, 1, s.Size())
		s.flush()
		require.True(t, s.Forget("user:u_1"))
		require.Equal(t, 0, s.Size())
		require.False(t, s.Forget("user:u_1")) // Unknown
		usage, err := s.Lookup("user:u_1")     // Back from the store
		require.Nil(t, err)
		require.Equal(t, int64(5), usage.Count(KindMessages))
	})
}

// The mesh tests below cover what a node tells its peers about its buckets (TakeDeltas) and
// what it does with what the peers tell it (Burn): see docs/10-cluster.md, section 10.

func TestService_TakeDeltas_NonTrivialOnly(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	for i := 0; i < 4; i++ {
		require.True(t, s.Allow("quiet", rule, 1).OK)
	}
	for i := 0; i < 5; i++ {
		require.True(t, s.Allow("busy", rule, 1).OK)
	}
	for i := 0; i < 31; i++ {
		require.True(t, s.Allow("low", rule, 1).OK) // Bucket below half
	}
	deltas := s.TakeDeltas()
	require.ElementsMatch(t, []Delta{
		{
			Key:  "busy",
			Kind: KindRequests,
			N:    5,
		},
		{
			Key:  "low",
			Kind: KindRequests,
			N:    31,
		},
	}, deltas)

	// Taken deltas are gone; the quiet key's delta keeps accumulating until it is worth sending
	require.Empty(t, s.TakeDeltas())
	require.True(t, s.Allow("quiet", rule, 1).OK)
	require.Equal(t, []Delta{{
		Key:  "quiet",
		Kind: KindRequests,
		N:    5,
	}}, s.TakeDeltas())

	// A key whose bucket is below half sends every delta, however small
	require.True(t, s.Allow("low", rule, 1).OK)
	require.Equal(t, []Delta{{
		Key:  "low",
		Kind: KindRequests,
		N:    1,
	}}, s.TakeDeltas())
}

func TestService_TakeDeltas_MeshKindsOnly(t *testing.T) {
	// Daily quotas go through the store, not the mesh; bytes and strikes do go through the mesh
	s, _ := newTestServiceWithClock(t)
	require.True(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 100,
	}, 50).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:   KindEmails,
		Burst:  10,
		Refill: time.Hour,
		Daily:  10,
	}, 5).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Second,
	}, 600).OK)
	require.True(t, s.Allow("k", Rule{
		Kind:   KindStrikes,
		Burst:  100,
		Refill: 6 * time.Second,
	}, 10).OK)
	require.ElementsMatch(t, []Delta{
		{
			Key:  "k",
			Kind: KindBandwidth,
			N:    600,
		},
		{
			Key:  "k",
			Kind: KindStrikes,
			N:    10,
		},
	}, s.TakeDeltas())
}

func TestService_Burn_DebtCappedAtOneBurst(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	for i := 0; i < 60; i++ {
		require.True(t, s.Allow("k", rule, 1).OK)
	}
	s.Burn("k", KindRequests, 120) // Two peers admitted 60 each
	require.Equal(t, int64(0), s.Tokens("k", rule))
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, 61*5*time.Second, d.RetryAfter) // One burst of debt, not two: 5 min to the first token
	clock.advance(5 * time.Minute)
	require.False(t, s.Allow("k", rule, 1).OK)
	clock.advance(5 * time.Second)
	require.True(t, s.Allow("k", rule, 1).OK)

	// Burning never counts as this node's usage, so it is not gossiped back
	require.Equal(t, int64(61), s.Usage("k").Count(KindRequests))
	s.TakeDeltas()
	s.Burn("k", KindRequests, 10)
	require.Empty(t, s.TakeDeltas())
}

func TestService_Burn_UnknownKeyOrBucketIgnored(t *testing.T) {
	// A key this node never served has a full bucket here anyway (one burst per node, once);
	// nothing to charge until it shows up
	s, _ := newTestServiceWithClock(t)
	s.Burn("unknown", KindRequests, 100)
	require.Equal(t, 0, s.Size())
	rule := Rule{
		Kind:   KindRequests,
		Burst:  10,
		Refill: time.Hour,
	}
	require.True(t, s.Allow("k", Rule{
		Kind:  KindMessages,
		Daily: 10,
	}, 1).OK)
	s.Burn("k", KindRequests, 100) // Known key, but its request bucket was never used
	require.Equal(t, int64(10), s.Tokens("k", rule))
}

func TestService_Burn_ThreeNodesConverge(t *testing.T) {
	// The plan's bound: three nodes, an abuser spread evenly, burst 60 and 1 token per 5 s.
	// 180 go through in the first round; after the exchange every bucket is at one burst of
	// debt, and from then on the cluster admits the configured rate, not three times it.
	clock := &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	conf := &Config{
		ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
		FlushInterval: time.Hour,
	}
	nodes := []*Service{newService(conf, clock.now, nil), newService(conf, clock.now, nil), newService(conf, clock.now, nil)}
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	admitted := 0
	exchange := func() {
		deltas := make([][]Delta, len(nodes))
		for i, n := range nodes {
			deltas[i] = n.TakeDeltas()
		}
		for i, n := range nodes {
			for j, ds := range deltas {
				if i == j {
					continue
				}
				for _, d := range ds {
					n.Burn(d.Key, d.Kind, d.N)
				}
			}
		}
	}
	// Minute one: 5 s rounds, the abuser hammers every node between rounds
	for round := 0; round < 12; round++ {
		for i := 0; i < 100; i++ {
			for _, n := range nodes {
				if n.Allow("abuser", rule, 1).OK {
					admitted++
				}
			}
		}
		exchange()
		clock.advance(5 * time.Second)
	}
	require.Equal(t, 180, admitted) // The first round; the debt then locks every node for 5 min
	// Hour one: 180 once, then 12 per minute from minute six, shared across the nodes
	for round := 12; round < 720; round++ {
		for i := 0; i < 100; i++ {
			for _, n := range nodes {
				if n.Allow("abuser", rule, 1).OK {
					admitted++
				}
			}
		}
		exchange()
		clock.advance(5 * time.Second)
	}
	require.InDelta(t, 840, admitted, 30) // vs 780 ideal and 2,340 today (see the cluster doc, section 10)
}

func TestService_Persist_DayRolloverWritesYesterdaysPending(t *testing.T) {
	// Codex review #2: the rollover cleared what was counted in the day's last flush interval
	// before it was written, so every day lost its last few seconds of usage
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		store := newStore()
		s := newPersistentTestService(t, store, clock)
		yesterday := s.today()
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 4).OK) // Not flushed
		s.RollDayForTest("2099-01-01")
		require.Equal(t, int64(0), s.Usage("user:u_1").Count(KindMessages)) // The new day starts at zero
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 1).OK)
		s.flush()
		rows, err := store.readKeys(context.Background(), yesterday, []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 4}, rowsByKeyKind(rows))
		rows, err = store.readKeys(context.Background(), "2099-01-01", []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 1}, rowsByKeyKind(rows))
		s.flush() // Written once, not again
		rows, err = store.readKeys(context.Background(), yesterday, []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 4}, rowsByKeyKind(rows))
	})
}

func TestService_Persist_OutageAcrossMidnightKeepsDeltas(t *testing.T) {
	// An outage spanning midnight must not lose the retained deltas of the day before
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		good := newStore()
		broken := newStore()
		closed, err := sql.Open("sqlite3", ":memory:")
		require.Nil(t, err)
		require.Nil(t, closed.Close())
		broken.db = db.New(&db.Host{DB: closed}, nil)
		s := newPersistentTestService(t, broken, clock)
		yesterday := s.today()
		require.True(t, s.Allow("user:u_1", Rule{
			Kind:  KindMessages,
			Daily: 10,
		}, 3).OK)
		s.flush() // Fails: retained
		s.RollDayForTest("2099-01-01")
		s.flush() // Still failing: still retained
		s.setStore(good)
		s.flush()
		rows, err := good.readKeys(context.Background(), yesterday, []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 3}, rowsByKeyKind(rows))
	})
}

// Codex review #1: a read of the store and a write's retirement must not interleave. Both tests
// start from a key that another node counted to 10 and this node to 3 (written, not yet read).

func newReadWriteRaceSetup(t *testing.T, newStore func() *Store) *Service {
	t.Helper()
	clock := &testClock{t: time.Now()}
	other := newPersistentTestService(t, newStore(), clock)
	require.True(t, other.Allow("user:u_1", Rule{
		Kind:  KindMessages,
		Daily: 100,
	}, 10).OK)
	require.Nil(t, other.Close())
	s := newPersistentTestService(t, newStore(), clock)
	require.True(t, s.Allow("user:u_1", Rule{
		Kind:  KindMessages,
		Daily: 100,
	}, 3).OK) // Queued for a read-back
	return s
}

// runConcurrently starts f and waits for it a little (long enough to finish if nothing holds it
// up); the returned channel closes when f is done
func runConcurrently(f func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		f()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
	}
	return done
}

func TestService_Persist_ReadAfterCommitBeforeRetire(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newReadWriteRaceSetup(t, newStore)
		var read <-chan struct{}
		s.afterWriteHook = func() {
			read = runConcurrently(s.readMisses) // Reads 13, which already includes the 3 about to be retired
		}
		s.write(context.Background(), s.store.Load())
		<-read
		require.Equal(t, int64(13), s.Usage("user:u_1").Count(KindMessages)) // Not 16
	})
}

func TestService_Persist_ReadBeforeCommitAppliedAfterRetire(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newReadWriteRaceSetup(t, newStore)
		var written <-chan struct{}
		s.beforeApplyHook = func() {
			s.beforeApplyHook = nil
			written = runConcurrently(func() {
				s.write(context.Background(), s.store.Load()) // Commits and retires the 3 the read did not see
			})
		}
		s.readMisses() // Reads 10
		<-written
		require.Equal(t, int64(13), s.Usage("user:u_1").Count(KindMessages)) // Not 10
	})
}

func TestService_Refund_CancelsUnsentDelta(t *testing.T) {
	// Codex review #7: a refund left the mesh delta, so peers were charged for refunded bytes
	// (the LimitReader/LimitWriter rollback path)
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindBandwidth,
		Burst:  1000,
		Refill: time.Second,
	}
	require.True(t, s.Allow("k", rule, 900).OK)
	s.Refund("k", KindBandwidth, 900)
	require.Empty(t, s.TakeDeltas())

	// Refunded after the delta went out: the next round carries the correction, and a peer applies
	// it as a refund, never past the burst
	require.True(t, s.Allow("k", rule, 900).OK)
	require.Equal(t, []Delta{{
		Key:  "k",
		Kind: KindBandwidth,
		N:    900,
	}}, s.TakeDeltas())
	s.Refund("k", KindBandwidth, 900)
	require.Equal(t, []Delta{{
		Key:  "k",
		Kind: KindBandwidth,
		N:    -900,
	}}, s.TakeDeltas())
	peer, _ := newTestServiceWithClock(t)
	require.True(t, peer.Allow("k", rule, 100).OK)
	peer.Burn("k", KindBandwidth, 900)
	require.Equal(t, int64(0), peer.Tokens("k", rule))
	peer.Burn("k", KindBandwidth, -900)
	require.Equal(t, int64(900), peer.Tokens("k", rule))
	peer.Burn("k", KindBandwidth, -5000)
	require.Equal(t, int64(1000), peer.Tokens("k", rule))
}

func TestService_Burn_RefillsToReceiptTime(t *testing.T) {
	// Codex review #6: Burn charged stale tokens and left the refill clock behind, so the next local
	// use got the whole idle interval's refill, including what the peers had already used
	s, clock := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	require.True(t, s.Allow("k", rule, 60).OK)
	for i := 0; i < 120; i++ { // Ten minutes: a peer uses each token as it refills
		clock.advance(5 * time.Second)
		s.Burn("k", KindRequests, 1)
	}
	require.Equal(t, int64(0), s.Tokens("k", rule)) // Not 60
}

func TestService_Burn_AlternatingNodes(t *testing.T) {
	// A drained key whose traffic stays on node B for ten minutes and then moves to node A (the
	// load balancer's choice): A was idle meanwhile, but B used every token as it refilled, so A must
	// not hand out a fresh burst when the traffic arrives
	clock := &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	conf := &Config{
		ResetTime:     time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
		FlushInterval: time.Hour,
	}
	a, b := newService(conf, clock.now, nil), newService(conf, clock.now, nil)
	rule := Rule{
		Kind:   KindRequests,
		Burst:  60,
		Refill: 5 * time.Second,
	}
	exchange := func() {
		da, db := a.TakeDeltas(), b.TakeDeltas()
		for _, d := range da {
			b.Burn(d.Key, d.Kind, d.N)
		}
		for _, d := range db {
			a.Burn(d.Key, d.Kind, d.N)
		}
	}
	require.True(t, a.Allow("k", rule, 30).OK)
	require.True(t, b.Allow("k", rule, 30).OK)
	exchange() // Both at zero: 60 used of 60
	round := func(n *Service) int {
		clock.advance(5 * time.Second)
		admitted := 0
		for i := 0; i < 10; i++ {
			if n.Allow("k", rule, 1).OK {
				admitted++
			}
		}
		exchange()
		return admitted
	}
	for i := 0; i < 120; i++ {
		round(b)
	}
	onA := 0
	for i := 0; i < 12; i++ { // One minute on A
		onA += round(a)
	}
	require.InDelta(t, 12, onA, 2) // The configured rate, not a fresh burst on top
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

func TestService_DayRollover_KeepsBuckets(t *testing.T) {
	// The day rollover starts the counts over, but the buckets keep their tokens: an email bucket
	// emptied right before the rollover is not refilled by it (the daily reset used to)
	s, _ := newTestServiceWithClock(t)
	rule := Rule{
		Kind:   KindEmails,
		Burst:  2,
		Refill: time.Hour,
		Daily:  3,
	}
	require.True(t, s.Allow("k", rule, 2).OK)
	s.RollDayForTest("2099-01-01")
	require.Equal(t, int64(0), s.Usage("k").Count(KindEmails))
	require.Equal(t, int64(0), s.Tokens("k", rule))
	d := s.Allow("k", rule, 1)
	require.False(t, d.OK)
	require.Equal(t, time.Hour, d.RetryAfter) // The bucket is what denies, not the day
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

// Fails if the day rollover also clears buckets; a rollover racing with publishes grants only one new daily allowance.
func TestService_DayRollover_ConcurrentAllow(t *testing.T) {
	for round := 0; round < 20; round++ {
		s := newTestService(t)
		rule := Rule{
			Kind:  KindMessages,
			Burst: 81,
			Daily: 40,
		}
		require.True(t, s.Allow("hot", rule, 40).OK)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var admitted atomic.Int64
		for g := 0; g < 16; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 40; i++ {
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
			s.RollDayForTest("2099-01-01")
		}()
		close(start)
		wg.Wait()
		for i := 0; i < 40; i++ {
			if s.Allow("hot", rule, 1).OK {
				admitted.Add(1)
			}
		}
		require.Equal(t, int64(40), admitted.Load())
		require.Equal(t, admitted.Load(), s.Usage("hot").Count(rule.Kind))
		require.Equal(t, int64(1), s.Tokens("hot", rule))
	}
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
