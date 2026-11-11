package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

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

func TestDB_HealthCheck(t *testing.T) {
	// The primary is checked like the replicas, so a node cut off from it shows as unhealthy
	// (ntfy_db_up), and healthy again once it is back
	path := filepath.Join(t.TempDir(), "primary.db")
	primary, err := sql.Open("sqlite3", path)
	require.Nil(t, err)
	d := db.New(&db.Host{
		Addr: "primary:5432",
		DB:   primary,
	}, nil)
	defer d.Close()
	require.True(t, d.Stats()[0].Healthy) // Just opened: healthy until a check says otherwise
	require.Nil(t, primary.Close())
	require.Eventually(t, func() bool {
		return !d.Stats()[0].Healthy
	}, 15*time.Second, 50*time.Millisecond)
}

func TestDB_ReadOnly_PrefersLeastBusyReplica(t *testing.T) {
	// A replica that stalls piles up connections in use; new reads go to the other one
	primary := openSQLite(t, "primary.db", 5)
	busy := openSQLite(t, "busy.db", 5)
	idle := openSQLite(t, "idle.db", 5)
	d := db.New(&db.Host{DB: primary}, []*db.Host{{DB: busy}, {DB: idle}})
	defer d.Close()
	d.MarkReplicasHealthyForTest()

	// Hold two connections on the busy replica
	for i := 0; i < 2; i++ {
		conn, err := busy.Conn(context.Background())
		require.Nil(t, err)
		defer conn.Close()
	}
	for i := 0; i < 10; i++ {
		require.Same(t, idle, d.ReadOnly())
	}
}

func TestDB_ReadOnly_RoundRobinWhenEquallyBusy(t *testing.T) {
	primary := openSQLite(t, "primary.db", 5)
	r1 := openSQLite(t, "r1.db", 5)
	r2 := openSQLite(t, "r2.db", 5)
	d := db.New(&db.Host{DB: primary}, []*db.Host{{DB: r1}, {DB: r2}})
	defer d.Close()
	d.MarkReplicasHealthyForTest()

	seen := map[*sql.DB]int{}
	for i := 0; i < 10; i++ {
		seen[d.ReadOnly()]++
	}
	require.Equal(t, 5, seen[r1])
	require.Equal(t, 5, seen[r2])
}

func TestDB_ReadOnly_ReplicaWhenReplicaPoolsFull(t *testing.T) {
	// Full pools wait for a connection on the least busy replica; the primary, which takes every
	// write, does not take the reads of stalled replicas
	primary := openSQLite(t, "primary.db", 5)
	r1 := openSQLite(t, "r1.db", 1)
	r2 := openSQLite(t, "r2.db", 2)
	d := db.New(&db.Host{DB: primary}, []*db.Host{{DB: r1}, {DB: r2}})
	defer d.Close()
	d.MarkReplicasHealthyForTest()

	// Fill both pools, r1 with one connection, r2 with two
	held := []*sql.DB{r1, r2, r2}
	for _, replica := range held {
		conn, err := replica.Conn(context.Background())
		require.Nil(t, err)
		defer conn.Close()
	}
	for i := 0; i < 10; i++ {
		require.Same(t, r1, d.ReadOnly())
	}
}

func TestDB_Stats_PingDuration(t *testing.T) {
	// The health check records how long each database took to answer, for ntfy_db_ping_seconds
	primary := openSQLite(t, "primary.db", 5)
	replica := openSQLite(t, "replica.db", 5)
	d := db.New(&db.Host{DB: primary}, []*db.Host{{DB: replica}})
	defer d.Close()
	require.Eventually(t, func() bool {
		stats := d.Stats()
		return stats[0].PingDuration > 0 && stats[1].PingDuration > 0
	}, 15*time.Second, 50*time.Millisecond)
}

func openSQLite(t *testing.T, name string, maxOpen int) *sql.DB {
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), name))
	require.Nil(t, err)
	conn.SetMaxOpenConns(maxOpen)
	return conn
}
