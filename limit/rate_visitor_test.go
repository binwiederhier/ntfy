package limit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestService_RateVisitor_SetTouchNote(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	require.Nil(t, s.RateVisitor("upAAAAAAAAAAAA"))

	// A refused push makes the topic known, without a rate visitor; a second one does not move its time
	s.NoteTopic("upAAAAAAAAAAAA")
	noted := clock.now()
	clock.advance(time.Hour)
	s.NoteTopic("upAAAAAAAAAAAA")
	require.Equal(t, &RateVisitor{
		SeenAt: noted,
	}, s.RateVisitor("upAAAAAAAAAAAA"))

	// A subscriber becomes the rate visitor; the latest one wins
	s.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
		Key:    "ip:1.2.3.4",
		SeenAt: clock.now(),
	})
	s.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
		Key:    "user:u_1",
		UserID: "u_1",
		SeenAt: clock.now(),
	})
	set := clock.now()
	require.Equal(t, userRateVisitor(set), s.RateVisitor("upAAAAAAAAAAAA"))

	// A touch keeps the rate visitor, and moves its time only once rateVisitorTouchInterval has passed
	clock.advance(rateVisitorTouchInterval - time.Second)
	s.TouchRateVisitor("upAAAAAAAAAAAA")
	require.Equal(t, userRateVisitor(set), s.RateVisitor("upAAAAAAAAAAAA"))
	clock.advance(time.Second)
	s.TouchRateVisitor("upAAAAAAAAAAAA")
	require.Equal(t, userRateVisitor(clock.now()), s.RateVisitor("upAAAAAAAAAAAA"))

	// A touch of an unknown topic makes it known, without a rate visitor
	s.TouchRateVisitor("upBBBBBBBBBBBB")
	require.Equal(t, &RateVisitor{
		SeenAt: clock.now(),
	}, s.RateVisitor("upBBBBBBBBBBBB"))
}

func TestService_RateVisitor_Prune(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.NoteTopic("upAAAAAAAAAAAA")
	clock.advance(time.Hour)
	s.NoteTopic("upBBBBBBBBBBBB")
	clock.advance(rateVisitorPruneAfter - 30*time.Minute)
	s.Prune(time.Hour)
	require.Nil(t, s.RateVisitor("upAAAAAAAAAAAA"))
	require.NotNil(t, s.RateVisitor("upBBBBBBBBBBBB"))
}

func TestService_Persist_RateVisitorSharedAcrossNodes(t *testing.T) {
	// A rate visitor set on one node is the rate visitor on the other after a sync; a touch on the other node
	// moves its time for both, and keeps the rate visitor
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		aged := time.Unix(time.Now().Add(-25*time.Hour).Unix(), 0).UTC()
		a.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "user:u_1",
			UserID: "u_1",
			SeenAt: aged,
		})
		a.sync()
		b.sync()
		require.Equal(t, userRateVisitor(aged), b.RateVisitor("upAAAAAAAAAAAA"))
		b.TouchRateVisitor("upAAAAAAAAAAAA")
		require.NotNil(t, b.RateVisitor("upAAAAAAAAAAAA"))
		touched := b.RateVisitor("upAAAAAAAAAAAA").SeenAt
		require.True(t, touched.After(aged))
		b.sync()
		a.sync()
		require.Equal(t, userRateVisitor(touched), a.RateVisitor("upAAAAAAAAAAAA"))
	})
}

func TestService_Persist_RateVisitorSurvivesRestart(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := startTestService(t, newStore(), time.Now)
		seen := time.Unix(time.Now().Unix(), 0).UTC()
		s.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "ip:1.2.3.4",
			SeenAt: seen,
		})
		s.Close() // Writes what is unwritten
		s = startTestService(t, newStore(), time.Now)
		require.Equal(t, ipRateVisitor(seen), s.RateVisitor("upAAAAAAAAAAAA"))
	})
}

func TestService_Persist_NoteKeepsStoredRateVisitor(t *testing.T) {
	// A refused push on a node that has not read the rate visitor yet must not wipe it
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		a.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		a.sync()
		b.NoteTopic("upAAAAAAAAAAAA")
		b.sync()
		a.sync()
		for _, s := range []*Service{a, b} {
			require.NotNil(t, s.RateVisitor("upAAAAAAAAAAAA"))
			require.Equal(t, "ip:1.2.3.4", s.RateVisitor("upAAAAAAAAAAAA").Key)
		}
	})
}

func TestService_Persist_ReadBackKeepsUnwrittenRateVisitor(t *testing.T) {
	// A rate visitor set here and not written yet is not overwritten by an older row read back
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		a.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		a.sync()
		b.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "ip:5.6.7.8",
			SeenAt: time.Now(),
		})
		_, err := b.readRateVisitors(context.Background(), 0)
		require.Nil(t, err)
		require.NotNil(t, b.RateVisitor("upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", b.RateVisitor("upAAAAAAAAAAAA").Key)
		b.sync()
		a.sync()
		require.NotNil(t, a.RateVisitor("upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", a.RateVisitor("upAAAAAAAAAAAA").Key)
	})
}

func TestService_Persist_RateVisitorWriteFailureRetries(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		s := startTestService(t, store, time.Now)
		s.SetRateVisitor("upAAAAAAAAAAAA", &RateVisitor{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		writeRateVisitorChunk := store.writeRateVisitorChunk
		store.writeRateVisitorChunk = func(ctx context.Context, rows []rateVisitorRow, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		require.Empty(t, readAllRateVisitors(t, store, 0))
		store.writeRateVisitorChunk = writeRateVisitorChunk
		s.sync()
		require.Equal(t, "ip:1.2.3.4", readAllRateVisitors(t, store, 0)["upAAAAAAAAAAAA"].key)
	})
}

// userRateVisitor returns the rate visitor of user u_1, seen at the given time
func userRateVisitor(seen time.Time) *RateVisitor {
	return &RateVisitor{
		Key:    "user:u_1",
		UserID: "u_1",
		SeenAt: seen,
	}
}

// ipRateVisitor returns the rate visitor of the anonymous visitor 1.2.3.4, seen at the given time
func ipRateVisitor(seen time.Time) *RateVisitor {
	return &RateVisitor{
		Key:    "ip:1.2.3.4",
		SeenAt: seen,
	}
}
