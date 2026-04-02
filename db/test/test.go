package dbtest

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/util"
)

const testPoolMaxConns = "2"

// CreateTestPostgresSchema creates a temporary PostgreSQL schema and returns the DSN pointing to it.
// It registers a cleanup function to drop the schema when the test finishes.
// If NTFY_TEST_DATABASE_URL is not set, the test is skipped.
func CreateTestPostgresSchema(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("NTFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	schema := fmt.Sprintf("test_%s", util.RandomString(10))
	u, err := url.Parse(dsn)
	require.Nil(t, err)
	q := u.Query()
	q.Set("pool_max_conns", testPoolMaxConns)
	u.RawQuery = q.Encode()
	dsn = u.String()
	setupHost, err := pg.Open(dsn)
	require.Nil(t, err)
	_, err = setupHost.DB.Exec(fmt.Sprintf("CREATE SCHEMA %s", schema))
	require.Nil(t, err)
	require.Nil(t, setupHost.DB.Close())
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	schemaDSN := u.String()
	t.Cleanup(func() {
		cleanHost, err := pg.Open(dsn)
		if err == nil {
			cleanHost.DB.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
			cleanHost.DB.Close()
		}
	})
	return schemaDSN
}

// CreateTestPostgres creates a temporary PostgreSQL schema and returns an open *db.DB connection to it.
// It registers cleanup functions to close the DB and drop the schema when the test finishes.
// If NTFY_TEST_DATABASE_URL is not set, the test is skipped.
func CreateTestPostgres(t *testing.T) *db.DB {
	t.Helper()
	schemaDSN := CreateTestPostgresSchema(t)
	testHost, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	d := db.New(testHost, nil)
	t.Cleanup(func() {
		d.Close()
	})
	return d
}
