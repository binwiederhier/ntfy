package server

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"io"
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

func TestUser_GetPaginated(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthUsers = []*user.User{{Name: "prov", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: user.RoleUser}}
		c.AuthAccess = map[string][]*user.Grant{"prov": {{TopicPattern: "prov_topic", Permission: user.PermissionRead}}}
		s := newTestServer(t, c)
		defer s.closeDatabases()

		// Create admin and some users, one with reservations
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		for i := 0; i < 12; i++ {
			require.Nil(t, s.userManager.AddUser(fmt.Sprintf("user%02d", i), "pass", user.RoleUser, false))
		}
		require.Nil(t, s.userManager.AllowAccess("user00", "granted", user.PermissionRead))
		require.Nil(t, s.userManager.AddReservation("user00", "mytopic", user.PermissionRead, 0))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// First page
		rr := request(t, s, "GET", "/v1/users?limit=3", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "15", rr.Header().Get("X-Total-Count")) // phil, prov, 12 users, Everyone
		users, err := util.UnmarshalJSON[[]*apiUserResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Len(t, *users, 3)
		require.Equal(t, "phil", (*users)[0].Username)
		require.Equal(t, "prov", (*users)[1].Username)
		require.True(t, (*users)[1].Provisioned)
		require.Len(t, (*users)[1].Grants, 1)
		require.True(t, (*users)[1].Grants[0].Provisioned)
		require.Equal(t, "user00", (*users)[2].Username)
		require.False(t, (*users)[2].Provisioned)
		require.Len(t, (*users)[2].Grants, 2)
		require.Equal(t, []*apiUserReservationResponse{{Topic: "mytopic", Everyone: "read-only"}}, (*users)[2].Reservations)

		// Second page, and search
		rr = request(t, s, "GET", "/v1/users?limit=3&offset=3", "", admin)
		require.Equal(t, 200, rr.Code)
		users, err = util.UnmarshalJSON[[]*apiUserResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, "user01", (*users)[0].Username)
		rr = request(t, s, "GET", "/v1/users?search=ER1", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "2", rr.Header().Get("X-Total-Count"))
		users, err = util.UnmarshalJSON[[]*apiUserResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, "user10", (*users)[0].Username)
		require.Equal(t, "user11", (*users)[1].Username)

		// No params returns all users (backwards compatible)
		rr = request(t, s, "GET", "/v1/users", "", admin)
		require.Equal(t, 200, rr.Code)
		users, err = util.UnmarshalJSON[[]*apiUserResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Len(t, *users, 15)

		// Invalid params
		rr = request(t, s, "GET", "/v1/users?limit=abc", "", admin)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "GET", "/v1/users?offset=-1", "", admin)
		require.Equal(t, 400, rr.Code)
	})
}

func TestUser_ChangeTierResetAndReservationLimit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", ReservationLimit: 2}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "small", ReservationLimit: 1}))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))
		require.Nil(t, s.userManager.AddReservation("ben", "topic1", user.PermissionDenyAll, 0))
		require.Nil(t, s.userManager.AddReservation("ben", "topic2", user.PermissionDenyAll, 0))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Downgrade fails, too many reservations
		rr := request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier": "small"}`, admin)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)

		// Remove reservation, then removing the tier still fails (no tier = no reservations)
		require.Nil(t, s.userManager.RemoveReservations("ben", "topic2"))
		rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier": "-"}`, admin)
		require.Equal(t, 400, rr.Code)

		// Downgrade works now, and so does removing the tier without reservations
		rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier": "small"}`, admin)
		require.Equal(t, 200, rr.Code)
		require.Nil(t, s.userManager.RemoveReservations("ben", "topic1"))
		rr = request(t, s, "PUT", "/v1/users", `{"username": "ben", "tier": "-"}`, admin)
		require.Equal(t, 200, rr.Code)
		u, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Nil(t, u.Tier)
	})
}

func TestUser_ProvisionedCannotBeChanged(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthUsers = []*user.User{{Name: "prov", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: user.RoleUser}}
		c.AuthAccess = map[string][]*user.Grant{"prov": {{TopicPattern: "prov_topic", Permission: user.PermissionRead}}}
		s := newTestServer(t, c)
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Password change and deletion are rejected
		rr := request(t, s, "PUT", "/v1/users", `{"username": "prov", "password": "new"}`, admin)
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40905, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "DELETE", "/v1/users", `{"username": "prov"}`, admin)
		require.Equal(t, 409, rr.Code)

		// Changing or resetting the provisioned grant is rejected
		rr = request(t, s, "PUT", "/v1/users/access", `{"username": "prov", "topic": "prov_topic", "permission": "rw"}`, admin)
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40909, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "DELETE", "/v1/users/access", `{"username": "prov", "topic": "prov_topic"}`, admin)
		require.Equal(t, 409, rr.Code)
		rr = request(t, s, "DELETE", "/v1/users/access", `{"username": "prov"}`, admin)
		require.Equal(t, 409, rr.Code)

		// Adding another, non-provisioned grant is fine
		rr = request(t, s, "PUT", "/v1/users/access", `{"username": "prov", "topic": "other", "permission": "rw"}`, admin)
		require.Equal(t, 200, rr.Code)
		grants, err := s.userManager.Grants("prov")
		require.Nil(t, err)
		require.Len(t, grants, 2)
	})
}

func TestAccess_AllowRejectsAdminsAndReservations(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddReservation("ben", "bens_topic", user.PermissionDenyAll, 0))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Admins have full access, grants make no sense
		rr := request(t, s, "PUT", "/v1/users/access", `{"username": "phil", "topic": "abc", "permission": "rw"}`, admin)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40061, toHTTPError(t, rr.Body.String()).Code)

		// Overwriting the user's own reservation would turn it into a regular grant
		rr = request(t, s, "PUT", "/v1/users/access", `{"username": "ben", "topic": "bens_topic", "permission": "ro"}`, admin)
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40910, toHTTPError(t, rr.Body.String()).Code)
		reservations, err := s.userManager.Reservations("ben")
		require.Nil(t, err)
		require.Len(t, reservations, 1)

		// Resetting it removes the reservation, including the Everyone entry
		rr = request(t, s, "DELETE", "/v1/users/access", `{"username": "ben", "topic": "bens_topic"}`, admin)
		require.Equal(t, 200, rr.Code)
		entries, _, err := s.userManager.AccessEntries(&user.AccessFilter{})
		require.Nil(t, err)
		require.Len(t, entries, 0)
	})
}

func TestAccess_GetPaginated(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AllowAccess("ben", "alerts", user.PermissionReadWrite))
		require.Nil(t, s.userManager.AllowAccess(user.Everyone, "announce*", user.PermissionRead))
		require.Nil(t, s.userManager.AddReservation("ben", "bens_topic", user.PermissionWrite, 0))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		rr := request(t, s, "GET", "/v1/users/access?limit=2&offset=1", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "4", rr.Header().Get("X-Total-Count"))
		entries, err := util.UnmarshalJSON[[]*apiAccessEntryResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, []*apiAccessEntryResponse{
			{Username: "*", Topic: "announce*", Permission: "read-only"},
			{Username: "ben", Topic: "bens_topic", Permission: "read-write", Owner: "ben"},
		}, *entries)

		rr = request(t, s, "GET", "/v1/users/access?search=bens", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "2", rr.Header().Get("X-Total-Count"))

		// Non-admins cannot list the ACL
		rr = request(t, s, "GET", "/v1/users/access", "", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)
	})
}

func TestTier_AdminCRUD(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Non-admins cannot access tiers
		rr := request(t, s, "GET", "/v1/admin/tiers", "", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)

		// Add tier, and add it again (conflict), and with invalid code
		rr = request(t, s, "POST", "/v1/admin/tiers", `{"code":"pro","name":"Pro","messages_limit":1000,"messages_expiry_duration":86400,"reservations_limit":5,"attachment_file_size_limit":1048576}`, admin)
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "POST", "/v1/admin/tiers", `{"code":"pro","name":"Pro"}`, admin)
		require.Equal(t, 409, rr.Code)
		rr = request(t, s, "POST", "/v1/admin/tiers", `{"code":"no spaces","name":"Pro"}`, admin)
		require.Equal(t, 400, rr.Code)

		// Update tier
		rr = request(t, s, "PUT", "/v1/admin/tiers", `{"code":"pro","name":"Pro Plus","messages_limit":2000,"messages_expiry_duration":86400,"reservations_limit":5}`, admin)
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "PUT", "/v1/admin/tiers", `{"code":"doesnotexist","name":"x"}`, admin)
		require.Equal(t, 400, rr.Code)
		tier, err := s.userManager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "Pro Plus", tier.Name)
		require.Equal(t, int64(2000), tier.MessageLimit)
		require.Equal(t, 24*time.Hour, tier.MessageExpiryDuration)

		// List tiers, including user count
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))
		rr = request(t, s, "GET", "/v1/admin/tiers", "", admin)
		require.Equal(t, 200, rr.Code)
		tiers, err := util.UnmarshalJSON[[]*apiAdminTier](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Len(t, *tiers, 1)
		require.Equal(t, "pro", (*tiers)[0].Code)
		require.Equal(t, int64(2000), (*tiers)[0].MessagesLimit)
		require.Equal(t, int64(86400), (*tiers)[0].MessagesExpiryDuration)
		require.Equal(t, int64(1), (*tiers)[0].Users)

		// Delete fails while in use
		rr = request(t, s, "DELETE", "/v1/admin/tiers", `{"code":"pro"}`, admin)
		require.Equal(t, 409, rr.Code)
		require.Nil(t, s.userManager.ResetTier("ben"))
		rr = request(t, s, "DELETE", "/v1/admin/tiers", `{"code":"pro"}`, admin)
		require.Equal(t, 200, rr.Code)
		_, err = s.userManager.Tier("pro")
		require.ErrorIs(t, err, user.ErrTierNotFound)
	})
}

func TestReservations_AdminListAddChangeRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("emma", "emma", user.RoleUser, false))
		require.Nil(t, s.userManager.AddReservation("emma", "emmas_topic", user.PermissionDenyAll, 0))
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		// Reserve a topic for ben (admins are not bound by the tier's reservation limit)
		rr := request(t, s, "PUT", "/v1/users/reservations", `{"username":"ben","topic":"bens_topic","everyone":"read-only"}`, admin)
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "GET", "/bens_topic/json?poll=1", "", nil)
		require.Equal(t, 200, rr.Code) // Everyone can read
		rr = request(t, s, "PUT", "/bens_topic", "hi", nil)
		require.Equal(t, 403, rr.Code) // .. but not write

		// List, with paging and search
		rr = request(t, s, "GET", "/v1/users/reservations?limit=1", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "2", rr.Header().Get("X-Total-Count"))
		entries, err := util.UnmarshalJSON[[]*apiReservationEntryResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, []*apiReservationEntryResponse{{Username: "ben", Topic: "bens_topic", Everyone: "read-only"}}, *entries)
		rr = request(t, s, "GET", "/v1/users/reservations?search=emma", "", admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "1", rr.Header().Get("X-Total-Count"))

		// Change everyone access
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"ben","topic":"bens_topic","everyone":"deny-all"}`, admin)
		require.Equal(t, 200, rr.Code)
		rr = request(t, s, "GET", "/bens_topic/json?poll=1", "", nil)
		require.Equal(t, 403, rr.Code)

		// Topic reserved by someone else, invalid users, invalid permission
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"ben","topic":"emmas_topic","everyone":"deny-all"}`, admin)
		require.Equal(t, 409, rr.Code)
		require.Equal(t, 40902, toHTTPError(t, rr.Body.String()).Code)
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"phil","topic":"abc","everyone":"deny-all"}`, admin)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"*","topic":"abc","everyone":"deny-all"}`, admin)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"ben","topic":"abc","everyone":"invalid"}`, admin)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "PUT", "/v1/users/reservations", `{"username":"ben","topic":"abc*","everyone":"deny-all"}`, admin)
		require.Equal(t, 400, rr.Code)

		// Remove: not a reservation of this user, then for real
		rr = request(t, s, "DELETE", "/v1/users/reservations", `{"username":"ben","topic":"emmas_topic"}`, admin)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "DELETE", "/v1/users/reservations", `{"username":"ben","topic":"bens_topic"}`, admin)
		require.Equal(t, 200, rr.Code)
		reservations, err := s.userManager.Reservations("ben")
		require.Nil(t, err)
		require.Len(t, reservations, 0)

		// Non-admins cannot use the endpoint
		rr = request(t, s, "GET", "/v1/users/reservations", "", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)
	})
}
