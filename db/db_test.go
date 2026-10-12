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
