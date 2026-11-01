package topic

import (
	"context"
	"errors"
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

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
}

func newTestServiceWithClock(t *testing.T) (*Service, *testClock) {
	t.Helper()
	clock := newTestClock()
	return newService(&Config{}, clock.now), clock
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

func TestService_SetTouchNote(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	require.Nil(t, s.State("upA"))

	// A refused push makes the topic known: first seen now, never subscribed, no rate visitor; a
	// second one changes nothing
	s.Note("upA")
	noted := clock.now()
	clock.advance(time.Hour)
	s.Note("upA")
	require.Equal(t, &State{FirstSeenAt: noted}, s.State("upA"))

	// A subscriber becomes the rate visitor, and counts as subscriber activity; the latest one wins
	s.SetRateVisitor("upA", &RateVisitor{IP: "1.2.3.4"})
	s.SetRateVisitor("upA", &RateVisitor{UserID: "u_1", IP: "1.2.3.4"})
	set := clock.now()
	require.Equal(t, &State{FirstSeenAt: noted, SubscribedAt: set, RateVisitor: userRateVisitor()}, s.State("upA"))

	// A touch keeps the rate visitor and moves the subscriber activity
	clock.advance(time.Minute)
	s.Touch("upA")
	touched := clock.now()
	require.Equal(t, &State{FirstSeenAt: noted, SubscribedAt: touched, RateVisitor: userRateVisitor()}, s.State("upA"))

	// A refused push to a known topic changes nothing, however old its subscriber activity
	clock.advance(48 * time.Hour)
	s.Note("upA")
	require.Equal(t, &State{FirstSeenAt: noted, SubscribedAt: touched, RateVisitor: userRateVisitor()}, s.State("upA"))

	// A touch or a subscribe of an unknown topic makes it known, first seen and subscribed now
	s.Touch("upB")
	require.Equal(t, &State{FirstSeenAt: clock.now(), SubscribedAt: clock.now()}, s.State("upB"))
	s.SetRateVisitor("upC", &RateVisitor{IP: "1.2.3.4"})
	require.Equal(t, &State{FirstSeenAt: clock.now(), SubscribedAt: clock.now(), RateVisitor: ipRateVisitor()}, s.State("upC"))
	require.Equal(t, 3, s.Size())
}

func TestService_Keepalive_Throttled(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.SetRateVisitor("upA", ipRateVisitor())
	set := clock.now()
	clock.advance(keepaliveInterval - time.Second)
	s.Keepalive("upA")
	require.Equal(t, set, s.State("upA").SubscribedAt)
	clock.advance(time.Second)
	s.Keepalive("upA")
	require.Equal(t, clock.now(), s.State("upA").SubscribedAt)
	require.Equal(t, ipRateVisitor(), s.State("upA").RateVisitor)

	// A keepalive of an unknown topic is a touch
	s.Keepalive("upB")
	require.Equal(t, &State{FirstSeenAt: clock.now(), SubscribedAt: clock.now()}, s.State("upB"))
}

func TestService_Note_Bounded(t *testing.T) {
	// Topics known only from refused pushes are capped; a subscriber frees a slot
	s, _ := newTestServiceWithClock(t)
	for i := 0; i < maxNoted; i++ {
		s.Note(fmt.Sprintf("up%012d", i))
	}
	require.Equal(t, maxNoted, s.Size())
	s.Note("upOneTooMany")
	require.Nil(t, s.State("upOneTooMany"))
	s.Touch("up000000000000")
	s.Note("upOneTooMany")
	require.NotNil(t, s.State("upOneTooMany"))
	s.Note("upTwoTooMany")
	require.Nil(t, s.State("upTwoTooMany"))
}

func TestService_Prune(t *testing.T) {
	// Topics are pruned pruneAfter after their last write, never while they have an unwritten change
	s, clock := newTestServiceWithClock(t)
	s.Note("upA")
	clock.advance(time.Hour)
	s.Touch("upB")
	clock.advance(pruneAfter - 30*time.Minute)
	s.prune(clock.now().Unix() - int64(pruneAfter/time.Second))
	require.Nil(t, s.State("upA"))
	require.NotNil(t, s.State("upB"))

	// A pruned topic may be noted again
	for i := 0; i < maxNoted; i++ {
		s.Note(fmt.Sprintf("up%012d", i))
	}
	s.Note("upOneTooMany")
	require.Nil(t, s.State("upOneTooMany"))
	clock.advance(pruneAfter + time.Hour)
	s.prune(clock.now().Unix() - int64(pruneAfter/time.Second))
	s.Note("upOneTooMany")
	require.NotNil(t, s.State("upOneTooMany"))
}

func TestService_Prune_AfterTwoWeeks(t *testing.T) {
	// The sync prunes topics two weeks after their last write: long enough that an abandoned
	// UnifiedPush topic still has its row when a push 7+ days later decides on the 410
	s, clock := newTestServiceWithClock(t)
	s.Touch("upA")
	clock.advance(2 * 24 * time.Hour)
	s.Touch("upB")
	clock.advance(13 * 24 * time.Hour)
	s.sync()
	require.Nil(t, s.State("upA"))    // Written 15 days ago
	require.NotNil(t, s.State("upB")) // Written 13 days ago
}

func TestService_RateVisitorUserIDs(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.SetRateVisitor("upA", &RateVisitor{UserID: "u_1", IP: "1.2.3.4"})
	s.SetRateVisitor("upB", &RateVisitor{IP: "1.2.3.4"})
	s.SetRateVisitor("upC", &RateVisitor{UserID: "u_2", IP: "5.6.7.8"})
	s.SetRateVisitor("upD", &RateVisitor{UserID: "u_1", IP: "5.6.7.8"})
	s.Note("upE")
	clock.advance(25 * time.Hour)
	s.Touch("upA")
	require.Equal(t, []string{"u_1"}, s.RateVisitorUserIDs(clock.now().Add(-24*time.Hour)))
	require.Equal(t, []string{"u_1", "u_2"}, s.RateVisitorUserIDs(clock.now().Add(-48*time.Hour)))
}

func TestService_SetForTest(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.SetRateVisitor("upA", ipRateVisitor())
	aged := &State{
		FirstSeenAt:  clock.now().Add(-48 * time.Hour),
		SubscribedAt: clock.now().Add(-25 * time.Hour),
		RateVisitor:  ipRateVisitor(),
	}
	s.SetForTest("upA", aged)
	require.Equal(t, aged, s.State("upA"))
	s.SetForTest("upB", &State{FirstSeenAt: clock.now().Add(-8 * 24 * time.Hour)})
	require.Equal(t, &State{FirstSeenAt: clock.now().Add(-8 * 24 * time.Hour)}, s.State("upB"))
}

func TestService_Persist_SharedAcrossNodes(t *testing.T) {
	// A rate visitor set on one node is the rate visitor on the other after a sync; a touch on the
	// other node moves the subscriber activity for both, and keeps the rate visitor
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", userRateVisitor())
		set := clock.now()
		a.sync()
		b.sync()
		require.Equal(t, &State{FirstSeenAt: set, SubscribedAt: set, RateVisitor: userRateVisitor()}, b.State("upA"))
		clock.advance(time.Hour)
		b.Touch("upA")
		b.sync()
		a.sync()
		require.Equal(t, &State{FirstSeenAt: set, SubscribedAt: clock.now(), RateVisitor: userRateVisitor()}, a.State("upA"))
		require.Equal(t, a.State("upA"), b.State("upA"))
	})
}

func TestService_Persist_SurvivesRestart(t *testing.T) {
	// Close writes what is unwritten; a start loads every row
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		s := startTestService(t, newStore(), clock.now)
		s.SetRateVisitor("upA", ipRateVisitor())
		s.Note("upB")
		s.Close()
		s = startTestService(t, newStore(), clock.now)
		require.Equal(t, &State{FirstSeenAt: clock.now(), SubscribedAt: clock.now(), RateVisitor: ipRateVisitor()}, s.State("upA"))
		require.Equal(t, &State{FirstSeenAt: clock.now()}, s.State("upB"))
		require.Equal(t, 2, s.Size())
	})
}

func TestService_Persist_NoteDoesNotReviveRateVisitor(t *testing.T) {
	// A refused push on a node that does not know the topic neither moves its subscriber activity
	// nor touches its rate visitor on the other nodes: it is insert-only
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", userRateVisitor())
		set := clock.now()
		a.sync()
		clock.advance(25 * time.Hour)
		b.Note("upA") // b has not read a's row yet
		b.sync()
		a.sync()
		b.sync()
		for _, s := range []*Service{a, b} {
			require.Equal(t, &State{FirstSeenAt: set, SubscribedAt: set, RateVisitor: userRateVisitor()}, s.State("upA"))
		}
	})
}

func TestService_Persist_TouchAfterPruneRestoresRow(t *testing.T) {
	// A touch written after another node pruned the row recreates it with the rate visitor: every
	// write carries the full row
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.SetRateVisitor("upA", userRateVisitor())
		set := clock.now()
		s.sync()
		deleted, err := store.prune(context.Background(), clock.now().Unix()+1)
		require.Nil(t, err)
		require.Equal(t, int64(1), deleted)
		clock.advance(time.Hour)
		s.Touch("upA")
		s.sync()
		require.Equal(t, row{topic: "upA", firstSeenAt: set.Unix(), subscribedAt: clock.now().Unix(), userID: "u_1", ip: "1.2.3.4", updatedAt: clock.now().Unix()}, readAll(t, store, 0)["upA"])
	})
}

func TestService_Persist_ReadBackKeepsUnwrittenRateVisitor(t *testing.T) {
	// A rate visitor set here and not written yet is not replaced by a row read back; once
	// written, it is the one every node has (last write wins)
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", ipRateVisitor())
		a.sync()
		b.SetRateVisitor("upA", &RateVisitor{IP: "5.6.7.8"})
		_, err := b.read(context.Background(), 0)
		require.Nil(t, err)
		require.Equal(t, &RateVisitor{IP: "5.6.7.8"}, b.State("upA").RateVisitor)
		b.sync()
		a.sync()
		require.Equal(t, &RateVisitor{IP: "5.6.7.8"}, a.State("upA").RateVisitor)
	})
}

func TestService_Persist_MergesFirstSeenAndSubscribed(t *testing.T) {
	// Two nodes that know a topic differently converge on the earliest first observation and the
	// latest subscriber activity, whichever writes first
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.Note("upA")
		noted := clock.now()
		clock.advance(time.Hour)
		b.Touch("upA")
		touched := clock.now()
		b.sync()
		a.sync()
		b.sync()
		for _, s := range []*Service{a, b} {
			require.Equal(t, &State{FirstSeenAt: noted, SubscribedAt: touched}, s.State("upA"))
		}
	})
}

func TestService_Persist_Refresh(t *testing.T) {
	// Refresh reads the row from the store, so a node that has not read back a fresher row yet
	// decides on the fresh one, and keeps it
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", ipRateVisitor())
		set := clock.now()
		a.sync()
		require.Nil(t, b.State("upA"))
		topic, err := b.Refresh(context.Background(), "upA")
		require.Nil(t, err)
		require.Equal(t, &State{FirstSeenAt: set, SubscribedAt: set, RateVisitor: ipRateVisitor()}, topic)
		require.Equal(t, topic, b.State("upA"))

		// A fresher row replaces what b knows
		clock.advance(time.Hour)
		a.Touch("upA")
		a.sync()
		topic, err = b.Refresh(context.Background(), "upA")
		require.Nil(t, err)
		require.Equal(t, clock.now(), topic.SubscribedAt)
		require.Equal(t, clock.now(), b.State("upA").SubscribedAt)

		// An unknown topic stays unknown; a failed read is an error
		topic, err = b.Refresh(context.Background(), "upB")
		require.Nil(t, err)
		require.Nil(t, topic)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = b.Refresh(ctx, "upA")
		require.NotNil(t, err)
	})
}

func TestService_Persist_WriteFailureRetries(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.SetRateVisitor("upA", ipRateVisitor())
		writeChunk := store.writeChunk
		store.writeChunk = func(ctx context.Context, rows []row, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		require.Empty(t, readAll(t, store, 0))
		store.writeChunk = writeChunk
		s.sync()
		require.Equal(t, "1.2.3.4", readAll(t, store, 0)["upA"].ip)
	})
}

func TestService_Persist_PrunesTableAndMemory(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.Note("upA")
		s.sync()
		clock.advance(pruneAfter + 2*time.Hour)
		s.Note("upB")
		s.sync() // Prunes: an hour has passed since the last one
		require.Nil(t, s.State("upA"))
		require.NotNil(t, s.State("upB"))
		require.Equal(t, []string{"upB"}, topicsOf(readAll(t, store, 0)))

		// A topic with an unwritten change is never pruned from memory, however old its last write
		s.SetRateVisitor("upC", ipRateVisitor())
		store.writeChunk = func(ctx context.Context, rows []row, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		s.mu.Lock()
		s.entries["upC"].updatedAt = 1
		s.mu.Unlock()
		clock.advance(pruneInterval)
		s.sync()
		require.NotNil(t, s.State("upC"))
	})
}

func TestService_WithoutStore(t *testing.T) {
	s := New(&Config{})
	require.Nil(t, s.Start(context.Background()))
	s.Note("upA")
	topic, err := s.Refresh(context.Background(), "upA")
	require.Nil(t, err)
	require.Equal(t, s.State("upA"), topic)
	s.Close()
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

// userRateVisitor returns the rate visitor of user u_1 at 1.2.3.4
func userRateVisitor() *RateVisitor {
	return &RateVisitor{
		UserID: "u_1",
		IP:     "1.2.3.4",
	}
}

// ipRateVisitor returns the rate visitor of the anonymous subscriber at 1.2.3.4
func ipRateVisitor() *RateVisitor {
	return &RateVisitor{
		IP: "1.2.3.4",
	}
}
