package limit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
)

// startTestService starts a service on the store with the given clock, whose worker never ticks
// on its own: the tests sync by hand. It is closed when the test ends.
func startTestService(t *testing.T, store *Store, now func() time.Time) *Service {
	t.Helper()
	s := newService(store, now)
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
		require.Nil(t, s.store.db.QueryRow(`SELECT updated_at FROM usage WHERE key = 'user:u_1'`).Scan(&updatedAt))
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
		require.Equal(t, int64(6), usageRowOf(t, a.store, "k").counts[0])
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
		require.Equal(t, usageRow{}, usageRowOf(t, s.store, "ip:1.2.3.4"))
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
		s.sync()
		require.Equal(t, int64(25), usageRowOf(t, s.store, "ip:1.2.3.4").counts[0])
		require.True(t, s.Allow("ip:1.2.3.4", rule, 1).OK)
		s.sync()
		require.Equal(t, int64(26), usageRowOf(t, s.store, "ip:1.2.3.4").counts[0])
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
		require.Equal(t, int64(40), usageRowOf(t, a.store, "ip:1.2.3.4").counts[0])
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
		s := newService(store, time.Now)
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

		// Another node rolled first: a leftover of the old period is dropped, and the row keeps the
		// new period and its counts
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
			counts:      [numDaily]int64{4, 0, 0},
		}, usageRowOf(t, store, "b"))
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
