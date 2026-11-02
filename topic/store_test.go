package topic

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
)

// forEachBackend runs f against a fresh topic table on SQLite and on Postgres (if
// NTFY_TEST_DATABASE_URL is set). newStore returns a store on the same table every time, like a
// second node or a restarted one.
func forEachBackend(t *testing.T, f func(t *testing.T, newStore func() *Store)) {
	t.Run("sqlite", func(t *testing.T) {
		d := newTestUserDB(t)
		f(t, func() *Store {
			return NewSQLiteStore(d)
		})
	})
	t.Run("postgres", func(t *testing.T) {
		d := dbtest.CreateTestPostgres(t)
		_, err := user.NewPostgresManager(d, &user.Config{}) // Creates the topic table
		require.Nil(t, err)
		f(t, func() *Store {
			return NewPostgresStore(d)
		})
	})
}

// newTestUserDB returns a fresh SQLite user database, which holds the topic table
func newTestUserDB(t testing.TB) *db.DB {
	d, err := user.OpenSQLite(filepath.Join(t.TempDir(), "user.db"))
	require.Nil(t, err)
	manager, err := user.NewSQLiteManager(d, "", &user.Config{})
	require.Nil(t, err)
	t.Cleanup(func() {
		manager.Close()
	})
	return d
}

// readAll returns the rows written at or after since, by topic
func readAll(t *testing.T, s *Store, since int64) map[string]*State {
	t.Helper()
	rows := make(map[string]*State)
	require.Nil(t, s.readSince(context.Background(), since, func(r *State) {
		c := *r // readSince reuses r for every row
		rows[r.Topic] = &c
	}))
	return rows
}

// topicsOf returns the map's topics, sorted
func topicsOf(rows map[string]*State) []string {
	topics := make([]string, 0, len(rows))
	for topic := range rows {
		topics = append(topics, topic)
	}
	slices.Sort(topics)
	return topics
}

func TestStore_Write_MergeRules(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		write := func(r *State, now int64) {
			require.Nil(t, s.write(context.Background(), []*State{r}, now))
		}
		current := func() *State {
			return readAll(t, s, 0)["up1"]
		}

		// A row without a rate visitor (a refused push) inserts as it is
		write(&State{Topic: "up1", FirstSeenAt: 100}, 101)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 100, UpdatedAt: 101}, current())

		// A subscribe sets the rate visitor and the subscriber activity; first_seen_at keeps the earliest
		write(&State{Topic: "up1", FirstSeenAt: 200, SubscribedAt: 200, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4"}, 201)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 100, SubscribedAt: 200, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4", UpdatedAt: 201}, current())

		// A row without a rate visitor keeps the stored one, and an older subscribed_at does not move it back
		write(&State{Topic: "up1", FirstSeenAt: 300, SubscribedAt: 150}, 301)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 100, SubscribedAt: 200, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4", UpdatedAt: 301}, current())

		// A later subscribed_at moves it forward
		write(&State{Topic: "up1", FirstSeenAt: 300, SubscribedAt: 250}, 302)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 100, SubscribedAt: 250, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4", UpdatedAt: 302}, current())

		// The last written rate visitor wins, whatever its subscribed_at; an earlier first_seen_at moves it back
		write(&State{Topic: "up1", FirstSeenAt: 50, SubscribedAt: 240, RateVisitorIP: "5.6.7.8"}, 303)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 50, SubscribedAt: 250, RateVisitorIP: "5.6.7.8", UpdatedAt: 303}, current())
	})
}

func TestStore_Write_Chunked(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		rows := make([]*State, 0)
		for i := 0; i < writeChunkRows+5; i++ {
			rows = append(rows, &State{Topic: fmt.Sprintf("up%012d", i), FirstSeenAt: 100, SubscribedAt: 100, RateVisitorIP: "1.2.3.4"})
		}
		require.Nil(t, s.write(context.Background(), rows, 100))
		require.Len(t, readAll(t, s, 0), writeChunkRows+5)
	})
}

func TestStore_ReadSince(t *testing.T) {
	// The read-back is by write time, not by the topic's own times
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		for i := 0; i < 5; i++ {
			require.Nil(t, s.write(context.Background(), []*State{{Topic: fmt.Sprintf("up%d", i), FirstSeenAt: 1000, SubscribedAt: 1000}}, int64(100+i)))
		}
		require.Equal(t, []string{"up0", "up1", "up2", "up3", "up4"}, topicsOf(readAll(t, s, 0)))
		require.Equal(t, []string{"up3", "up4"}, topicsOf(readAll(t, s, 103)))
		require.Empty(t, readAll(t, s, 105))
	})
}

func TestStore_ReadOne(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		require.Nil(t, s.write(context.Background(), []*State{{Topic: "up1", FirstSeenAt: 100, SubscribedAt: 200, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4"}}, 201))
		r, err := s.readOne(context.Background(), "up1")
		require.Nil(t, err)
		require.Equal(t, &State{Topic: "up1", FirstSeenAt: 100, SubscribedAt: 200, RateVisitorUserID: "u_1", RateVisitorIP: "1.2.3.4", UpdatedAt: 201}, r)
		r, err = s.readOne(context.Background(), "up2")
		require.Nil(t, err)
		require.Nil(t, r)
	})
}

func TestStore_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		for i := 0; i < 5; i++ {
			require.Nil(t, s.write(context.Background(), []*State{{Topic: fmt.Sprintf("up%d", i), FirstSeenAt: 1}}, int64(100+i)))
		}
		deleted, err := s.prune(context.Background(), 103)
		require.Nil(t, err)
		require.Equal(t, int64(3), deleted)
		require.Equal(t, []string{"up3", "up4"}, topicsOf(readAll(t, s, 0)))
	})
}
