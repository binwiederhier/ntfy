package ratevisitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func newTestStore(t *testing.T) *Store {
	host, err := pg.Open(dbtest.CreateTestPostgresSchema(t))
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	store, err := New(d)
	require.Nil(t, err)
	return store
}

func TestStore_SetGet(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.Get("up123456789012")
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, s.Set("up123456789012", "ip:1.2.3.4", "", time.Now().Add(time.Hour)))
	key, userID, err := s.Get("up123456789012")
	require.Nil(t, err)
	require.Equal(t, "ip:1.2.3.4", key)
	require.Equal(t, "", userID)

	// Re-registration replaces the assignment (e.g. the subscriber moved or logged in)
	require.Nil(t, s.Set("up123456789012", "user:u_abc", "u_abc", time.Now().Add(time.Hour)))
	key, userID, err = s.Get("up123456789012")
	require.Nil(t, err)
	require.Equal(t, "user:u_abc", key)
	require.Equal(t, "u_abc", userID)
}

func TestStore_ExpiryAndPrune(t *testing.T) {
	s := newTestStore(t)
	require.Nil(t, s.Set("up123456789012", "ip:1.2.3.4", "", time.Now().Add(-time.Minute)))
	_, _, err := s.Get("up123456789012")
	require.ErrorIs(t, err, ErrNotFound) // Expired rows are invisible even before pruning
	require.Nil(t, s.Prune())
	var count int
	require.Nil(t, s.pool.QueryRow(`SELECT COUNT(*) FROM topic_rate_visitor`).Scan(&count))
	require.Equal(t, 0, count)
}
