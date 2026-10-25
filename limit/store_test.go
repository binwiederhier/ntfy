package limit

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func forEachBackend(t *testing.T, f func(t *testing.T, newStore func() *Store)) {
	t.Run("sqlite", func(t *testing.T) {
		filename := filepath.Join(t.TempDir(), "usage.db")
		f(t, func() *Store {
			s, err := NewSQLiteStore(filename)
			require.Nil(t, err)
			t.Cleanup(func() { s.Close() })
			return s
		})
	})
	t.Run("postgres", func(t *testing.T) {
		testDB := dbtest.CreateTestPostgres(t)
		f(t, func() *Store {
			s, err := NewPostgresStore(testDB)
			require.Nil(t, err)
			return s
		})
	})
}

func rowsByKeyKind(rows []usageRow) map[string]int64 {
	m := make(map[string]int64)
	for _, r := range rows {
		m[r.key+"/"+r.kind.String()] = r.count
	}
	return m
}

func TestStore_FlushAddsUp_ReadKeys(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		written, err := s.flush(context.Background(), "2026-10-25", []usageRow{
			{key: "ip:1.2.3.4", kind: KindMessages, count: 3},
			{key: "ip:1.2.3.4", kind: KindEmails, count: 1},
			{key: "user:u_1", kind: KindMessages, count: 10},
		})
		require.Nil(t, err)
		require.Equal(t, 3, written)
		written, err = s.flush(context.Background(), "2026-10-25", []usageRow{
			{key: "ip:1.2.3.4", kind: KindMessages, count: 2}, // Adds up, not overwrites
			{key: "user:u_1", kind: KindCalls, count: 1},
		})
		require.Nil(t, err)
		require.Equal(t, 2, written)
		rows, err := s.readKeys(context.Background(), "2026-10-25", []string{"ip:1.2.3.4", "user:u_1", "ip:9.9.9.9"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{
			"ip:1.2.3.4/messages": 5,
			"ip:1.2.3.4/emails":   1,
			"user:u_1/messages":   10,
			"user:u_1/calls":      1,
		}, rowsByKeyKind(rows))

		// Another day is another row
		rows, err = s.readKeys(context.Background(), "2026-10-26", []string{"ip:1.2.3.4"})
		require.Nil(t, err)
		require.Empty(t, rows)
		rows, err = s.readKeys(context.Background(), "2026-10-25", nil)
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
			rows = append(rows, usageRow{key: fmt.Sprintf("ip:10.0.%d.%d", i/256, i%256), kind: KindMessages, count: int64(i)})
			keys = append(keys, rows[i].key)
		}
		written, err := s.flush(context.Background(), "2026-10-25", rows)
		require.Nil(t, err)
		require.Equal(t, len(rows), written)
		read, err := s.readKeys(context.Background(), "2026-10-25", keys)
		require.Nil(t, err)
		require.Len(t, read, len(rows)) // A delta of 0 still writes a row
		m := rowsByKeyKind(read)
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
		_, err = s.flush(context.Background(), "2026-10-25", []usageRow{{key: "ip:1.2.3.4", kind: KindMessages, count: 3}})
		require.Nil(t, err)
		rows, err := s.readSince(context.Background(), "2026-10-25", before)
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"ip:1.2.3.4/messages": 3}, rowsByKeyKind(rows))
		rows, err = s.readSince(context.Background(), "2026-10-25", before+10)
		require.Nil(t, err)
		require.Empty(t, rows)
		rows, err = s.readSince(context.Background(), "2026-10-24", before)
		require.Nil(t, err)
		require.Empty(t, rows)
	})
}

func TestStore_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		for _, day := range []string{"2026-10-17", "2026-10-18", "2026-10-25"} {
			_, err := s.flush(context.Background(), day, []usageRow{{key: "ip:1.2.3.4", kind: KindMessages, count: 1}})
			require.Nil(t, err)
		}
		require.Nil(t, s.prune(context.Background(), "2026-10-18"))
		for day, want := range map[string]int{"2026-10-17": 0, "2026-10-18": 1, "2026-10-25": 1} {
			rows, err := s.readKeys(context.Background(), day, []string{"ip:1.2.3.4"})
			require.Nil(t, err)
			require.Len(t, rows, want, day)
		}
	})
}

func TestStore_Reopen_KeepsRows(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		s := newStore()
		_, err := s.flush(context.Background(), "2026-10-25", []usageRow{{key: "user:u_1", kind: KindMessages, count: 7}})
		require.Nil(t, err)
		require.Nil(t, s.Close())
		s = newStore() // Same file or schema: the migration must be idempotent, the rows kept
		rows, err := s.readKeys(context.Background(), "2026-10-25", []string{"user:u_1"})
		require.Nil(t, err)
		require.Equal(t, map[string]int64{"user:u_1/messages": 7}, rowsByKeyKind(rows))
	})
}

func TestStore_MigrateFrom1To2_AddsTopicTable(t *testing.T) {
	// A usage database of the first version (the usage table only) gains the topic table, and
	// ends up with the same schema as a fresh one
	const usageV1 = `
		CREATE TABLE usage (key TEXT NOT NULL, day TEXT NOT NULL, kind TEXT NOT NULL, count %[1]s NOT NULL DEFAULT 0, updated_at %[1]s NOT NULL, PRIMARY KEY (key, day, kind));
		CREATE INDEX idx_usage_day_updated_at ON usage (day, updated_at);
	`
	t.Run("sqlite", func(t *testing.T) {
		dir := t.TempDir()
		old, err := sql.Open("sqlite3", filepath.Join(dir, "old.db"))
		require.Nil(t, err)
		_, err = old.Exec(fmt.Sprintf(usageV1, "INT") + `CREATE TABLE schemaVersion (id INT PRIMARY KEY, version INT NOT NULL, min_version INT NOT NULL DEFAULT 1); INSERT INTO schemaVersion VALUES (1, 1, 1);`)
		require.Nil(t, err)
		require.Nil(t, old.Close())
		migrated, err := NewSQLiteStore(filepath.Join(dir, "old.db"))
		require.Nil(t, err)
		defer migrated.Close()
		fresh, err := NewSQLiteStore(filepath.Join(dir, "fresh.db"))
		require.Nil(t, err)
		defer fresh.Close()
		require.Equal(t, dbtest.SQLiteSchema(t, fresh.db), dbtest.SQLiteSchema(t, migrated.db))
		require.Contains(t, dbtest.SQLiteSchema(t, migrated.db), "table topic")
	})
	t.Run("postgres", func(t *testing.T) {
		old := dbtest.CreateTestPostgres(t)
		_, err := old.Exec(fmt.Sprintf(usageV1, "BIGINT") + `CREATE TABLE schema_version (store TEXT PRIMARY KEY, version INT NOT NULL, min_version INT NOT NULL DEFAULT 1); INSERT INTO schema_version VALUES ('usage', 1, 1);`)
		require.Nil(t, err)
		_, err = NewPostgresStore(old)
		require.Nil(t, err)
		fresh := dbtest.CreateTestPostgres(t)
		_, err = NewPostgresStore(fresh)
		require.Nil(t, err)
		require.Equal(t, dbtest.PostgresSchema(t, fresh), dbtest.PostgresSchema(t, old))
		require.Contains(t, dbtest.PostgresSchema(t, old), "table topic")
	})
}
