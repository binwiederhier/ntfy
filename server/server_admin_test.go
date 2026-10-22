package server

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVersion_Admin(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.BuildVersion = "1.2.3"
		c.BuildCommit = "abcdef0"
		c.BuildDate = "2026-02-08T00:00:00Z"
		s := newTestServer(t, c)
		defer s.closeDatabases()

		// Create admin and regular user
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

		// Admin can access /v1/version
		rr := request(t, s, "GET", "/v1/version", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)

		var versionResponse apiVersionResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&versionResponse))
		require.Equal(t, "1.2.3", versionResponse.Version)
		require.Equal(t, "abcdef0", versionResponse.Commit)
		require.Equal(t, "2026-02-08T00:00:00Z", versionResponse.Date)

		// Non-admin user cannot access /v1/version
		rr = request(t, s, "GET", "/v1/version", "", map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 401, rr.Code)

		// Unauthenticated user cannot access /v1/version
		rr = request(t, s, "GET", "/v1/version", "", nil)
		require.Equal(t, 401, rr.Code)
	})
}

func TestUser_AddRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_AddWithPasswordHash(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_ChangeUserPassword(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_ChangeUserTier(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_ChangeUserPasswordAndTier(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_ChangeUserPasswordWithHash(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_DontChangeAdminPassword(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()

		// Create admin
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("admin", "admin", user.RoleAdmin, false))

		// Try to change password via API
		rr := request(t, s, "PUT", "/v1/users", `{"username": "admin", "password": "admin-new"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 403, rr.Code)
	})
}

func TestUser_AddRemove_Failures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
	})
}

func TestUser_ChangeRemoveProvisionedUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		conf := newTestConfigWithAuthFile(t, databaseURL)
		conf.AuthUsers = []*user.User{
			{Name: "philuser", Hash: "$2a$10$U4WSIYY6evyGmZaraavM2e2JeVG6EMGUKN1uUwufUeeRd4Jpg6cGC", Role: user.RoleUser}, // philuser:philpass
		}
		s := newTestServer(t, conf)
		defer s.closeDatabases()

		// Create admin
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

		// Cannot delete provisioned user
		rr := request(t, s, "DELETE", "/v1/users", `{"username": "philuser"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40905, toHTTPError(t, rr.Body.String()).Code)

		// Cannot change password of provisioned user
		rr = request(t, s, "PUT", "/v1/users", `{"username": "philuser", "password": "newpass"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40905, toHTTPError(t, rr.Body.String()).Code)

		// User is still there, with the old password
		u, err := s.userManager.Authenticate("philuser", "philpass")
		require.Nil(t, err)
		require.Equal(t, "philuser", u.Name)
	})
}

func TestAccess_AllowReset(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
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
	})
}

func TestAccess_AllowReset_NonAdminAttempt(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
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
	})
}

func TestAccess_AllowReset_KillConnection(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
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
	})
}

func TestUser_GetWithDetails(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		conf := newTestConfigWithAuthFile(t, databaseURL)
		conf.AuthDefault = user.PermissionRead
		conf.AuthUsers = []*user.User{
			{Name: "philuser", Hash: "$2a$10$U4WSIYY6evyGmZaraavM2e2JeVG6EMGUKN1uUwufUeeRd4Jpg6cGC", Role: user.RoleUser}, // philuser:philpass
		}
		conf.AuthAccess = map[string][]*user.Grant{
			"philuser": {{TopicPattern: "provisioned-topic", Permission: user.PermissionReadWrite}},
		}
		s := newTestServer(t, conf)
		defer s.closeDatabases()

		// Admin, a user with a tier and a manual grant
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", Name: "Pro"}))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))
		require.Nil(t, s.userManager.AllowAccess("ben", "bens-topic", user.PermissionRead))

		rr := request(t, s, "GET", "/v1/users", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
		var users []*apiUserResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&users))
		byName := make(map[string]*apiUserResponse)
		for _, u := range users {
			byName[u.Username] = u
		}

		// Tier code and display name
		require.Equal(t, "pro", byName["ben"].Tier)
		require.Equal(t, "Pro", byName["ben"].TierName)
		require.False(t, byName["ben"].Provisioned)
		require.Equal(t, []*apiUserGrantResponse{{Topic: "bens-topic", Permission: "read-only"}}, byName["ben"].Grants)

		// Provisioned user and grant
		require.True(t, byName["philuser"].Provisioned)
		require.Equal(t, []*apiUserGrantResponse{{Topic: "provisioned-topic", Permission: "read-write", Provisioned: true}}, byName["philuser"].Grants)

		// Default access only on the everyone entry
		require.Equal(t, "read-only", byName[user.Everyone].DefaultAccess)
		require.Equal(t, "", byName["ben"].DefaultAccess)
		require.Equal(t, "", byName["phil"].DefaultAccess)
	})
}

func TestUser_AddWithRole(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

		// Role over HTTP is refused, even for admins
		rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben", "role":"admin"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 403, rr.Code)
		require.Equal(t, 40302, toHTTPError(t, rr.Body.String()).Code)
		_, err := s.userManager.User("ben")
		require.ErrorIs(t, err, user.ErrUserNotFound)

		// Invalid role over the socket
		rr = requestAdminSocket(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben", "role":"superuser"}`)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40066, toHTTPError(t, rr.Body.String()).Code)

		// Role over the socket works
		rr = requestAdminSocket(t, s, "POST", "/v1/users", `{"username": "ben", "password":"ben", "role":"admin"}`)
		require.Equal(t, 200, rr.Code)
		u, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Equal(t, user.RoleAdmin, u.Role)
	})
}

func TestUser_AddWithInvalidHash(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

		rr := request(t, s, "POST", "/v1/users", `{"username": "ben", "hash":"not-a-bcrypt-hash"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40069, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestUser_ChangeTierReset(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", Name: "Pro"}))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))

		rr := request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier":"-"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
		u, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Nil(t, u.Tier)
	})
}

func TestUser_ChangeTierTooManyReservations(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "big", Name: "Big", ReservationLimit: 5}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "small", Name: "Small", ReservationLimit: 1}))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("ben", "big"))
		require.Nil(t, s.userManager.AddReservation("ben", "topic1", user.PermissionDenyAll, 5))
		require.Nil(t, s.userManager.AddReservation("ben", "topic2", user.PermissionDenyAll, 5))

		rr := request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier":"small"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40063, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestUser_ChangeRole(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

		// Refused over HTTP
		rr := request(t, s, "PUT", "/v1/users", `{"username": "ben", "role":"admin"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 403, rr.Code)
		require.Equal(t, 40302, toHTTPError(t, rr.Body.String()).Code)

		// Invalid role, and missing user over the socket
		rr = requestAdminSocket(t, s, "PUT", "/v1/users", `{"username": "ben", "role":"superuser"}`)
		require.Equal(t, 40066, toHTTPError(t, rr.Body.String()).Code)
		rr = requestAdminSocket(t, s, "PUT", "/v1/users", `{"username": "emma", "role":"admin"}`)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)
		_, err := s.userManager.User("emma")
		require.ErrorIs(t, err, user.ErrUserNotFound)

		// Promote and demote over the socket
		rr = requestAdminSocket(t, s, "PUT", "/v1/users", `{"username": "ben", "role":"admin"}`)
		require.Equal(t, 200, rr.Code)
		u, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Equal(t, user.RoleAdmin, u.Role)
		rr = requestAdminSocket(t, s, "PUT", "/v1/users", `{"username": "ben", "role":"user"}`)
		require.Equal(t, 200, rr.Code)
		u, err = s.userManager.User("ben")
		require.Nil(t, err)
		require.Equal(t, user.RoleUser, u.Role)
	})
}

func TestUser_ChangeAdminPasswordOverSocket(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("admin", "admin", user.RoleAdmin, false))

		rr := requestAdminSocket(t, s, "PUT", "/v1/users", `{"username": "admin", "password": "admin-new"}`)
		require.Equal(t, 200, rr.Code)
		_, err := s.userManager.Authenticate("admin", "admin-new")
		require.Nil(t, err)
	})
}

func TestUser_DeleteAdmin(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("admin", "admin", user.RoleAdmin, false))

		// Refused over HTTP
		rr := request(t, s, "DELETE", "/v1/users", `{"username": "admin"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 401, rr.Code)

		// Allowed over the socket
		rr = requestAdminSocket(t, s, "DELETE", "/v1/users", `{"username": "admin"}`)
		require.Equal(t, 200, rr.Code)
		_, err := s.userManager.User("admin")
		require.ErrorIs(t, err, user.ErrUserNotFound)
	})
}

func TestUser_DeleteRemovesWebPushSubscriptions(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, configureAuth(t, newTestConfigWithWebPush(t, databaseURL)))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		u, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Nil(t, s.webPush.UpsertSubscription(testWebPushEndpoint, "kSC3T8aN1JCQxxPdrFLrZg", "BMKKbxdUU_xLS7G1Wh5AN8PvWOjCzkCuKZYb8apcqYrDxjOF_2piggBnoJLQYx9IeSD70fNuwawI3e9Y8m3S3PE", u.ID, netip.MustParseAddr("1.2.3.4"), []string{"test-topic"}))
		requireSubscriptionCount(t, s, "test-topic", 1)

		rr := requestAdminSocket(t, s, "DELETE", "/v1/users", `{"username": "ben"}`)
		require.Equal(t, 200, rr.Code)
		requireSubscriptionCount(t, s, "test-topic", 0)
	})
}

func TestAccess_AllowAdminUser(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))

		rr := requestAdminSocket(t, s, "PUT", "/v1/users/access", `{"username": "phil", "topic":"gold", "permission":"ro"}`)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40064, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestAccess_ResetAll(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("emma", "emma", user.RoleUser, false))
		require.Nil(t, s.userManager.AllowAccess("ben", "gold", user.PermissionRead))
		require.Nil(t, s.userManager.AllowAccess("emma", "silver", user.PermissionRead))
		require.Nil(t, s.userManager.AllowAccess(user.Everyone, "public", user.PermissionRead))

		rr := requestAdminSocket(t, s, "DELETE", "/v1/users/access", `{}`)
		require.Equal(t, 200, rr.Code)
		grants, err := s.userManager.AllGrants()
		require.Nil(t, err)
		require.Empty(t, grants)
	})
}

func TestAccess_ResetUserAllTopics(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("emma", "emma", user.RoleUser, false))
		require.Nil(t, s.userManager.AllowAccess("ben", "gold", user.PermissionRead))
		require.Nil(t, s.userManager.AllowAccess("ben", "silver", user.PermissionRead))
		require.Nil(t, s.userManager.AllowAccess("emma", "silver", user.PermissionRead))

		rr := requestAdminSocket(t, s, "DELETE", "/v1/users/access", `{"username":"ben"}`)
		require.Equal(t, 200, rr.Code)
		bensGrants, err := s.userManager.Grants("ben")
		require.Nil(t, err)
		require.Empty(t, bensGrants)
		emmasGrants, err := s.userManager.Grants("emma")
		require.Nil(t, err)
		require.Len(t, emmasGrants, 1)

		// Unknown user
		rr = requestAdminSocket(t, s, "DELETE", "/v1/users/access", `{"username":"nobody"}`)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestTokens_AddListRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("emma", "emma", user.RoleUser, false))
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// No tokens yet: empty array, not null
		rr := request(t, s, "GET", "/v1/users/tokens", "", auth)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "[]", strings.TrimSpace(rr.Body.String()))

		// Add a token that never expires, and one that does
		rr = request(t, s, "POST", "/v1/users/tokens", `{"username":"ben","label":"forever"}`, auth)
		require.Equal(t, 200, rr.Code)
		var forever apiUserTokenResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&forever))
		require.Equal(t, "ben", forever.Username)
		require.Equal(t, "forever", forever.Label)
		require.Equal(t, int64(0), forever.Expires)
		require.True(t, user.ValidToken(forever.Token))

		expires := time.Now().Add(time.Hour).Unix()
		rr = request(t, s, "POST", "/v1/users/tokens", fmt.Sprintf(`{"username":"emma","expires":%d}`, expires), auth)
		require.Equal(t, 200, rr.Code)
		var expiring apiUserTokenResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&expiring))
		require.Equal(t, expires, expiring.Expires)

		// The new token works
		rr = request(t, s, "GET", "/v1/account", "", map[string]string{"Authorization": util.BearerAuth(forever.Token)})
		require.Equal(t, 200, rr.Code)

		// List all, and one user
		rr = request(t, s, "GET", "/v1/users/tokens", "", auth)
		require.Equal(t, 200, rr.Code)
		var tokens []*apiUserTokenResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&tokens))
		require.Len(t, tokens, 2)
		rr = request(t, s, "GET", "/v1/users/tokens?username=ben", "", auth)
		require.Equal(t, 200, rr.Code)
		tokens = nil
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&tokens))
		require.Len(t, tokens, 1)
		require.Equal(t, forever.Token, tokens[0].Token)

		// Remove
		rr = request(t, s, "DELETE", "/v1/users/tokens", fmt.Sprintf(`{"username":"ben","token":"%s"}`, forever.Token), auth)
		require.Equal(t, 200, rr.Code)
		bensTokens, err := s.userManager.Tokens(mustUserID(t, s, "ben"))
		require.Nil(t, err)
		require.Empty(t, bensTokens)
	})
}

func TestTokens_Failures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		conf := newTestConfigWithAuthFile(t, databaseURL)
		conf.AuthUsers = []*user.User{
			{Name: "philuser", Hash: "$2a$10$U4WSIYY6evyGmZaraavM2e2JeVG6EMGUKN1uUwufUeeRd4Jpg6cGC", Role: user.RoleUser}, // philuser:philpass
		}
		conf.AuthTokens = map[string][]*user.Token{
			"philuser": {{Value: "tk_op56p8lz5bf3cxkz9je99v9oc37lo", Label: "provisioned"}},
		}
		s := newTestServer(t, conf)
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Expiry in the past
		rr := request(t, s, "POST", "/v1/users/tokens", fmt.Sprintf(`{"username":"ben","expires":%d}`, time.Now().Add(-time.Hour).Unix()), auth)
		require.Equal(t, 400, rr.Code)

		// Unknown user, everyone
		rr = request(t, s, "POST", "/v1/users/tokens", `{"username":"nobody"}`, auth)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "POST", "/v1/users/tokens", `{"username":"*"}`, auth)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "GET", "/v1/users/tokens?username=nobody", "", auth)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)

		// Unknown token, provisioned token
		rr = request(t, s, "DELETE", "/v1/users/tokens", `{"username":"ben","token":"tk_doesnotexistdoesnotexist12345"}`, auth)
		require.Equal(t, 40065, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "DELETE", "/v1/users/tokens", `{"username":"philuser","token":"tk_op56p8lz5bf3cxkz9je99v9oc37lo"}`, auth)
		require.Equal(t, 40906, toHTTPError(t, rr.Body.String()).Code)

		// Non-admins cannot use the API
		rr = request(t, s, "GET", "/v1/users/tokens", "", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)
	})
}

func TestTokens_AdminTokensOnlyOverSocket(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		_, err := s.userManager.CreateToken(mustUserID(t, s, "phil"), "admin token", time.Unix(0, 0), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		_, err = s.userManager.CreateToken(mustUserID(t, s, "ben"), "user token", time.Unix(0, 0), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Over HTTP, admin tokens are hidden and cannot be created or removed
		rr := request(t, s, "GET", "/v1/users/tokens", "", auth)
		require.Equal(t, 200, rr.Code)
		var tokens []*apiUserTokenResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&tokens))
		require.Len(t, tokens, 1)
		require.Equal(t, "ben", tokens[0].Username)
		rr = request(t, s, "POST", "/v1/users/tokens", `{"username":"phil"}`, auth)
		require.Equal(t, 403, rr.Code)

		// Over the socket, they are visible and can be created
		rr = requestAdminSocket(t, s, "GET", "/v1/users/tokens", "")
		require.Equal(t, 200, rr.Code)
		tokens = nil
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&tokens))
		require.Len(t, tokens, 2)
		rr = requestAdminSocket(t, s, "POST", "/v1/users/tokens", `{"username":"phil"}`)
		require.Equal(t, 200, rr.Code)
	})
}

func TestPasswordReset_Link(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s, mailer, _ := newEmailTestServer(t, databaseURL)
		defer s.closeDatabases()
		ben, err := s.userManager.User("ben")
		require.Nil(t, err)

		// Link only
		rr := requestAdminSocket(t, s, "POST", "/v1/users/password/reset", `{"username":"ben"}`)
		require.Equal(t, 200, rr.Code)
		var resp apiUsersPasswordResetResponse
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&resp))
		require.Equal(t, "", resp.Email)
		token := tokenFromLink(t, resp.Link, "https://ntfy.example.com/account/password/reset/")
		link, err := s.userManager.MagicLinkByToken(token)
		require.Nil(t, err)
		require.Equal(t, ben.ID, link.UserID)
		require.Empty(t, mailer.resetLinks)

		// Send email: no primary email yet
		rr = requestAdminSocket(t, s, "POST", "/v1/users/password/reset", `{"username":"ben","send_email":true}`)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40067, toHTTPError(t, rr.Body.String()).Code)

		// Send email to the primary email
		require.Nil(t, s.userManager.AddEmail(ben.ID, "ben@example.com"))
		require.Nil(t, s.userManager.SetPrimaryEmail(ben.ID, "ben@example.com"))
		rr = requestAdminSocket(t, s, "POST", "/v1/users/password/reset", `{"username":"ben","send_email":true}`)
		require.Equal(t, 200, rr.Code)
		resp = apiUsersPasswordResetResponse{}
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&resp))
		require.Equal(t, "ben@example.com", resp.Email)
		require.Equal(t, resp.Link, mailer.resetLinks["ben@example.com"])

		// Unknown user
		rr = requestAdminSocket(t, s, "POST", "/v1/users/password/reset", `{"username":"nobody"}`)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestPasswordReset_Failures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		conf := newTestConfigWithAuthFile(t, databaseURL)
		conf.AuthUsers = []*user.User{
			{Name: "philuser", Hash: "$2a$10$U4WSIYY6evyGmZaraavM2e2JeVG6EMGUKN1uUwufUeeRd4Jpg6cGC", Role: user.RoleUser}, // philuser:philpass
		}
		s := newTestServer(t, conf)
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// No mailer configured
		rr := request(t, s, "POST", "/v1/users/password/reset", `{"username":"ben","send_email":true}`, auth)
		require.Equal(t, 400, rr.Code)

		// Provisioned user, admin over HTTP
		rr = request(t, s, "POST", "/v1/users/password/reset", `{"username":"philuser"}`, auth)
		require.Equal(t, 40905, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "POST", "/v1/users/password/reset", `{"username":"phil"}`, auth)
		require.Equal(t, 403, rr.Code)

		// No base URL
		s.config.BaseURL = ""
		rr = request(t, s, "POST", "/v1/users/password/reset", `{"username":"ben"}`, auth)
		require.Equal(t, 40068, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestTiers_AddListUpdateRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Add
		rr := request(t, s, "POST", "/v1/admin/tiers", `{"code":"pro","name":"Pro","messages_limit":1000,"messages_expiry_duration":86400,"attachment_file_size_limit":1048576}`, auth)
		require.Equal(t, 200, rr.Code)
		tier, err := s.userManager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, int64(1000), tier.MessageLimit)
		require.Equal(t, 24*time.Hour, tier.MessageExpiryDuration)
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))

		// List
		rr = request(t, s, "GET", "/v1/admin/tiers", "", auth)
		require.Equal(t, 200, rr.Code)
		var tiers []*apiAdminTier
		require.Nil(t, json.NewDecoder(rr.Body).Decode(&tiers))
		require.Len(t, tiers, 1)
		require.Equal(t, tier.ID, tiers[0].ID)
		require.Equal(t, "Pro", tiers[0].Name)
		require.Equal(t, int64(86400), tiers[0].MessagesExpiryDuration)
		require.Equal(t, int64(1048576), tiers[0].AttachmentFileSizeLimit)
		require.Equal(t, int64(1), tiers[0].Users)

		// Update
		rr = request(t, s, "PUT", "/v1/admin/tiers", `{"code":"pro","name":"Pro Plus","messages_limit":2000}`, auth)
		require.Equal(t, 200, rr.Code)
		tier, err = s.userManager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "Pro Plus", tier.Name)
		require.Equal(t, int64(2000), tier.MessageLimit)

		// Remove: refused while in use, works after moving the user
		rr = request(t, s, "DELETE", "/v1/admin/tiers", `{"code":"pro"}`, auth)
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40910, toHTTPError(t, rr.Body.String()).Code)
		require.Nil(t, s.userManager.ResetTier("ben"))
		rr = request(t, s, "DELETE", "/v1/admin/tiers", `{"code":"pro"}`, auth)
		require.Equal(t, 200, rr.Code)
		_, err = s.userManager.Tier("pro")
		require.ErrorIs(t, err, user.ErrTierNotFound)
	})
}

func TestTiers_Failures(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", Name: "Pro"}))
		auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Duplicate, bad code, missing name
		rr := request(t, s, "POST", "/v1/admin/tiers", `{"code":"pro","name":"Pro"}`, auth)
		require.Equal(t, 40909, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "POST", "/v1/admin/tiers", `{"code":"not valid","name":"X"}`, auth)
		require.Equal(t, 40062, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "POST", "/v1/admin/tiers", `{"code":"x"}`, auth)
		require.Equal(t, 40062, toHTTPError(t, rr.Body.String()).Code)

		// Update or remove a missing tier
		rr = request(t, s, "PUT", "/v1/admin/tiers", `{"code":"missing","name":"Missing"}`, auth)
		require.Equal(t, 40030, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "DELETE", "/v1/admin/tiers", `{"code":"missing"}`, auth)
		require.Equal(t, 40030, toHTTPError(t, rr.Body.String()).Code)

		// Non-admins cannot use the API
		rr = request(t, s, "GET", "/v1/admin/tiers", "", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)
	})
}

// requestAdminSocket sends a request through the admin socket handler, i.e. with socket semantics
// (no Authorization header, contextAdminSocket set), without a listener.
func requestAdminSocket(t *testing.T, s *Server, method, url, body string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	r, err := http.NewRequest(method, url, strings.NewReader(body))
	require.Nil(t, err)
	s.handleAdminSocket(rr, r)
	return rr
}

func mustUserID(t *testing.T, s *Server, username string) string {
	u, err := s.userManager.User(username)
	require.Nil(t, err)
	return u.ID
}
