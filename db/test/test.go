package dbtest

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

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

// CreateTestPostgresWithLatency is CreateTestPostgres, but every request the client sends reaches
// the database latency later, so each database round trip costs at least latency (as it does
// against a remote managed database). Use it to test how many round trips a code path makes.
func CreateTestPostgresWithLatency(t *testing.T, latency time.Duration) *db.DB {
	t.Helper()
	testHost, err := pg.Open(WithLatency(t, CreateTestPostgresSchema(t), latency))
	require.Nil(t, err)
	d := db.New(testHost, nil)
	t.Cleanup(func() {
		d.Close()
	})
	return d
}

// WithLatency starts a local TCP proxy to the database in dsn that delays everything the client
// sends by latency, and returns dsn pointing at the proxy. The proxy stops when the test ends.
func WithLatency(t *testing.T, dsn string, latency time.Duration) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.Nil(t, err)
	upstream := u.Host
	if u.Port() == "" {
		upstream = net.JoinHostPort(u.Hostname(), "5432")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	t.Cleanup(func() {
		listener.Close()
	})
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return // Listener closed
			}
			go proxyWithLatency(client, upstream, latency)
		}
	}()
	u.Host = listener.Addr().String()
	return u.String()
}

func proxyWithLatency(client net.Conn, upstream string, latency time.Duration) {
	defer client.Close()
	server, err := net.Dial("tcp", upstream)
	if err != nil {
		return
	}
	defer server.Close()
	go func() {
		io.Copy(client, server)
		client.Close()
	}()
	buf := make([]byte, 64*1024)
	for {
		n, err := client.Read(buf)
		if n > 0 {
			time.Sleep(latency)
			if _, err := server.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
