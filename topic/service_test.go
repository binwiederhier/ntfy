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

func (c *testClock) unix() int64 {
	return c.now().Unix()
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
	return newService(nil, clock.now), clock
}

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

func TestService_SetTouchNote(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	require.Nil(t, s.State("upA"))

	// A refused push makes the topic known: first seen now, never subscribed, no rate visitor; a
	// second one changes nothing
	noted := clock.unix()
	require.Equal(t, state("upA", noted, 0, "", ""), withoutUpdatedAt(s.Note("upA")))
	clock.advance(time.Hour)
	require.Equal(t, state("upA", noted, 0, "", ""), withoutUpdatedAt(s.Note("upA")))
	require.Equal(t, state("upA", noted, 0, "", ""), stateOf(s, "upA"))

	// A subscriber becomes the rate visitor, and counts as subscriber activity; the latest one wins
	s.SetRateVisitor("upA", "", "1.2.3.4")
	s.SetRateVisitor("upA", "u_1", "1.2.3.4")
	set := clock.unix()
	require.Equal(t, state("upA", noted, set, "u_1", "1.2.3.4"), stateOf(s, "upA"))

	// A touch keeps the rate visitor and moves the subscriber activity
	clock.advance(time.Minute)
	s.Touch("upA")
	touched := clock.unix()
	require.Equal(t, state("upA", noted, touched, "u_1", "1.2.3.4"), stateOf(s, "upA"))

	// A refused push to a known topic changes nothing, however old its subscriber activity
	clock.advance(48 * time.Hour)
	s.Note("upA")
	require.Equal(t, state("upA", noted, touched, "u_1", "1.2.3.4"), stateOf(s, "upA"))

	// A touch or a subscribe of an unknown topic makes it known, first seen and subscribed now
	s.Touch("upB")
	require.Equal(t, state("upB", clock.unix(), clock.unix(), "", ""), stateOf(s, "upB"))
	s.SetRateVisitor("upC", "", "1.2.3.4")
	require.Equal(t, state("upC", clock.unix(), clock.unix(), "", "1.2.3.4"), stateOf(s, "upC"))
	require.Equal(t, 3, s.Size())
}

func TestService_State_ReturnsCopy(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	s.SetRateVisitor("upA", "", "1.2.3.4")
	s.State("upA").RateVisitorIP = "5.6.7.8"
	s.Note("upA").RateVisitorIP = "5.6.7.8"
	require.Equal(t, "1.2.3.4", s.State("upA").RateVisitorIP)
}

func TestService_Keepalive_Throttled(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.SetRateVisitor("upA", "", "1.2.3.4")
	set := clock.unix()
	clock.advance(keepaliveInterval - time.Second)
	s.Keepalive("upA")
	require.Equal(t, set, s.State("upA").SubscribedAt)
	clock.advance(time.Second)
	s.Keepalive("upA")
	require.Equal(t, clock.unix(), s.State("upA").SubscribedAt)
	require.Equal(t, "1.2.3.4", s.State("upA").RateVisitorIP)

	// A keepalive of an unknown topic is a touch
	s.Keepalive("upB")
	require.Equal(t, state("upB", clock.unix(), clock.unix(), "", ""), stateOf(s, "upB"))
}

func TestService_Note_Bounded(t *testing.T) {
	// Topics known only from refused pushes are capped; a subscriber frees a slot
	s, _ := newTestServiceWithClock(t)
	for i := 0; i < maxNoted; i++ {
		s.Note(fmt.Sprintf("up%012d", i))
	}
	require.Equal(t, maxNoted, s.Size())
	require.Nil(t, s.Note("upOneTooMany"))
	require.Nil(t, s.State("upOneTooMany"))
	s.Touch("up000000000000")
	require.NotNil(t, s.Note("upOneTooMany"))
	require.Nil(t, s.Note("upTwoTooMany"))
}

func TestService_Prune(t *testing.T) {
	// Topics are pruned pruneAfter after their last write
	s, clock := newTestServiceWithClock(t)
	s.Note("upA")
	clock.advance(time.Hour)
	s.Touch("upB")
	clock.advance(pruneAfter - 30*time.Minute)
	s.prune(clock.unix() - int64(pruneAfter/time.Second))
	require.Nil(t, s.State("upA"))
	require.NotNil(t, s.State("upB"))

	// A pruned topic may be noted again
	for i := 0; i < maxNoted; i++ {
		s.Note(fmt.Sprintf("up%012d", i))
	}
	require.Nil(t, s.Note("upOneTooMany"))
	clock.advance(pruneAfter + time.Hour)
	s.prune(clock.unix() - int64(pruneAfter/time.Second))
	require.NotNil(t, s.Note("upOneTooMany"))
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

func TestService_Persist_SharedAcrossNodes(t *testing.T) {
	// A rate visitor set on one node is the rate visitor on the other after a sync; a touch on the
	// other node moves the subscriber activity for both, and keeps the rate visitor
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", "u_1", "1.2.3.4")
		set := clock.unix()
		a.sync()
		b.sync()
		require.Equal(t, state("upA", set, set, "u_1", "1.2.3.4"), stateOf(b, "upA"))
		clock.advance(time.Hour)
		b.Touch("upA")
		b.sync()
		a.sync()
		require.Equal(t, state("upA", set, clock.unix(), "u_1", "1.2.3.4"), stateOf(a, "upA"))
		require.Equal(t, a.State("upA"), b.State("upA"))
	})
}

func TestService_Persist_SurvivesRestart(t *testing.T) {
	// Close writes what is unwritten; a start loads every row
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		s := startTestService(t, newStore(), clock.now)
		s.SetRateVisitor("upA", "", "1.2.3.4")
		s.Note("upB")
		s.Close()
		s = startTestService(t, newStore(), clock.now)
		require.Equal(t, state("upA", clock.unix(), clock.unix(), "", "1.2.3.4"), stateOf(s, "upA"))
		require.Equal(t, state("upB", clock.unix(), 0, "", ""), stateOf(s, "upB"))
		require.Equal(t, 2, s.Size())
	})
}

func TestService_Persist_StartFailsIfLoadFails(t *testing.T) {
	// A start does not serve without the topics: if they cannot be loaded, it fails
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newService(newStore(), time.Now)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorContains(t, s.Start(ctx), "cannot load topics")
		s.Close()
	})
}

func TestService_Persist_NoteDoesNotReviveRateVisitor(t *testing.T) {
	// A refused push on a node that does not know the topic neither moves its subscriber activity
	// nor touches its rate visitor on the other nodes: it is insert-only
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", "u_1", "1.2.3.4")
		set := clock.unix()
		a.sync()
		clock.advance(25 * time.Hour)
		b.Note("upA") // b has not read a's row yet
		b.sync()
		a.sync()
		b.sync()
		for _, s := range []*Service{a, b} {
			require.Equal(t, state("upA", set, set, "u_1", "1.2.3.4"), stateOf(s, "upA"))
		}
	})
}

func TestService_Persist_TouchAfterPruneRecreatesRow(t *testing.T) {
	// A touch written after another node pruned the row recreates it without the rate visitor, since
	// a touch never writes one (it may be stale); this node keeps its own until the next subscribe
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.SetRateVisitor("upA", "u_1", "1.2.3.4")
		set := clock.unix()
		s.sync()
		deleted, err := store.prune(context.Background(), clock.unix()+1)
		require.Nil(t, err)
		require.Equal(t, int64(1), deleted)
		clock.advance(time.Hour)
		s.Touch("upA")
		s.sync()
		expected := state("upA", set, clock.unix(), "", "")
		expected.UpdatedAt = clock.unix()
		require.Equal(t, *expected, readAll(t, store, 0)["upA"])
		require.Equal(t, "1.2.3.4", s.State("upA").RateVisitorIP)
	})
}

func TestService_Persist_ReadBackKeepsUnwrittenRateVisitor(t *testing.T) {
	// A rate visitor set here and not written yet is not replaced by a row read back; once
	// written, it is the one every node has (last write wins)
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		a := startTestService(t, newStore(), clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", "", "1.2.3.4")
		a.sync()
		b.SetRateVisitor("upA", "", "5.6.7.8")
		_, err := b.read(context.Background(), 0)
		require.Nil(t, err)
		require.Equal(t, "5.6.7.8", b.State("upA").RateVisitorIP)
		b.sync()
		a.sync()
		require.Equal(t, "5.6.7.8", a.State("upA").RateVisitorIP)
	})
}

func TestService_Persist_TouchKeepsNewerRateVisitor(t *testing.T) {
	// A touch or keepalive does not write this node's rate visitor over a newer one set on
	// another node, and the node adopts the newer one even while its touch is unwritten
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		a := startTestService(t, store, clock.now)
		b := startTestService(t, newStore(), clock.now)
		a.SetRateVisitor("upA", "", "1.1.1.1")
		a.sync()
		clock.advance(keepaliveInterval)
		a.Keepalive("upA")
		a.Touch("upA")
		b.SetRateVisitor("upA", "u_2", "2.2.2.2")
		b.sync()
		a.sync()
		b.sync()
		a.sync()
		for _, s := range []*Service{a, b} {
			require.Equal(t, "2.2.2.2", s.State("upA").RateVisitorIP)
			require.Equal(t, "u_2", s.State("upA").RateVisitorUserID)
		}
		require.Equal(t, "2.2.2.2", readAll(t, store, 0)["upA"].RateVisitorIP)
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
		noted := clock.unix()
		clock.advance(time.Hour)
		b.Touch("upA")
		touched := clock.unix()
		b.sync()
		a.sync()
		b.sync()
		for _, s := range []*Service{a, b} {
			require.Equal(t, state("upA", noted, touched, "", ""), stateOf(s, "upA"))
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
		a.SetRateVisitor("upA", "", "1.2.3.4")
		set := clock.unix()
		a.sync()
		require.Nil(t, b.State("upA"))
		topic, err := b.Refresh(context.Background(), "upA")
		require.Nil(t, err)
		require.Equal(t, topic, b.State("upA"))
		require.Equal(t, state("upA", set, set, "", "1.2.3.4"), withoutUpdatedAt(topic))

		// A fresher row replaces what b knows
		clock.advance(time.Hour)
		a.Touch("upA")
		a.sync()
		topic, err = b.Refresh(context.Background(), "upA")
		require.Nil(t, err)
		require.Equal(t, clock.unix(), topic.SubscribedAt)
		require.Equal(t, clock.unix(), b.State("upA").SubscribedAt)

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
		s.SetRateVisitor("upA", "", "1.2.3.4")
		writeChunk := store.writeChunk
		store.writeChunk = func(ctx context.Context, rows []State, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		require.Empty(t, readAll(t, store, 0))
		store.writeChunk = writeChunk
		s.sync()
		require.Equal(t, "1.2.3.4", readAll(t, store, 0)["upA"].RateVisitorIP)
	})
}

func TestService_Persist_WriteFailureKeepsNewerChange(t *testing.T) {
	// A change made while a write fails is not undone by the failed rows: it is the one retried
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.SetRateVisitor("upA", "", "1.2.3.4")
		writeChunk := store.writeChunk
		store.writeChunk = func(ctx context.Context, rows []State, now int64) error {
			s.SetRateVisitor("upA", "", "5.6.7.8")
			return errors.New("database is down")
		}
		s.sync()
		store.writeChunk = writeChunk
		s.sync()
		require.Equal(t, "5.6.7.8", readAll(t, store, 0)["upA"].RateVisitorIP)
		require.Equal(t, "5.6.7.8", s.State("upA").RateVisitorIP)
	})
}

func TestService_Persist_WriteFailureKeepsUnwrittenRateVisitor(t *testing.T) {
	// A rate visitor whose write failed is retried even if a touch came in meanwhile
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := newTestClock()
		store := newStore()
		s := startTestService(t, store, clock.now)
		s.SetRateVisitor("upA", "u_1", "1.2.3.4")
		writeChunk := store.writeChunk
		store.writeChunk = func(ctx context.Context, rows []State, now int64) error {
			s.Touch("upA")
			return errors.New("database is down")
		}
		s.sync()
		store.writeChunk = writeChunk
		s.sync()
		require.Equal(t, "u_1", readAll(t, store, 0)["upA"].RateVisitorUserID)
		require.Equal(t, "1.2.3.4", readAll(t, store, 0)["upA"].RateVisitorIP)
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

		// A topic not written for pruneAfter is pruned with its unwritten change
		s.SetRateVisitor("upC", "", "1.2.3.4")
		store.writeChunk = func(ctx context.Context, rows []State, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		s.mu.Lock()
		s.entries["upC"].UpdatedAt = 1
		s.mu.Unlock()
		clock.advance(pruneInterval)
		s.sync()
		require.Nil(t, s.State("upC"))
		require.Empty(t, s.pending)
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

func TestService_WithoutStore_NothingWaitsToBeWritten(t *testing.T) {
	// Without a store nothing is ever written, so nothing may wait to be: pruned topics are gone for good
	s, clock := newTestServiceWithClock(t)
	s.Note("upA")
	s.Touch("upB")
	s.SetRateVisitor("upC", "", "1.2.3.4")
	clock.advance(pruneAfter + time.Hour)
	s.sync()
	require.Equal(t, 0, s.Size())
	require.Empty(t, s.pending)
}

// state returns the state of a topic as the tests expect it, without its write time
func state(topic string, firstSeenAt, subscribedAt int64, userID, ip string) *State {
	return &State{
		Topic:             topic,
		FirstSeenAt:       firstSeenAt,
		SubscribedAt:      subscribedAt,
		RateVisitorUserID: userID,
		RateVisitorIP:     ip,
	}
}

// stateOf returns the topic's state without its write time, which most tests do not check
func stateOf(s *Service, topic string) *State {
	return withoutUpdatedAt(s.State(topic))
}

// withoutUpdatedAt clears the state's write time, if there is a state
func withoutUpdatedAt(st *State) *State {
	if st != nil {
		st.UpdatedAt = 0
	}
	return st
}
