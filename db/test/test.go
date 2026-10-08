package dbtest

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync/atomic"
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
func CreateTestPostgresSchema(t testing.TB) string {
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
func CreateTestPostgres(t testing.TB) *db.DB {
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
// against a remote managed database). The returned proxy counts the requests, so a test can
// assert how many round trips a code path makes without depending on the speed of the test host.
func CreateTestPostgresWithLatency(t *testing.T, latency time.Duration) (*db.DB, *LatencyProxy) {
	t.Helper()
	proxy := NewLatencyProxy(t, CreateTestPostgresSchema(t), latency)
	testHost, err := pg.Open(proxy.DSN())
	require.Nil(t, err)
	d := db.New(testHost, nil)
	t.Cleanup(func() {
		d.Close()
	})
	return d, proxy
}

// WithLatency starts a LatencyProxy to the database in dsn and returns the DSN pointing at it
func WithLatency(t *testing.T, dsn string, latency time.Duration) string {
	t.Helper()
	return NewLatencyProxy(t, dsn, latency).DSN()
}

// LatencyProxy is a local TCP proxy to a database that delays everything the client sends by a
// fixed latency and counts the client's requests (a request is one client write, which for the
// pgx driver is one round trip: one query, or one batch of pipelined protocol messages).
type LatencyProxy struct {
	dsn      string
	latency  time.Duration
	requests atomic.Int64
}

// NewLatencyProxy starts a LatencyProxy to the database in dsn; it stops when the test ends
func NewLatencyProxy(t *testing.T, dsn string, latency time.Duration) *LatencyProxy {
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
	p := &LatencyProxy{latency: latency}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return // Listener closed
			}
			go p.proxy(client, upstream)
		}
	}()
	u.Host = listener.Addr().String()
	p.dsn = u.String()
	return p
}

// DSN returns the database URL pointing at the proxy
func (p *LatencyProxy) DSN() string {
	return p.dsn
}

// Requests returns the number of client requests forwarded so far
func (p *LatencyProxy) Requests() int64 {
	return p.requests.Load()
}

func (p *LatencyProxy) proxy(client net.Conn, upstream string) {
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
			p.requests.Add(1)
			time.Sleep(p.latency)
			if _, err := server.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
