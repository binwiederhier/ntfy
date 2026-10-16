package pg

import (
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen_InvalidScheme(t *testing.T) {
	_, err := Open("postgresql+psycopg2://user:pass@localhost/db")
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid database URL scheme "postgresql+psycopg2"`)
	require.Contains(t, err.Error(), "*****")
	require.NotContains(t, err.Error(), "pass")
}

func TestOpen_InvalidURL(t *testing.T) {
	_, err := Open("not a valid url\x00")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid database URL")
}

func TestCensorPassword(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "with password",
			url:      "postgres://user:secret@localhost/db",
			expected: "postgres://user:*****@localhost/db",
		},
		{
			name:     "without password",
			url:      "postgres://localhost/db",
			expected: "postgres://localhost/db",
		},
		{
			name:     "user only",
			url:      "postgres://user@localhost/db",
			expected: "postgres://user@localhost/db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)
			require.Equal(t, tt.expected, censorPassword(u))
		})
	}
}

func TestOpen_KeepsPoolConnectionsIdle(t *testing.T) {
	// database/sql keeps only 2 idle connections by default, so every burst above that closed
	// and re-opened connections (DNS lookup + TCP + TLS + auth per query). Unless configured
	// otherwise, the pool now keeps as many idle connections as it may open.
	dsn := os.Getenv("NTFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(dsn)
	require.Nil(t, err)
	q := u.Query()
	q.Set("pool_max_conns", "8")
	u.RawQuery = q.Encode()
	host, err := Open(u.String())
	require.Nil(t, err)
	defer host.DB.Close()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := host.DB.Exec("SELECT pg_sleep(0.2)")
			require.Nil(t, err)
		}()
	}
	wg.Wait()
	require.Equal(t, 6, host.DB.Stats().Idle)
}
