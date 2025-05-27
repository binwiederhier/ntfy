package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"sync/atomic"
	"testing"
	"time"
)

func TestUser_AddRemove(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin, tier
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "tier1",
	}))

	// Create user via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Create user with tier via API
	rr = request(t, s, "PUT", "/v1/users", `{"username": "emma", "password":"emma", "tier": "tier1"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check users
	users, err := s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, 4, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "ben", users[1].Name)
	require.Equal(t, user.RoleUser, users[1].Role)
	require.Nil(t, users[1].Tier)
	require.Equal(t, "emma", users[2].Name)
	require.Equal(t, user.RoleUser, users[2].Role)
	require.Equal(t, "tier1", users[2].Tier.Code)
	require.Equal(t, user.Everyone, users[3].Name)

	// Delete user via API
	rr = request(t, s, "DELETE", "/v1/users", `{"username": "ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check user was deleted
	users, err = s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, 3, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "emma", users[1].Name)
	require.Equal(t, user.Everyone, users[2].Name)

	// Reject invalid user change
	rr = request(t, s, "PUT", "/v1/users", `{"username": "ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 400, rr.Code)
}

func TestUser_AddWithPasswordHash(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

	// Create user via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "hash":"$2a$04$2aPIIqPXQU16OfkSUZH1XOzpu1gsPRKkrfVdFLgWQ.tqb.vtTCuVe"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check that user can login with password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, rr.Code)

	// Check users
	users, err := s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, 3, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, user.RoleAdmin, users[0].Role)
	require.Equal(t, "ben", users[1].Name)
	require.Equal(t, user.RoleUser, users[1].Role)
}

func TestUser_ChangeUserPassword(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

	// Create user via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password": "ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Try to login with first password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, rr.Code)

	// Change password via API
	rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "password": "ben-two"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Make sure first password fails
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)

	// Try to login with second password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben-two"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestUser_ChangeUserTier(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin, tier
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "tier1",
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "tier2",
	}))

	// Create user with tier via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben", "tier": "tier1"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check users
	users, err := s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, 3, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "ben", users[1].Name)
	require.Equal(t, user.RoleUser, users[1].Role)
	require.Equal(t, "tier1", users[1].Tier.Code)

	// Change user tier via API
	rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier": "tier2"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check users again
	users, err = s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, "tier2", users[1].Tier.Code)
}

func TestUser_ChangeUserPasswordAndTier(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin, tier
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "tier1",
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "tier2",
	}))

	// Create user with tier via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben", "tier": "tier1"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Check users
	users, err := s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, 3, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "ben", users[1].Name)
	require.Equal(t, user.RoleUser, users[1].Role)
	require.Equal(t, "tier1", users[1].Tier.Code)

	// Change user password and tier via API
	rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "password":"ben-two", "tier": "tier2"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Make sure first password fails
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)

	// Try to login with second password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben-two"),
	})
	require.Equal(t, 200, rr.Code)

	// Check new tier
	users, err = s.userManager.Users()
	require.Nil(t, err)
	require.Equal(t, "tier2", users[1].Tier.Code)
}

func TestUser_ChangeUserPasswordWithHash(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

	// Create user with tier via API
	rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password":"not-ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Try to login with first password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "not-ben"),
	})
	require.Equal(t, 200, rr.Code)

	// Change user password and tier via API
	rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "hash":"$2a$04$2aPIIqPXQU16OfkSUZH1XOzpu1gsPRKkrfVdFLgWQ.tqb.vtTCuVe"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Try to login with second password
	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestUser_DontChangeAdminPassword(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddUser("admin", "admin", user.RoleAdmin, false))

	// Try to change password via API
	rr := request(t, s, "PUT", "/v1/users", `{"username": "admin", "password": "admin-new"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 403, rr.Code)
}

func TestUser_AddRemove_Failures(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	// Create admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

	// Cannot create user with invalid username
	rr := request(t, s, "POST", "/v1/users", `{"username": "not valid", "password":"ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 400, rr.Code)

	// Cannot create user if user already exists
	rr = request(t, s, "POST", "/v1/users", `{"username": "phil", "password":"phil"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 40901, toHTTPError(t, rr.Body.String()).Code)

	// Cannot create user with invalid tier
	rr = request(t, s, "POST", "/v1/users", `{"username": "emma", "password":"emma", "tier": "invalid"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 40030, toHTTPError(t, rr.Body.String()).Code)

	// Cannot delete user as non-admin
	rr = request(t, s, "DELETE", "/v1/users", `{"username": "ben"}`, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)

	// Delete user via API
	rr = request(t, s, "DELETE", "/v1/users", `{"username": "ben"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestAccess_AllowReset(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	defer s.closeDatabases()

	// User and admin
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

	// Subscribing not allowed
	rr := request(t, s, "GET", "/gold/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 403, rr.Code)

	// Grant access
	rr = request(t, s, "POST", "/v1/users/access", `{"username": "ben", "topic":"gold", "permission":"ro"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Now subscribing is allowed
	rr = request(t, s, "GET", "/gold/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, rr.Code)

	// Reset access
	rr = request(t, s, "DELETE", "/v1/users/access", `{"username": "ben", "topic":"gold"}`, map[string]string{
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
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

	// Grant access fails, because non-admin
	rr := request(t, s, "POST", "/v1/users/access", `{"username": "ben", "topic":"gold", "permission":"ro"}`, map[string]string{
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
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
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
	rr := request(t, s, "DELETE", "/v1/users/access", `{"username": "ben", "topic":"gol*"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Wait for connection to be killed; this will fail if the connection is never killed
	waitFor(t, func() bool {
		return timeTaken.Load() >= 500
	})
}
