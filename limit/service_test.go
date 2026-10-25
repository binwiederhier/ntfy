package limit

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
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
	conf := &Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC), FlushInterval: time.Hour}
	return newService(conf, clock.now, nil), clock
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

	// The next day starts over (see TestService_DayRollover)
	s.rollDayForTest("2099-01-01")
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
	rule := Rule{Kind: KindRequests, Burst: 60, Refill: 5 * time.Second, Daily: 1 << 40}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Allow("ip:1.2.3.4", rule, 1)
	}
}

func BenchmarkAllow_ManyKeys(b *testing.B) {
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)}, nil)
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

// The persistence tests below run on both backends (see forEachBackend in store_test.go). They
// drive the flush and the miss read-back by hand (the loops are off: a long interval), and
// treat two services on one store as two nodes.

func newPersistentTestService(t *testing.T, store *Store, clock *testClock) *Service {
	t.Helper()
	conf := &Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC), FlushInterval: time.Hour}
	s := newService(conf, clock.now, store)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestService_Persist_FlushAndReadBack(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		a := newPersistentTestService(t, newStore(), clock)
		b := newPersistentTestService(t, newStore(), clock)
		rule := Rule{Kind: KindMessages, Daily: 100}
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
		require.False(t, a.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 5}, 1).OK)
		require.True(t, a.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 6}, 1).OK)
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
		rule := Rule{Kind: KindMessages, Daily: 250, Threshold: 3} // 1% of the quota, rounded up
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
		require.True(t, s.Allow("ip:1.2.3.4", Rule{Kind: KindRequests, Burst: 100, Refill: time.Second}, 50).OK)
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
		rule := Rule{Kind: KindMessages, Daily: 3}
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
	rule := Rule{Kind: KindMessages, Daily: 3}
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
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 10}, 4).OK)
		s.flush()
		today := s.today()
		s.rollDayForTest("2099-01-01")
		require.Equal(t, int64(0), s.Usage("user:u_1").Count(KindMessages))
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 10}, 1).OK)
		s.write(context.Background(), store) // Not flush: that would sync the day back to the store's clock
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
		rule := Rule{Kind: KindMessages, Daily: 10}
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
		require.False(t, s.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 6}, 1).OK)

		// Below the threshold there is no row, so the count resumes at zero, as designed
		below := Rule{Kind: KindMessages, Daily: 250, Threshold: 3}
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
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 10}, 5).OK)
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
		rule := Rule{Kind: KindMessages, Daily: 10}
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
		_, err := store.flush(context.Background(), old, []usageRow{{key: "user:u_1", kind: KindMessages, count: 1}})
		require.Nil(t, err)
		require.True(t, s.Allow("user:u_1", Rule{Kind: KindMessages, Daily: 10}, 1).OK)
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
		conf := &Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC), FlushInterval: 100 * time.Millisecond}
		a := New(conf, newStore())
		b := New(conf, newStore())
		t.Cleanup(func() {
			a.Close()
			b.Close()
		})
		rule := Rule{Kind: KindMessages, Daily: 100}
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
	s := New(&Config{ResetTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC), FlushInterval: time.Hour}, store)
	defer s.Close()
	rule := Rule{Kind: KindMessages, Daily: 1 << 40}
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
		emails := Rule{Kind: KindEmails, Burst: 10, Refill: time.Hour, Daily: 5}
		calls := Rule{Kind: KindCalls, Daily: 2}
		messages := Rule{Kind: KindMessages, Daily: 3}
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
		require.True(t, s.Allow("ip:heavy", Rule{Kind: KindMessages, Daily: 250, Threshold: 3}, 3).OK)
		require.True(t, s.Allow("ip:light", Rule{Kind: KindMessages, Daily: 250, Threshold: 3}, 1).OK)
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
		rule := Rule{Kind: KindMessages, Daily: 10}
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
