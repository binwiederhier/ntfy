package pg_test

import (
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func TestOpen_KeepsPoolConnectionsIdle(t *testing.T) {
	// database/sql keeps only 2 idle connections by default, so every burst above that closed
	// and re-opened connections (DNS lookup + TCP + TLS + auth per query). Unless configured
	// otherwise, the pool now keeps as many idle connections as it may open.
	u, err := url.Parse(dbtest.CreateTestPostgresSchema(t))
	require.Nil(t, err)
	q := u.Query()
	q.Set("pool_max_conns", "8")
	u.RawQuery = q.Encode()
	host, err := pg.Open(u.String())
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
