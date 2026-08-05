package apns_test

import (
	"net/netip"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/apns"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func forEachAPNSBackend(t *testing.T, f func(t *testing.T, store *apns.Store)) {
	t.Run("sqlite", func(t *testing.T) {
		store, err := apns.NewSQLiteStore(filepath.Join(t.TempDir(), "apns.db"), "")
		require.Nil(t, err)
		f(t, store)
	})
	t.Run("postgres", func(t *testing.T) {
		testDB := dbtest.CreateTestPostgres(t)
		store, err := apns.NewPostgresStore(testDB)
		require.Nil(t, err)
		f(t, store)
	})
}

func TestStoreRegisterGetTokensUnregister(t *testing.T) {
	forEachAPNSBackend(t, func(t *testing.T, store *apns.Store) {
		token := "apns-test-device-token-12345"
		topic := "my-alerts"
		userID := "user-123"
		ip := netip.MustParseAddr("1.2.3.4")

		// 1. Register token
		err := store.Register(token, topic, userID, ip)
		require.Nil(t, err)

		// 2. Query tokens
		tokens, err := store.GetTokens(topic)
		require.Nil(t, err)
		require.Len(t, tokens, 1)
		require.Equal(t, token, tokens[0])

		// Query different topic
		tokens2, err := store.GetTokens("other-alerts")
		require.Nil(t, err)
		require.Len(t, tokens2, 0)

		// 3. Update registration
		err = store.Register(token, topic, "new-user-456", netip.MustParseAddr("5.6.7.8"))
		require.Nil(t, err)

		tokens3, err := store.GetTokens(topic)
		require.Nil(t, err)
		require.Len(t, tokens3, 1)
		require.Equal(t, token, tokens3[0])

		// 4. Unregister token
		err = store.Unregister(token, topic)
		require.Nil(t, err)

		tokens4, err := store.GetTokens(topic)
		require.Nil(t, err)
		require.Len(t, tokens4, 0)
	})
}
