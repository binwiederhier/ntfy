package server

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
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

func TestAccess_WritesTellPeers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// Subscribers are per node: an access write tells the other nodes to reload their access
		// cache (which ends the streams that lost access there). Deleting a user also asks them to
		// cancel the user's streams, since a user without access entries (e.g. an admin) leaves
		// nothing in the cache to find.
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		defer s.closeDatabases()
		b := &fakeCluster{}
		s.cluster = b
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AllowAccess("ben", "gol*", user.PermissionRead))
		ben, err := s.userManager.User("ben")
		require.Nil(t, err)
		admin := map[string]string{"Authorization": util.BasicAuth("phil", "phil")}

		before := b.AccessChanged()
		rr := request(t, s, "DELETE", "/v1/users/access", `{"username": "ben", "topic":"gol*"}`, admin)
		require.Equal(t, 200, rr.Code)
		require.Greater(t, b.AccessChanged(), before)
		require.Empty(t, b.Cancels())

		rr = request(t, s, "DELETE", "/v1/users", `{"username": "ben"}`, admin)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, []*cluster.SubscriberCancel{
			{
				Topic:  "*",
				UserID: ben.ID,
			},
		}, b.Cancels())
	})
}

func TestAccess_PeerAccessChangedReloadsAndCancels(t *testing.T) {
	// Told by a peer that access changed, this node reloads its access cache: the reload ends the
	// subscriber who lost access, and the client's reconnect is denied, not let in by a stale entry
	dsn := dbtest.CreateTestPostgresSchema(t)
	newNode := func() *Server {
		conf := newTestConfig(t, dsn)
		conf.AuthDefault = user.PermissionDenyAll
		conf.AuthAccessCacheEnabled = true
		conf.AuthAccessCacheReloadInterval = time.Hour
		return newTestServer(t, conf)
	}
	a := newNode()
	require.Nil(t, a.userManager.AddUser("alice", "password", user.RoleUser, false))
	require.Nil(t, a.userManager.AllowAccess("alice", "private", user.PermissionRead))
	b := newNode()
	headers := map[string]string{"Authorization": util.BasicAuth("alice", "password")}
	stream := subscribeStream(t, b, "private", headers)

	require.Nil(t, a.userManager.ResetAccess("alice", "private"))
	b.handlePeerAccessChanged()
	select {
	case <-stream.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("alice's stream was not canceled")
	}
	require.Equal(t, 403, request(t, b, "GET", "/private/json?poll=1", "", headers).Code)
}

func TestAccess_PeerUserCancelCancels(t *testing.T) {
	// A peer deleted a user: this node cancels the user's streams
	s := newTestServer(t, newTestConfigWithAuthFile(t, ""))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleAdmin, false))
	ben, err := s.userManager.User("ben")
	require.Nil(t, err)
	stream := subscribeStream(t, s, "mytopic", map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
	s.applySubscriberCancel(&cluster.SubscriberCancel{
		Topic:  "*",
		UserID: ben.ID,
	})
	select {
	case <-stream.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("ben's stream was not canceled")
	}
}

func TestAccess_ExternalRevocationCaughtByAccessReload(t *testing.T) {
	// An access change made elsewhere (another node whose relay got lost, or the ntfy access CLI)
	// reaches node B at its next access cache reload. B ends the streams of the user who lost
	// access, and only those: carol, still allowed on the same topic, keeps her stream.
	dsn := dbtest.CreateTestPostgresSchema(t)
	newNode := func() *Server {
		conf := newTestConfig(t, dsn)
		conf.AuthDefault = user.PermissionDenyAll
		conf.AuthAccessCacheEnabled = true
		conf.AuthAccessCacheReloadInterval = 500 * time.Millisecond
		return newTestServer(t, conf)
	}
	a := newNode()
	for _, name := range []string{"alice", "carol"} {
		require.Nil(t, a.userManager.AddUser(name, "password", user.RoleUser, false))
		require.Nil(t, a.userManager.AllowAccess(name, "private", user.PermissionRead))
	}
	b := newNode()
	alice := map[string]string{"Authorization": util.BasicAuth("alice", "password")}
	carol := map[string]string{"Authorization": util.BasicAuth("carol", "password")}
	aliceStream := subscribeStream(t, b, "private", alice)
	carolStream := subscribeStream(t, b, "private", carol)

	require.Nil(t, a.userManager.ResetAccess("alice", "private"))
	select {
	case <-aliceStream.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("alice's stream on node B was not canceled by its access cache reload")
	}
	require.Equal(t, 403, request(t, b, "GET", "/private/json?poll=1", "", alice).Code)
	time.Sleep(time.Second) // Two more reloads
	select {
	case <-carolStream.ended:
		t.Fatal("carol's stream was canceled, but she can still read the topic")
	default:
	}
}
