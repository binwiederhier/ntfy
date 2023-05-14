package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccess_AllowReset(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	defer s.closeDatabases()

	// User and admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))

	// Subscribing not allowed
	rr := request(t, s, "GET", "/gold/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 403, rr.Code)

	// Grant access
	rr = request(t, s, "POST", "/v1/access", `{"username": "ben", "topic":"gold", "permission":"ro"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Now subscribing is allowed
	rr = request(t, s, "GET", "/gold/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, rr.Code)

	// Reset access
	rr = request(t, s, "DELETE", "/v1/access", `{"username": "ben", "topic":"gold"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Subscribing not allowed (again)
	rr = request(t, s, "GET", "/gold/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 403, rr.Code)
}

func TestAccess_AllowReset_NonAdminAttempt(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	defer s.closeDatabases()

	// User
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))

	// Grant access fails, because non-admin
	rr := request(t, s, "POST", "/v1/access", `{"username": "ben", "topic":"gold", "permission":"ro"}`, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccess_AllowReset_KillConnection(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	defer s.closeDatabases()

	// User and admin, grant access to "gol*" topics
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "gol*", user.PermissionRead)) // Wildcard!

	start, timeTaken := time.Now(), atomic.Int64{}
	go func() {
		rr := request(t, s, "GET", "/gold/json", "", map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)
		timeTaken.Store(time.Since(start).Milliseconds())
	}()
	time.Sleep(500 * time.Millisecond)

	// Reset access
	rr := request(t, s, "DELETE", "/v1/access", `{"username": "ben", "topic":"gol*"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Wait for connection to be killed; this will fail if the connection is never killed
	waitFor(t, func() bool {
		return timeTaken.Load() >= 500
	})
}
