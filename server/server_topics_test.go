package server

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestTopics_ListShared(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		ben, err := s.userManager.User("ben")
		require.Nil(t, err)
		require.Nil(t, s.userManager.AddReservation("ben", "ben_shared", user.PermissionRead, 0))
		require.Nil(t, s.userManager.AddReservation("ben", "ben_private", user.PermissionRead, 0))
		require.Nil(t, s.userManager.SetTopicVisibility(ben.ID, "ben_shared", user.VisibilityShared))

		// Any authenticated user can see shared topics, not just admins
		rr := request(t, s, "GET", "/v1/topics?visibility=shared", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
		response, err := util.UnmarshalJSON[apiTopicsResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, 1, len(response.Topics))
		require.Equal(t, "ben_shared", response.Topics[0].Topic)
		require.Equal(t, "ben", response.Topics[0].Owner)

		// Anonymous users are rejected
		rr = request(t, s, "GET", "/v1/topics?visibility=shared", "", nil)
		require.Equal(t, 401, rr.Code)

		// Only ?visibility=shared is defined
		rr = request(t, s, "GET", "/v1/topics?visibility=private", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 400, rr.Code)
	})
}

func TestTopics_PatchVisibility(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("admin", "admin", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddReservation("ben", "mytopic", user.PermissionDenyAll, 0))

		// Non-owner cannot change visibility
		rr := request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 403, rr.Code)

		// A topic that is not reserved has no owner, so there is nothing to patch
		rr = request(t, s, "PATCH", "/v1/topics/nope", `{"visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 404, rr.Code)

		// Invalid visibility value
		rr = request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"bogus"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 400, rr.Code)

		// Owner can share it
		rr = request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)
		visibility, owner, err := s.userManager.TopicVisibility("mytopic")
		require.Nil(t, err)
		require.Equal(t, user.VisibilityShared, visibility)
		require.NotEmpty(t, owner)

		// Admin can change it back for the owner
		rr = request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"private"}`, map[string]string{
			"Authorization": util.BasicAuth("admin", "admin"),
		})
		require.Equal(t, 200, rr.Code)
		visibility, _, err = s.userManager.TopicVisibility("mytopic")
		require.Nil(t, err)
		require.Equal(t, user.VisibilityPrivate, visibility)

		// Anonymous users are rejected
		rr = request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"shared"}`, nil)
		require.Equal(t, 401, rr.Code)
	})
}

func TestTopics_SharedOnCreation_AnnouncesToDirectory(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", ReservationLimit: 5}))
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))

		// The directory is readable by any authenticated user even under deny-all ...
		rr := request(t, s, "GET", "/~directory/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)

		// ... but not by anonymous users
		rr = request(t, s, "GET", "/~directory/json?poll=1", "", nil)
		require.Equal(t, 403, rr.Code)

		// Creating a shared reservation announces it on the directory
		rr = request(t, s, "POST", "/v1/account/reservation", `{"topic":"announced","everyone":"deny-all","visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)
		visibility, owner, err := s.userManager.TopicVisibility("announced")
		require.Nil(t, err)
		require.Equal(t, user.VisibilityShared, visibility)
		require.NotEmpty(t, owner)

		rr = request(t, s, "GET", "/~directory/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)
		require.Contains(t, rr.Body.String(), "announced")
		require.Contains(t, rr.Body.String(), "ben")
	})
}

func TestTopics_ReservedNameCannotBeClaimedOrRead(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

		// A regular user cannot reserve a reserved system topic name via the API ...
		rr := request(t, s, "POST", "/v1/account/reservation", `{"topic":"~directory","everyone":"deny-all"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 400, rr.Code)

		// ... nor via the manager directly.
		require.Equal(t, user.ErrInvalidArgument, s.userManager.AddReservation("ben", "~directory", user.PermissionDenyAll, 0))

		// Other reserved system topics stay internal, even for authenticated users.
		rr = request(t, s, "GET", "/~control/json?poll=1", "", map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 403, rr.Code)
	})
}

// TestTopics_PatchVisibility_UpgradesDenyAllEveryone verifies that sharing a topic whose Everyone
// ACL is deny-all immediately makes it readable (no separate grant needed), and that going private
// again does not revert the grant.
func TestTopics_PatchVisibility_UpgradesDenyAllEveryone(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		phil, err := s.userManager.User("phil")
		require.Nil(t, err)
		require.Nil(t, s.userManager.AddReservation("ben", "mytopic", user.PermissionDenyAll, 0))
		require.Equal(t, user.ErrUnauthorized, s.userManager.Authorize(phil, "mytopic", user.PermissionRead))

		rr := request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)

		// The Everyone grant was upgraded, so a new subscriber can now read.
		require.Nil(t, s.userManager.Authorize(phil, "mytopic", user.PermissionRead))
		reservations, err := s.userManager.Reservations("ben")
		require.Nil(t, err)
		require.Equal(t, 1, len(reservations))
		require.Equal(t, user.PermissionRead, reservations[0].Everyone)

		// Asymmetry: private does not revert the Everyone grant.
		rr = request(t, s, "PATCH", "/v1/topics/mytopic", `{"visibility":"private"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)
		require.Nil(t, s.userManager.Authorize(phil, "mytopic", user.PermissionRead))
	})
}

// TestTopics_CreationShared_UpgradesDenyAllEveryone verifies the same upgrade happens on the
// reservation-creation path when visibility=shared is requested (as Discover's flow can do).
func TestTopics_CreationShared_UpgradesDenyAllEveryone(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()

		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "pro", ReservationLimit: 5}))
		require.Nil(t, s.userManager.ChangeTier("ben", "pro"))
		phil, err := s.userManager.User("phil")
		require.Nil(t, err)

		rr := request(t, s, "POST", "/v1/account/reservation", `{"topic":"fresh","everyone":"deny-all","visibility":"shared"}`, map[string]string{
			"Authorization": util.BasicAuth("ben", "ben"),
		})
		require.Equal(t, 200, rr.Code)

		require.Nil(t, s.userManager.Authorize(phil, "fresh", user.PermissionRead))
		reservations, err := s.userManager.Reservations("ben")
		require.Nil(t, err)
		require.Equal(t, 1, len(reservations))
		require.Equal(t, user.PermissionRead, reservations[0].Everyone)
	})
}
