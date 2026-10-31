package limit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestService_Payer_SetTouchNote(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	require.Nil(t, s.Payer("upAAAAAAAAAAAA"))

	// A refused push makes the topic known, without a payer; a second one does not move its time
	s.NoteTopic("upAAAAAAAAAAAA")
	noted := clock.now()
	clock.advance(time.Hour)
	s.NoteTopic("upAAAAAAAAAAAA")
	require.Equal(t, &Payer{
		SeenAt: noted,
	}, s.Payer("upAAAAAAAAAAAA"))

	// A subscriber becomes the payer; the latest one wins
	s.SetPayer("upAAAAAAAAAAAA", &Payer{
		Key:    "ip:1.2.3.4",
		SeenAt: clock.now(),
	})
	s.SetPayer("upAAAAAAAAAAAA", &Payer{
		Key:    "user:u_1",
		UserID: "u_1",
		SeenAt: clock.now(),
	})
	set := clock.now()
	require.Equal(t, userPayer(set), s.Payer("upAAAAAAAAAAAA"))

	// A touch keeps the payer, and moves its time only once payerTouchInterval has passed
	clock.advance(payerTouchInterval - time.Second)
	s.TouchPayer("upAAAAAAAAAAAA")
	require.Equal(t, userPayer(set), s.Payer("upAAAAAAAAAAAA"))
	clock.advance(time.Second)
	s.TouchPayer("upAAAAAAAAAAAA")
	require.Equal(t, userPayer(clock.now()), s.Payer("upAAAAAAAAAAAA"))

	// A touch of an unknown topic makes it known, without a payer
	s.TouchPayer("upBBBBBBBBBBBB")
	require.Equal(t, &Payer{
		SeenAt: clock.now(),
	}, s.Payer("upBBBBBBBBBBBB"))
}

func TestService_Payer_Prune(t *testing.T) {
	s, clock := newTestServiceWithClock(t)
	s.NoteTopic("upAAAAAAAAAAAA")
	clock.advance(time.Hour)
	s.NoteTopic("upBBBBBBBBBBBB")
	clock.advance(payerPruneAfter - 30*time.Minute)
	s.Prune(time.Hour)
	require.Nil(t, s.Payer("upAAAAAAAAAAAA"))
	require.NotNil(t, s.Payer("upBBBBBBBBBBBB"))
}

func TestService_Persist_PayerSharedAcrossNodes(t *testing.T) {
	// A payer set on one node is the payer on the other after a sync; a touch on the other node
	// moves its time for both, and keeps the payer
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		aged := time.Unix(time.Now().Add(-25*time.Hour).Unix(), 0).UTC()
		a.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "user:u_1",
			UserID: "u_1",
			SeenAt: aged,
		})
		a.sync()
		b.sync()
		require.Equal(t, userPayer(aged), b.Payer("upAAAAAAAAAAAA"))
		b.TouchPayer("upAAAAAAAAAAAA")
		require.NotNil(t, b.Payer("upAAAAAAAAAAAA"))
		touched := b.Payer("upAAAAAAAAAAAA").SeenAt
		require.True(t, touched.After(aged))
		b.sync()
		a.sync()
		require.Equal(t, userPayer(touched), a.Payer("upAAAAAAAAAAAA"))
	})
}

func TestService_Persist_PayerSurvivesRestart(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := startTestService(t, newStore(), time.Now)
		seen := time.Unix(time.Now().Unix(), 0).UTC()
		s.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "ip:1.2.3.4",
			SeenAt: seen,
		})
		s.Close() // Writes what is unwritten
		s = startTestService(t, newStore(), time.Now)
		require.Equal(t, ipPayer(seen), s.Payer("upAAAAAAAAAAAA"))
	})
}

func TestService_Persist_NoteKeepsStoredPayer(t *testing.T) {
	// A refused push on a node that has not read the payer yet must not wipe it
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		a.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		a.sync()
		b.NoteTopic("upAAAAAAAAAAAA")
		b.sync()
		a.sync()
		for _, s := range []*Service{a, b} {
			require.NotNil(t, s.Payer("upAAAAAAAAAAAA"))
			require.Equal(t, "ip:1.2.3.4", s.Payer("upAAAAAAAAAAAA").Key)
		}
	})
}

func TestService_Persist_ReadBackKeepsUnwrittenPayer(t *testing.T) {
	// A payer set here and not written yet is not overwritten by an older row read back
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		a := startTestService(t, newStore(), time.Now)
		b := startTestService(t, newStore(), time.Now)
		a.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		a.sync()
		b.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "ip:5.6.7.8",
			SeenAt: time.Now(),
		})
		_, err := b.readPayers(context.Background(), 0)
		require.Nil(t, err)
		require.NotNil(t, b.Payer("upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", b.Payer("upAAAAAAAAAAAA").Key)
		b.sync()
		a.sync()
		require.NotNil(t, a.Payer("upAAAAAAAAAAAA"))
		require.Equal(t, "ip:5.6.7.8", a.Payer("upAAAAAAAAAAAA").Key)
	})
}

func TestService_Persist_PayerWriteFailureRetries(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		store := newStore()
		s := startTestService(t, store, time.Now)
		s.SetPayer("upAAAAAAAAAAAA", &Payer{
			Key:    "ip:1.2.3.4",
			SeenAt: time.Now(),
		})
		writePayerChunk := store.writePayerChunk
		store.writePayerChunk = func(ctx context.Context, rows []payerRow, now int64) error {
			return errors.New("database is down")
		}
		s.sync()
		require.Empty(t, readAllPayers(t, store, 0))
		store.writePayerChunk = writePayerChunk
		s.sync()
		require.Equal(t, "ip:1.2.3.4", readAllPayers(t, store, 0)["upAAAAAAAAAAAA"].key)
	})
}

// userPayer returns the payer of user u_1, seen at the given time
func userPayer(seen time.Time) *Payer {
	return &Payer{
		Key:    "user:u_1",
		UserID: "u_1",
		SeenAt: seen,
	}
}

// ipPayer returns the payer of the anonymous visitor 1.2.3.4, seen at the given time
func ipPayer(seen time.Time) *Payer {
	return &Payer{
		Key:    "ip:1.2.3.4",
		SeenAt: seen,
	}
}
