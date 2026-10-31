package limit

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
)

// forEachBackend runs f against a fresh usage table on SQLite and on Postgres (if
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
		_, err := user.NewPostgresManager(d, &user.Config{}) // Creates the usage table
		require.Nil(t, err)
		f(t, func() *Store {
			return NewPostgresStore(d)
		})
	})
}

// newTestUserDB returns a fresh SQLite user database, which holds the usage table
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

// readAll returns the rows written at or after since, by key
func readAll(t *testing.T, s *Store, since int64) map[string]usageRow {
	t.Helper()
	rows := make(map[string]usageRow)
	require.Nil(t, s.readSince(context.Background(), since, func(r *usageRow) {
		rows[r.key] = *r
	}))
	return rows
}

func TestStore_WriteAddsSamePeriodReplacesNewer(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		ctx := context.Background()
		written, err := s.write(ctx, []usageRow{
			{
				key:         "user:u_1",
				periodStart: 1000,
				counts:      [numDaily]int64{10, 0, 1},
			},
			{
				key:         "ip:1.2.3.4",
				periodStart: 2000,
				counts:      [numDaily]int64{3, 1, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		require.Equal(t, 2, written)
		written, err = s.write(ctx, []usageRow{
			{
				key:         "ip:1.2.3.4",
				periodStart: 2000,
				counts:      [numDaily]int64{2, 0, 0},
			}, // Same period: adds up
			{
				key:         "user:u_1",
				periodStart: 1000 + daySeconds,
				counts:      [numDaily]int64{4, 0, 0},
			}, // Newer period: replaces
		}, time.Now().Unix())
		require.Nil(t, err)
		require.Equal(t, 2, written)
		rows := readAll(t, s, 0)
		require.Equal(t, usageRow{
			key:         "ip:1.2.3.4",
			periodStart: 2000,
			counts:      [numDaily]int64{5, 1, 0},
		}, rows["ip:1.2.3.4"])
		require.Equal(t, usageRow{
			key:         "user:u_1",
			periodStart: 1000 + daySeconds,
			counts:      [numDaily]int64{4, 0, 0},
		}, rows["user:u_1"])

		// A leftover of an older period lands in the stored, newer one instead of taking the row back
		_, err = s.write(ctx, []usageRow{
			{
				key:         "user:u_1",
				periodStart: 1000,
				counts:      [numDaily]int64{2, 0, 0},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		require.Equal(t, usageRow{
			key:         "user:u_1",
			periodStart: 1000 + daySeconds,
			counts:      [numDaily]int64{6, 0, 0},
		}, readAll(t, s, 0)["user:u_1"])
	})
}

func TestStore_WriteManyRows_Chunked(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		rows := make([]usageRow, 2500)
		for i := range rows {
			rows[i] = usageRow{
				key:         fmt.Sprintf("ip:10.0.%d.%d", i/256, i%256),
				periodStart: 1000,
				counts:      [numDaily]int64{int64(i), 0, 0},
			}
		}
		written, err := s.write(context.Background(), rows, time.Now().Unix())
		require.Nil(t, err)
		require.Equal(t, 2500, written)
		read := readAll(t, s, 0)
		require.Len(t, read, 2500)
		require.Equal(t, int64(2499), read["ip:10.0.9.195"].counts[0])
	})
}

func TestStore_ReadSince(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		ctx := context.Background()
		_, err := s.db.Exec(`INSERT INTO usage (key, period_start, messages, emails, calls, updated_at) VALUES ('old', 1, 1, 0, 0, 100), ('new', 1, 2, 0, 0, 200)`)
		require.Nil(t, err)
		require.Equal(t, []string{"new"}, keysOf(readAll(t, s, 200)))
		require.Equal(t, []string{"new", "old"}, keysOf(readAll(t, s, 100)))

		// A write stamps the given time
		dbNow := time.Now().Unix()
		_, err = s.write(ctx, []usageRow{
			{
				key:         "written",
				periodStart: 1,
				counts:      [numDaily]int64{0, 0, 1},
			},
		}, time.Now().Unix())
		require.Nil(t, err)
		require.Equal(t, []string{"written"}, keysOf(readAll(t, s, dbNow)))
	})
}

func TestStore_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		for i := 0; i < 25; i++ {
			_, err := s.db.Exec(fmt.Sprintf(`INSERT INTO usage (key, period_start, messages, emails, calls, updated_at) VALUES ('k%d', 1, 1, 0, 0, %d)`, i, 100+i))
			require.Nil(t, err)
		}
		deleted, err := s.prune(context.Background(), 120)
		require.Nil(t, err)
		require.Equal(t, int64(20), deleted)
		require.Equal(t, []string{"k20", "k21", "k22", "k23", "k24"}, keysOf(readAll(t, s, 0)))
	})
}

func TestStore_PruneKeepsRowRefreshedMeanwhile(t *testing.T) {
	// A row another node updates while the prune waits for its lock must survive: Postgres
	// rechecks the outer predicate on the updated row, so it must test the age, not only the key
	d := dbtest.CreateTestPostgres(t)
	_, err := user.NewPostgresManager(d, &user.Config{})
	require.Nil(t, err)
	d.Primary().SetMaxOpenConns(3)
	store := NewPostgresStore(d)
	_, err = d.Exec(`INSERT INTO usage (key, period_start, messages, emails, calls, updated_at) VALUES ('k', 1, 10, 0, 0, 1)`)
	require.Nil(t, err)
	tx, err := d.Begin()
	require.Nil(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE usage SET period_start = 200, messages = 100, updated_at = 200 WHERE key = 'k'`)
	require.Nil(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := store.prune(context.Background(), 100)
		done <- err
	}()
	require.Eventually(t, func() bool {
		var n int
		err := d.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query = $1`, postgresPruneQuery).Scan(&n)
		return err == nil && n > 0
	}, 2*time.Second, time.Millisecond)
	require.Nil(t, tx.Commit())
	require.Nil(t, <-done)
	require.Equal(t, int64(100), readAll(t, store, 0)["k"].counts[0])
}

// keysOf returns the map's keys, sorted
func keysOf(rows map[string]usageRow) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestStore_WriteRateVisitors_SetReplacesTouchKeeps(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		write := func(topic, key string, seenAt, now int64) {
			require.Nil(t, s.writeRateVisitors(context.Background(), []rateVisitorRow{newRateVisitorRow(topic, key, seenAt)}, now))
		}
		write("up1", "ip:1.2.3.4", 100, 101)
		write("up1", "", 200, 201) // A touch keeps the rate visitor
		require.Equal(t, newRateVisitorRow("up1", "ip:1.2.3.4", 200), readAllRateVisitors(t, s, 0)["up1"])
		write("up1", "", 150, 202) // An older touch does not move seen_at back
		require.Equal(t, newRateVisitorRow("up1", "ip:1.2.3.4", 200), readAllRateVisitors(t, s, 0)["up1"])
		write("up1", "user:u_1", 50, 203) // A set replaces both
		require.Equal(t, newRateVisitorRow("up1", "user:u_1", 50), readAllRateVisitors(t, s, 0)["up1"])
		write("up2", "", 300, 301) // A touch of an unknown topic: no rate visitor
		require.Equal(t, newRateVisitorRow("up2", "", 300), readAllRateVisitors(t, s, 0)["up2"])

		// The read-back is by write time
		require.Equal(t, []string{"up2"}, rateVisitorTopicsOf(readAllRateVisitors(t, s, 301)))
	})
}

func TestStore_WriteRateVisitors_Chunked(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		rows := make([]rateVisitorRow, 0)
		for i := 0; i < writeChunkRows+5; i++ {
			rows = append(rows, newRateVisitorRow(fmt.Sprintf("up%012d", i), "ip:1.2.3.4", 100))
		}
		require.Nil(t, s.writeRateVisitors(context.Background(), rows, 100))
		require.Len(t, readAllRateVisitors(t, s, 0), writeChunkRows+5)
	})
}

func TestStore_PruneRateVisitors(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		for i := 0; i < 5; i++ {
			require.Nil(t, s.writeRateVisitors(context.Background(), []rateVisitorRow{newRateVisitorRow(fmt.Sprintf("up%d", i), "", 1)}, int64(100+i)))
		}
		deleted, err := s.pruneRateVisitors(context.Background(), 103)
		require.Nil(t, err)
		require.Equal(t, int64(3), deleted)
		require.Equal(t, []string{"up3", "up4"}, rateVisitorTopicsOf(readAllRateVisitors(t, s, 0)))
	})
}

// readAllRateVisitors returns the rate visitor rows written at or after since, by topic
func readAllRateVisitors(t *testing.T, s *Store, since int64) map[string]rateVisitorRow {
	t.Helper()
	rows := make(map[string]rateVisitorRow)
	require.Nil(t, s.readRateVisitorsSince(context.Background(), since, func(r *rateVisitorRow) {
		rows[r.topic] = *r
	}))
	return rows
}

// rateVisitorTopicsOf returns the map's topics, sorted
func rateVisitorTopicsOf(rows map[string]rateVisitorRow) []string {
	topics := make([]string, 0, len(rows))
	for topic := range rows {
		topics = append(topics, topic)
	}
	slices.Sort(topics)
	return topics
}

// newRateVisitorRow returns a rate visitor row; a user key ("user:<id>") has its user ID
func newRateVisitorRow(topic, key string, seenAt int64) rateVisitorRow {
	userID, _ := strings.CutPrefix(key, "user:")
	if userID == key {
		userID = ""
	}
	return rateVisitorRow{
		topic:  topic,
		key:    key,
		userID: userID,
		seenAt: seenAt,
	}
}
