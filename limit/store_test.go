package limit

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
)

func forEachBackend(t *testing.T, f func(t *testing.T, newStore func() *Store)) {
	t.Run("sqlite", func(t *testing.T) {
		d := newTestUserDB(t)
		f(t, func() *Store {
			return NewSQLiteStore(d)
		})
	})
	t.Run("postgres", func(t *testing.T) {
		testDB := dbtest.CreateTestPostgres(t)
		_, err := user.NewPostgresManager(testDB, &user.Config{}) // Creates the usage table
		require.Nil(t, err)
		f(t, func() *Store {
			return NewPostgresStore(testDB)
		})
	})
}

// newTestUserDB returns a fresh SQLite user database, which holds the usage table
func newTestUserDB(t testing.TB) *db.DB {
	manager, err := user.NewSQLiteManager(filepath.Join(t.TempDir(), "user.db"), "", &user.Config{})
	require.Nil(t, err)
	t.Cleanup(func() {
		manager.Close()
	})
	return manager.DB()
}

// rowsByKeyKind maps "key/kind" to the count of the rows of the given period
func rowsByKeyKind(rows []usageRow, period int64) map[string]int64 {
	m := make(map[string]int64)
	for _, r := range rows {
		if r.period == period {
			m[r.key+"/"+r.kind.String()] = r.count
		}
	}
	return m
}

func TestStore_FlushAddsUp_ReadKeys(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		written, err := s.flush(context.Background(), []usageRow{
			{
				key:    "ip:1.2.3.4",
				period: 20000,
				kind:   KindMessages,
				count:  3,
			},
			{
				key:    "ip:1.2.3.4",
				period: 20000,
				kind:   KindEmails,
				count:  1,
			},
			{
				key:    "user:u_1",
				period: 20001,
				kind:   KindMessages,
				count:  10,
			},
		})
		require.Nil(t, err)
		require.Equal(t, 3, written)
		written, err = s.flush(context.Background(), []usageRow{
			{
				key:    "ip:1.2.3.4",
				period: 20000,
				kind:   KindMessages,
				count:  2,
			}, // Adds up, not overwrites
			{
				key:    "ip:1.2.3.4",
				period: 20001,
				kind:   KindMessages,
				count:  7,
			}, // Another period is another row
			{
				key:    "user:u_1",
				period: 20001,
				kind:   KindCalls,
				count:  1,
			},
		})
		require.Nil(t, err)
		require.Equal(t, 3, written)
		rows, err := s.readKeys(context.Background(), []string{"ip:1.2.3.4", "user:u_1", "ip:9.9.9.9"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{
			"ip:1.2.3.4/messages": 5,
			"ip:1.2.3.4/emails":   1,
		}, rowsByKeyKind(rows, 20000))
		require.Equal(t, map[string]int64{
			"ip:1.2.3.4/messages": 7,
			"user:u_1/messages":   10,
			"user:u_1/calls":      1,
		}, rowsByKeyKind(rows, 20001))
		rows, err = s.readKeys(context.Background(), nil)
		require.Nil(t, err)
		require.Empty(t, rows)
	})
}

func TestStore_FlushManyRows_Chunked(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		rows := make([]usageRow, 0, 2*flushChunkRows+7)
		keys := make([]string, 0, cap(rows))
		for i := 0; i < cap(rows); i++ {
			rows = append(rows, usageRow{
				key:    fmt.Sprintf("ip:10.0.%d.%d", i/256, i%256),
				period: 20000,
				kind:   KindMessages,
				count:  int64(i),
			})
			keys = append(keys, rows[i].key)
		}
		written, err := s.flush(context.Background(), rows)
		require.Nil(t, err)
		require.Equal(t, len(rows), written)
		read, err := s.readKeys(context.Background(), keys)
		require.Nil(t, err)
		require.Len(t, read, len(rows)) // A delta of 0 still writes a row
		m := rowsByKeyKind(read, 20000)
		require.Equal(t, int64(1), m["ip:10.0.0.1/messages"])
		require.Equal(t, int64(len(rows)-1), m[rows[len(rows)-1].key+"/messages"])
	})
}

func TestStore_ReadSince_UsesDatabaseClock(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		before, err := s.clock(context.Background())
		require.Nil(t, err)
		require.InDelta(t, time.Now().Unix(), before, 5) // The test database's clock is this host's
		_, err = s.flush(context.Background(), []usageRow{
			{
				key:    "ip:1.2.3.4",
				period: 20000,
				kind:   KindMessages,
				count:  3,
			},
			{
				key:    "user:u_1",
				period: 20001,
				kind:   KindMessages,
				count:  4,
			},
		})
		require.Nil(t, err)
		rows, err := s.readSince(context.Background(), before)
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"ip:1.2.3.4/messages": 3}, rowsByKeyKind(rows, 20000))
		require.Equal(t, map[string]int64{"user:u_1/messages": 4}, rowsByKeyKind(rows, 20001)) // Every key's own period
		rows, err = s.readSince(context.Background(), before+10)
		require.Nil(t, err)
		require.Empty(t, rows)
	})
}

func TestStore_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		// Rows are pruned by when they were last written, on the database clock: a row not
		// written for longer than a period is never the current one of its key
		s := newStore()
		for _, key := range []string{"ip:1.1.1.1", "ip:2.2.2.2", "ip:3.3.3.3"} {
			_, err := s.flush(context.Background(), []usageRow{{
				key:    key,
				period: 20000,
				kind:   KindMessages,
				count:  1,
			}})
			require.Nil(t, err)
		}
		_, err := s.db.Exec(`UPDATE usage SET updated_at = updated_at - 50 * 3600 WHERE key = 'ip:1.1.1.1'`)
		require.Nil(t, err)
		_, err = s.db.Exec(`UPDATE usage SET updated_at = updated_at - 30 * 3600 WHERE key = 'ip:2.2.2.2'`)
		require.Nil(t, err)
		require.Nil(t, s.prune(context.Background(), 48*time.Hour))
		rows, err := s.readKeys(context.Background(), []string{"ip:1.1.1.1", "ip:2.2.2.2", "ip:3.3.3.3"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{
			"ip:2.2.2.2/messages": 1,
			"ip:3.3.3.3/messages": 1,
		}, rowsByKeyKind(rows, 20000))
	})
}
