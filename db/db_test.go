package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
)

func TestDB_ExecContextAndQueryContext(t *testing.T) {
	sqlite, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.Nil(t, err)
	d := db.New(&db.Host{DB: sqlite}, nil)
	defer d.Close()

	ctx := context.Background()
	_, err = d.ExecContext(ctx, "CREATE TABLE things (id INT)")
	require.Nil(t, err)
	_, err = d.ExecContext(ctx, "INSERT INTO things VALUES (1)")
	require.Nil(t, err)
	rows, err := d.QueryContext(ctx, "SELECT id FROM things")
	require.Nil(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var id int
	require.Nil(t, rows.Scan(&id))
	require.Equal(t, 1, id)

	// The context is passed through, so a cancelled one is honored
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = d.ExecContext(cancelled, "INSERT INTO things VALUES (2)")
	require.ErrorIs(t, err, context.Canceled)
	_, err = d.QueryContext(cancelled, "SELECT id FROM things")
	require.ErrorIs(t, err, context.Canceled)
}

func TestDB_Stats(t *testing.T) {
	// One entry per pool, primary first, so the server can export each pool's connection counts
	primary, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "primary.db"))
	require.Nil(t, err)
	primary.SetMaxOpenConns(7)
	replica, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "replica.db"))
	require.Nil(t, err)
	replica.SetMaxOpenConns(3)
	d := db.New(&db.Host{
		Addr: "primary:5432",
		DB:   primary,
	}, []*db.Host{
		{
			Addr: "replica:5432",
			DB:   replica,
		},
	})
	defer d.Close()

	// Hold one primary connection, so it shows as in use
	conn, err := primary.Conn(context.Background())
	require.Nil(t, err)
	defer conn.Close()

	stats := d.Stats()
	require.Len(t, stats, 2)
	require.Equal(t, db.PoolRolePrimary, stats[0].Role)
	require.Equal(t, "primary:5432", stats[0].Addr)
	require.Equal(t, 7, stats[0].Stats.MaxOpenConnections)
	require.Equal(t, 1, stats[0].Stats.InUse)
	require.Equal(t, db.PoolRoleReplica, stats[1].Role)
	require.Equal(t, "replica:5432", stats[1].Addr)
	require.Equal(t, 3, stats[1].Stats.MaxOpenConnections)
	require.Equal(t, 0, stats[1].Stats.InUse)
}
