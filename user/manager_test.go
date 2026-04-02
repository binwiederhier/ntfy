package user

import (
	"database/sql"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/util"
)

const minBcryptTimingMillis = int64(40) // Ideally should be >100ms, but this should also run on a Raspberry Pi without massive resources

// newManagerFunc creates a Manager with the given config. Calling it multiple
// times within the same test returns a new Manager pointing at the same
// underlying data (same SQLite file / same PostgreSQL schema), enabling
// close-and-reopen tests.
type newManagerFunc func(config *Config) *Manager

func forEachBackend(t *testing.T, f func(t *testing.T, newManager newManagerFunc)) {
	t.Run("sqlite", func(t *testing.T) {
		dir := t.TempDir()
		f(t, func(config *Config) *Manager {
			a, err := NewSQLiteManager(filepath.Join(dir, "user.db"), "", config)
			require.Nil(t, err)
			return a
		})
	})
	t.Run("postgres", func(t *testing.T) {
		schemaDSN := dbtest.CreateTestPostgresSchema(t)
		f(t, func(config *Config) *Manager {
			host, err := pg.Open(schemaDSN)
			require.Nil(t, err)
			a, err := NewPostgresManager(db.New(host, nil), config)
			require.Nil(t, err)
			return a
		})
	})
}

func TestManager_FullScenario_Default_DenyAll(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleAdmin, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AddUser("john", "john", RoleUser, false))
		require.Nil(t, a.AllowAccess("ben", "mytopic", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("ben", "readme", PermissionRead))
		require.Nil(t, a.AllowAccess("ben", "writeme", PermissionWrite))
		require.Nil(t, a.AllowAccess("ben", "everyonewrite", PermissionDenyAll)) // How unfair!
		require.Nil(t, a.AllowAccess("john", "*", PermissionRead))
		require.Nil(t, a.AllowAccess("john", "mytopic*", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("john", "mytopic_ro*", PermissionRead))
		require.Nil(t, a.AllowAccess("john", "mytopic_deny*", PermissionDenyAll))
		require.Nil(t, a.AllowAccess(Everyone, "announcements", PermissionRead))
		require.Nil(t, a.AllowAccess(Everyone, "everyonewrite", PermissionReadWrite))
		require.Nil(t, a.AllowAccess(Everyone, "up*", PermissionWrite)) // Everyone can write to /up*

		phil, err := a.Authenticate("phil", "phil")
		require.Nil(t, err)
		require.Equal(t, "phil", phil.Name)
		require.True(t, strings.HasPrefix(phil.Hash, "$2a$04$"))
		require.Equal(t, RoleAdmin, phil.Role)

		philGrants, err := a.Grants("phil")
		require.Nil(t, err)
		require.Equal(t, []Grant{}, philGrants)

		ben, err := a.Authenticate("ben", "ben")
		require.Nil(t, err)
		require.Equal(t, "ben", ben.Name)
		require.True(t, strings.HasPrefix(ben.Hash, "$2a$04$"))
		require.Equal(t, RoleUser, ben.Role)

		benGrants, err := a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, []Grant{
			{"everyonewrite", PermissionDenyAll, false},
			{"mytopic", PermissionReadWrite, false},
			{"writeme", PermissionWrite, false},
			{"readme", PermissionRead, false},
		}, benGrants)

		john, err := a.Authenticate("john", "john")
		require.Nil(t, err)
		require.Equal(t, "john", john.Name)
		require.True(t, strings.HasPrefix(john.Hash, "$2a$04$"))
		require.Equal(t, RoleUser, john.Role)

		johnGrants, err := a.Grants("john")
		require.Nil(t, err)
		require.Equal(t, []Grant{
			{"mytopic_deny*", PermissionDenyAll, false},
			{"mytopic_ro*", PermissionRead, false},
			{"mytopic*", PermissionReadWrite, false},
			{"*", PermissionRead, false},
		}, johnGrants)

		notben, err := a.Authenticate("ben", "this is wrong")
		require.Nil(t, notben)
		require.Equal(t, ErrUnauthenticated, err)

		// Admin can do everything
		require.Nil(t, a.Authorize(phil, "sometopic", PermissionWrite))
		require.Nil(t, a.Authorize(phil, "mytopic", PermissionRead))
		require.Nil(t, a.Authorize(phil, "readme", PermissionWrite))
		require.Nil(t, a.Authorize(phil, "writeme", PermissionWrite))
		require.Nil(t, a.Authorize(phil, "announcements", PermissionWrite))
		require.Nil(t, a.Authorize(phil, "everyonewrite", PermissionWrite))

		// User cannot do everything
		require.Nil(t, a.Authorize(ben, "mytopic", PermissionWrite))
		require.Nil(t, a.Authorize(ben, "mytopic", PermissionRead))
		require.Nil(t, a.Authorize(ben, "readme", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "readme", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "writeme", PermissionRead))
		require.Nil(t, a.Authorize(ben, "writeme", PermissionWrite))
		require.Nil(t, a.Authorize(ben, "writeme", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "everyonewrite", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "everyonewrite", PermissionWrite))
		require.Nil(t, a.Authorize(ben, "announcements", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "announcements", PermissionWrite))

		// User john should have
		//  "deny" to mytopic_deny*,
		//    "ro" to mytopic_ro*,
		//    "rw" to mytopic*,
		//    "ro" to the rest
		require.Equal(t, ErrUnauthorized, a.Authorize(john, "mytopic_deny_case", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(john, "mytopic_deny_case", PermissionWrite))
		require.Nil(t, a.Authorize(john, "mytopic_ro_test_case", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(john, "mytopic_ro_test_case", PermissionWrite))
		require.Nil(t, a.Authorize(john, "mytopic_case1", PermissionRead))
		require.Nil(t, a.Authorize(john, "mytopic_case1", PermissionWrite))
		require.Nil(t, a.Authorize(john, "readme", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(john, "writeme", PermissionWrite))

		// Everyone else can do barely anything
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "mytopic", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "mytopic", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "readme", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "readme", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "writeme", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "writeme", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "announcements", PermissionWrite))
		require.Nil(t, a.Authorize(nil, "announcements", PermissionRead))
		require.Nil(t, a.Authorize(nil, "everyonewrite", PermissionRead))
		require.Nil(t, a.Authorize(nil, "everyonewrite", PermissionWrite))
		require.Nil(t, a.Authorize(nil, "up1234", PermissionWrite)) // Wildcard permission
		require.Nil(t, a.Authorize(nil, "up5678", PermissionWrite))
	})
}

func TestManager_Access_Order_LengthWriteRead(t *testing.T) {
	// This test validates issue #914 / #917, i.e. that write permissions are prioritized over read permissions,
	// and longer ACL rules are prioritized as well.
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AllowAccess("ben", "test*", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("ben", "*", PermissionRead))

		ben, err := a.Authenticate("ben", "ben")
		require.Nil(t, err)
		require.Nil(t, a.Authorize(ben, "any-topic-can-be-read", PermissionRead))
		require.Nil(t, a.Authorize(ben, "this-too", PermissionRead))
		require.Nil(t, a.Authorize(ben, "test123", PermissionWrite))
	})
}

func TestManager_AddUser_Invalid(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Equal(t, ErrInvalidArgument, a.AddUser("  invalid  ", "pass", RoleAdmin, false))
		require.Equal(t, ErrInvalidArgument, a.AddUser("validuser", "pass", "invalid-role", false))
	})
}

func TestManager_AddUser_Timing(t *testing.T) {
	a := newTestManagerFromFile(t, filepath.Join(t.TempDir(), "user.db"), "", PermissionDenyAll, DefaultUserPasswordBcryptCost, DefaultUserStatsQueueWriterInterval)
	start := time.Now().UnixMilli()
	require.Nil(t, a.AddUser("user", "pass", RoleAdmin, false))
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestManager_AddUser_And_Query(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("user", "pass", RoleAdmin, false))
		require.Nil(t, a.ChangeBilling("user", &Billing{
			StripeCustomerID:            "acct_123",
			StripeSubscriptionID:        "sub_123",
			StripeSubscriptionStatus:    "active",
			StripeSubscriptionInterval:  "month",
			StripeSubscriptionPaidUntil: time.Now().Add(time.Hour),
			StripeSubscriptionCancelAt:  time.Unix(0, 0),
		}))

		u, err := a.User("user")
		require.Nil(t, err)
		require.Equal(t, "user", u.Name)

		u2, err := a.UserByID(u.ID)
		require.Nil(t, err)
		require.Equal(t, u.Name, u2.Name)

		u3, err := a.UserByStripeCustomer("acct_123")
		require.Nil(t, err)
		require.Equal(t, u.ID, u3.ID)
	})
}

func TestManager_MarkUserRemoved_RemoveDeletedUsers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		// Create user, add reservations and token
		require.Nil(t, a.AddUser("user", "pass", RoleAdmin, false))
		require.Nil(t, a.AddReservation("user", "mytopic", PermissionRead, 0))

		u, err := a.User("user")
		require.Nil(t, err)
		require.False(t, u.Deleted)

		token, err := a.CreateToken(u.ID, "", time.Now().Add(time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)

		u, err = a.Authenticate("user", "pass")
		require.Nil(t, err)

		_, err = a.AuthenticateToken(token.Value)
		require.Nil(t, err)

		reservations, err := a.Reservations("user")
		require.Nil(t, err)
		require.Equal(t, 1, len(reservations))

		// Mark deleted: cannot auth anymore, and all reservations are gone
		require.Nil(t, a.MarkUserRemoved(u))

		_, err = a.Authenticate("user", "pass")
		require.Equal(t, ErrUnauthenticated, err)

		_, err = a.AuthenticateToken(token.Value)
		require.Equal(t, ErrUnauthenticated, err)

		reservations, err = a.Reservations("user")
		require.Nil(t, err)
		require.Equal(t, 0, len(reservations))

		// Make sure user is still there
		u, err = a.User("user")
		require.Nil(t, err)
		require.True(t, u.Deleted)

		// Backdate the deleted timestamp so RemoveDeletedUsers will prune the user
		_, err = testDB(a).Exec(a.queries.updateUserDeleted, time.Now().Add(-1*(userHardDeleteAfterDuration+time.Hour)).Unix(), u.ID)
		require.Nil(t, err)
		require.Nil(t, a.RemoveDeletedUsers())

		_, err = a.User("user")
		require.Equal(t, ErrUserNotFound, err)
	})
}

func TestManager_CreateToken_Only_Lower(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		// Create user, add reservations and token
		require.Nil(t, a.AddUser("user", "pass", RoleAdmin, false))
		u, err := a.User("user")
		require.Nil(t, err)

		token, err := a.CreateToken(u.ID, "", time.Now().Add(time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.Equal(t, token.Value, strings.ToLower(token.Value))
	})
}

func TestManager_UserManagement(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleAdmin, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AllowAccess("ben", "mytopic", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("ben", "readme", PermissionRead))
		require.Nil(t, a.AllowAccess("ben", "writeme", PermissionWrite))
		require.Nil(t, a.AllowAccess("ben", "everyonewrite", PermissionDenyAll)) // How unfair!
		require.Nil(t, a.AllowAccess(Everyone, "announcements", PermissionRead))
		require.Nil(t, a.AllowAccess(Everyone, "everyonewrite", PermissionReadWrite))

		// Query user details
		phil, err := a.User("phil")
		require.Nil(t, err)
		require.Equal(t, "phil", phil.Name)
		require.True(t, strings.HasPrefix(phil.Hash, "$2a$04$")) // Min cost for testing
		require.Equal(t, RoleAdmin, phil.Role)

		philGrants, err := a.Grants("phil")
		require.Nil(t, err)
		require.Equal(t, []Grant{}, philGrants)

		ben, err := a.User("ben")
		require.Nil(t, err)
		require.Equal(t, "ben", ben.Name)
		require.True(t, strings.HasPrefix(ben.Hash, "$2a$04$")) // Min cost for testing
		require.Equal(t, RoleUser, ben.Role)

		benGrants, err := a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, []Grant{
			{"everyonewrite", PermissionDenyAll, false},
			{"mytopic", PermissionReadWrite, false},
			{"writeme", PermissionWrite, false},
			{"readme", PermissionRead, false},
		}, benGrants)

		everyone, err := a.User(Everyone)
		require.Nil(t, err)
		require.Equal(t, "*", everyone.Name)
		require.Equal(t, "", everyone.Hash)
		require.Equal(t, RoleAnonymous, everyone.Role)

		everyoneGrants, err := a.Grants(Everyone)
		require.Nil(t, err)
		require.Equal(t, []Grant{
			{"everyonewrite", PermissionReadWrite, false},
			{"announcements", PermissionRead, false},
		}, everyoneGrants)

		// Ben: Before revoking
		require.Nil(t, a.AllowAccess("ben", "mytopic", PermissionReadWrite)) // Overwrite!
		require.Nil(t, a.AllowAccess("ben", "readme", PermissionRead))
		require.Nil(t, a.AllowAccess("ben", "writeme", PermissionWrite))
		require.Nil(t, a.Authorize(ben, "mytopic", PermissionRead))
		require.Nil(t, a.Authorize(ben, "mytopic", PermissionWrite))
		require.Nil(t, a.Authorize(ben, "readme", PermissionRead))
		require.Nil(t, a.Authorize(ben, "writeme", PermissionWrite))

		// Revoke access for "ben" to "mytopic", then check again
		require.Nil(t, a.ResetAccess("ben", "mytopic"))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "mytopic", PermissionWrite)) // Revoked
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "mytopic", PermissionRead))  // Revoked
		require.Nil(t, a.Authorize(ben, "readme", PermissionRead))                      // Unchanged
		require.Nil(t, a.Authorize(ben, "writeme", PermissionWrite))                    // Unchanged

		// Revoke rest of the access
		require.Nil(t, a.ResetAccess("ben", ""))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "readme", PermissionRead))    // Revoked
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, "wrtiteme", PermissionWrite)) // Revoked

		// User list
		users, err := a.Users()
		require.Nil(t, err)
		require.Equal(t, 3, len(users))
		require.Equal(t, "phil", users[0].Name)
		require.Equal(t, "ben", users[1].Name)
		require.Equal(t, "*", users[2].Name)

		// Remove user
		require.Nil(t, a.RemoveUser("ben"))
		_, err = a.User("ben")
		require.Equal(t, ErrUserNotFound, err)

		users, err = a.Users()
		require.Nil(t, err)
		require.Equal(t, 2, len(users))
		require.Equal(t, "phil", users[0].Name)
		require.Equal(t, "*", users[1].Name)
	})
}

func TestManager_ChangePassword(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleAdmin, false))
		require.Nil(t, a.AddUser("jane", "$2a$10$OyqU72muEy7VMd1SAU2Iru5IbeSMgrtCGHu/fWLmxL1MwlijQXWbG", RoleUser, true))

		_, err := a.Authenticate("phil", "phil")
		require.Nil(t, err)

		_, err = a.Authenticate("jane", "jane")
		require.Nil(t, err)

		require.Nil(t, a.ChangePassword("phil", "newpass", false))
		_, err = a.Authenticate("phil", "phil")
		require.Equal(t, ErrUnauthenticated, err)
		_, err = a.Authenticate("phil", "newpass")
		require.Nil(t, err)

		require.Nil(t, a.ChangePassword("jane", "$2a$10$CNaCW.q1R431urlbQ5Drh.zl48TiiOeJSmZgfcswkZiPbJGQ1ApSS", true))
		_, err = a.Authenticate("jane", "jane")
		require.Equal(t, ErrUnauthenticated, err)
		_, err = a.Authenticate("jane", "newpass")
		require.Nil(t, err)
	})
}

func TestManager_ChangeRole(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AllowAccess("ben", "mytopic", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("ben", "readme", PermissionRead))

		ben, err := a.User("ben")
		require.Nil(t, err)
		require.Equal(t, RoleUser, ben.Role)

		benGrants, err := a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, 2, len(benGrants))

		require.Nil(t, a.ChangeRole("ben", RoleAdmin))

		ben, err = a.User("ben")
		require.Nil(t, err)
		require.Equal(t, RoleAdmin, ben.Role)

		benGrants, err = a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, 0, len(benGrants))
	})
}

func TestManager_Reservations(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AddReservation("ben", "ztopic_", PermissionDenyAll, 0))
		require.Nil(t, a.AddReservation("ben", "readme", PermissionRead, 0))
		require.Nil(t, a.AllowAccess("ben", "something-else", PermissionRead))

		reservations, err := a.Reservations("ben")
		require.Nil(t, err)
		require.Equal(t, 2, len(reservations))
		require.Equal(t, Reservation{
			Topic:    "readme",
			Owner:    PermissionReadWrite,
			Everyone: PermissionRead,
		}, reservations[0])
		require.Equal(t, Reservation{
			Topic:    "ztopic_",
			Owner:    PermissionReadWrite,
			Everyone: PermissionDenyAll,
		}, reservations[1])

		b, err := a.HasReservation("ben", "readme")
		require.Nil(t, err)
		require.True(t, b)

		b, err = a.HasReservation("ben", "ztopic_")
		require.Nil(t, err)
		require.True(t, b)

		b, err = a.HasReservation("ben", "ztopicX") // _ != X (used to be a SQL wildcard issue)
		require.Nil(t, err)
		require.False(t, b)

		b, err = a.HasReservation("notben", "readme")
		require.Nil(t, err)
		require.False(t, b)

		b, err = a.HasReservation("ben", "something-else")
		require.Nil(t, err)
		require.False(t, b)

		count, err := a.ReservationsCount("ben")
		require.Nil(t, err)
		require.Equal(t, int64(2), count)

		count, err = a.ReservationsCount("phil")
		require.Nil(t, err)
		require.Equal(t, int64(0), count)

		err = a.AllowReservation("phil", "readme")
		require.Equal(t, errTopicOwnedByOthers, err)

		err = a.AllowReservation("phil", "ztopic_")
		require.Equal(t, errTopicOwnedByOthers, err)

		err = a.AllowReservation("phil", "ztopicX")
		require.Nil(t, err)

		err = a.AllowReservation("phil", "not-reserved")
		require.Nil(t, err)

		// Now remove them again
		require.Nil(t, a.RemoveReservations("ben", "ztopic_", "readme"))

		count, err = a.ReservationsCount("ben")
		require.Nil(t, err)
		require.Equal(t, int64(0), count)
	})
}

func TestManager_ChangeRoleFromTierUserToAdmin(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddTier(&Tier{
			Code:                     "pro",
			Name:                     "ntfy Pro",
			StripeMonthlyPriceID:     "price123",
			MessageLimit:             5_000,
			MessageExpiryDuration:    3 * 24 * time.Hour,
			EmailLimit:               50,
			ReservationLimit:         5,
			AttachmentFileSizeLimit:  52428800,
			AttachmentTotalSizeLimit: 524288000,
			AttachmentExpiryDuration: 24 * time.Hour,
		}))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.ChangeTier("ben", "pro"))
		require.Nil(t, a.AddReservation("ben", "mytopic", PermissionDenyAll, 0))

		ben, err := a.User("ben")
		require.Nil(t, err)
		require.Equal(t, RoleUser, ben.Role)
		require.Equal(t, "pro", ben.Tier.Code)
		require.Equal(t, int64(5000), ben.Tier.MessageLimit)
		require.Equal(t, 3*24*time.Hour, ben.Tier.MessageExpiryDuration)
		require.Equal(t, int64(50), ben.Tier.EmailLimit)
		require.Equal(t, int64(5), ben.Tier.ReservationLimit)
		require.Equal(t, int64(52428800), ben.Tier.AttachmentFileSizeLimit)
		require.Equal(t, int64(524288000), ben.Tier.AttachmentTotalSizeLimit)
		require.Equal(t, 24*time.Hour, ben.Tier.AttachmentExpiryDuration)

		benGrants, err := a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, 1, len(benGrants))
		require.Equal(t, PermissionReadWrite, benGrants[0].Permission)

		everyoneGrants, err := a.Grants(Everyone)
		require.Nil(t, err)
		require.Equal(t, 1, len(everyoneGrants))
		require.Equal(t, PermissionDenyAll, everyoneGrants[0].Permission)

		benReservations, err := a.Reservations("ben")
		require.Nil(t, err)
		require.Equal(t, 1, len(benReservations))
		require.Equal(t, "mytopic", benReservations[0].Topic)
		require.Equal(t, PermissionReadWrite, benReservations[0].Owner)
		require.Equal(t, PermissionDenyAll, benReservations[0].Everyone)

		// Switch to admin, this should remove all grants and owned ACL entries
		require.Nil(t, a.ChangeRole("ben", RoleAdmin))

		benGrants, err = a.Grants("ben")
		require.Nil(t, err)
		require.Equal(t, 0, len(benGrants))

		everyoneGrants, err = a.Grants(Everyone)
		require.Nil(t, err)
		require.Equal(t, 0, len(everyoneGrants))
	})
}

func TestManager_Token_Valid(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		u, err := a.User("ben")
		require.Nil(t, err)

		// Create token for user
		token, err := a.CreateToken(u.ID, "some label", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token.Value)
		require.Equal(t, "some label", token.Label)
		require.True(t, time.Now().Add(71*time.Hour).Unix() < token.Expires.Unix())

		u2, err := a.AuthenticateToken(token.Value)
		require.Nil(t, err)
		require.Equal(t, u.Name, u2.Name)
		require.Equal(t, token.Value, u2.Token)

		token2, err := a.Token(u.ID, token.Value)
		require.Nil(t, err)
		require.Equal(t, token.Value, token2.Value)
		require.Equal(t, "some label", token2.Label)

		tokens, err := a.Tokens(u.ID)
		require.Nil(t, err)
		require.Equal(t, 1, len(tokens))
		require.Equal(t, "some label", tokens[0].Label)

		tokens, err = a.Tokens("u_notauser")
		require.Nil(t, err)
		require.Equal(t, 0, len(tokens))

		// Remove token and auth again
		require.Nil(t, a.RemoveToken(u2.ID, u2.Token))
		u3, err := a.AuthenticateToken(token.Value)
		require.Equal(t, ErrUnauthenticated, err)
		require.Nil(t, u3)

		tokens, err = a.Tokens(u.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(tokens))
	})
}

func TestManager_Token_Invalid(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		u, err := a.AuthenticateToken(strings.Repeat("x", 32)) // 32 == token length
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)

		u, err = a.AuthenticateToken("not long enough anyway")
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)
	})
}

func TestManager_Token_NotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		_, err := a.Token("u_bla", "notfound")
		require.Equal(t, ErrTokenNotFound, err)
	})
}

func TestManager_Token_Expire(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		u, err := a.User("ben")
		require.Nil(t, err)

		// Create tokens for user
		token1, err := a.CreateToken(u.ID, "", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token1.Value)
		require.True(t, time.Now().Add(71*time.Hour).Unix() < token1.Expires.Unix())

		token2, err := a.CreateToken(u.ID, "", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token2.Value)
		require.NotEqual(t, token1.Value, token2.Value)
		require.True(t, time.Now().Add(71*time.Hour).Unix() < token2.Expires.Unix())

		// See that tokens work
		_, err = a.AuthenticateToken(token1.Value)
		require.Nil(t, err)

		_, err = a.AuthenticateToken(token2.Value)
		require.Nil(t, err)

		// Expire token1 via the API
		_, err = a.ChangeToken(u.ID, token1.Value, nil, util.Time(time.Unix(1, 0)))
		require.Nil(t, err)

		// Now token1 shouldn't work anymore
		_, err = a.AuthenticateToken(token1.Value)
		require.Equal(t, ErrUnauthenticated, err)

		// But the token row should still exist
		tokens, err := a.Tokens(u.ID)
		require.Nil(t, err)
		require.Equal(t, 2, len(tokens))

		// Expire tokens and check that token1 is gone
		require.Nil(t, a.RemoveExpiredTokens())

		tokens, err = a.Tokens(u.ID)
		require.Nil(t, err)
		require.Equal(t, 1, len(tokens))
		require.Equal(t, token2.Value, tokens[0].Value)
	})
}

func TestManager_Token_Extend(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		// Try to extend token for user without token
		u, err := a.User("ben")
		require.Nil(t, err)

		_, err = a.ChangeToken(u.ID, u.Token, util.String("some label"), util.Time(time.Now().Add(time.Hour)))
		require.Equal(t, errNoTokenProvided, err)

		// Create token for user
		token, err := a.CreateToken(u.ID, "", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token.Value)

		userWithToken, err := a.AuthenticateToken(token.Value)
		require.Nil(t, err)

		extendedToken, err := a.ChangeToken(userWithToken.ID, userWithToken.Token, util.String("changed label"), util.Time(time.Now().Add(100*time.Hour)))
		require.Nil(t, err)
		require.Equal(t, token.Value, extendedToken.Value)
		require.Equal(t, "changed label", extendedToken.Label)
		require.True(t, token.Expires.Unix() < extendedToken.Expires.Unix())
		require.True(t, time.Now().Add(99*time.Hour).Unix() < extendedToken.Expires.Unix())
	})
}

func TestManager_Token_MaxCount_AutoDelete(t *testing.T) {
	// Tests that tokens are automatically deleted when the maximum number of tokens is reached
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))

		ben, err := a.User("ben")
		require.Nil(t, err)

		phil, err := a.User("phil")
		require.Nil(t, err)

		// Create 2 tokens for phil
		philTokens := make([]string, 0)
		token, err := a.CreateToken(phil.ID, "", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token.Value)
		philTokens = append(philTokens, token.Value)

		token, err = a.CreateToken(phil.ID, "", time.Unix(0, 0), netip.IPv4Unspecified(), false)
		require.Nil(t, err)
		require.NotEmpty(t, token.Value)
		philTokens = append(philTokens, token.Value)

		// Create 62 tokens for ben (only 60 allowed!)
		baseTime := time.Now().Add(24 * time.Hour)
		benTokens := make([]string, 0)
		for i := 0; i < 62; i++ { //
			token, err := a.CreateToken(ben.ID, "", time.Now().Add(72*time.Hour), netip.IPv4Unspecified(), false)
			require.Nil(t, err)
			require.NotEmpty(t, token.Value)
			benTokens = append(benTokens, token.Value)

			// Manually modify expiry date to avoid sorting issues (this is a hack)
			_, err = a.ChangeToken(ben.ID, token.Value, nil, util.Time(baseTime.Add(time.Duration(i)*time.Minute)))
			require.Nil(t, err)
		}

		// Ben: The first 2 tokens should have been wiped and should not work anymore!
		_, err = a.AuthenticateToken(benTokens[0])
		require.Equal(t, ErrUnauthenticated, err)

		_, err = a.AuthenticateToken(benTokens[1])
		require.Equal(t, ErrUnauthenticated, err)

		// Ben: The other tokens should still work
		for i := 2; i < 62; i++ {
			userWithToken, err := a.AuthenticateToken(benTokens[i])
			require.Nil(t, err, "token[%d]=%s failed", i, benTokens[i])
			require.Equal(t, "ben", userWithToken.Name)
			require.Equal(t, benTokens[i], userWithToken.Token)
		}

		// Phil: All tokens should still work
		for i := 0; i < 2; i++ {
			userWithToken, err := a.AuthenticateToken(philTokens[i])
			require.Nil(t, err, "token[%d]=%s failed", i, philTokens[i])
			require.Equal(t, "phil", userWithToken.Name)
			require.Equal(t, philTokens[i], userWithToken.Token)
		}

		benTokensList, err := a.Tokens(ben.ID)
		require.Nil(t, err)
		require.Equal(t, 60, len(benTokensList))

		philTokensList, err := a.Tokens(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 2, len(philTokensList))
	})
}

func TestManager_EnqueueStats_ResetStats(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:       PermissionReadWrite,
			BcryptCost:          bcrypt.MinCost,
			QueueWriterInterval: 1500 * time.Millisecond,
		}
		a := newTestManagerFromConfig(t, newManager, conf)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		// Baseline: No messages or emails
		u, err := a.User("ben")
		require.Nil(t, err)
		require.Equal(t, int64(0), u.Stats.Messages)
		require.Equal(t, int64(0), u.Stats.Emails)
		a.EnqueueUserStats(u.ID, &Stats{
			Messages: 11,
			Emails:   2,
		})

		// Still no change, because it's queued asynchronously
		u, err = a.User("ben")
		require.Nil(t, err)
		require.Equal(t, int64(0), u.Stats.Messages)
		require.Equal(t, int64(0), u.Stats.Emails)

		// After 2 seconds they should be persisted
		time.Sleep(2 * time.Second)

		u, err = a.User("ben")
		require.Nil(t, err)
		require.Equal(t, int64(11), u.Stats.Messages)
		require.Equal(t, int64(2), u.Stats.Emails)

		// Now reset stats (enqueued stats will be thrown out)
		a.EnqueueUserStats(u.ID, &Stats{
			Messages: 99,
			Emails:   23,
		})
		require.Nil(t, a.ResetStats())

		u, err = a.User("ben")
		require.Nil(t, err)
		require.Equal(t, int64(0), u.Stats.Messages)
		require.Equal(t, int64(0), u.Stats.Emails)
	})
}

func TestManager_EnqueueTokenUpdate(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:       PermissionReadWrite,
			BcryptCost:          bcrypt.MinCost,
			QueueWriterInterval: 500 * time.Millisecond,
		}
		a := newTestManagerFromConfig(t, newManager, conf)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		// Create user and token
		u, err := a.User("ben")
		require.Nil(t, err)

		token, err := a.CreateToken(u.ID, "", time.Now().Add(time.Hour), netip.IPv4Unspecified(), false)
		require.Nil(t, err)

		// Queue token update
		a.EnqueueTokenUpdate(token.Value, &TokenUpdate{
			LastAccess: time.Unix(111, 0).UTC(),
			LastOrigin: netip.MustParseAddr("1.2.3.3"),
		})

		// Token has not changed yet.
		token2, err := a.Token(u.ID, token.Value)
		require.Nil(t, err)
		require.Equal(t, token.LastAccess.Unix(), token2.LastAccess.Unix())
		require.Equal(t, token.LastOrigin, token2.LastOrigin)

		// After a second or so they should be persisted
		time.Sleep(time.Second)

		token3, err := a.Token(u.ID, token.Value)
		require.Nil(t, err)
		require.Equal(t, time.Unix(111, 0).UTC().Unix(), token3.LastAccess.Unix())
		require.Equal(t, netip.MustParseAddr("1.2.3.3"), token3.LastOrigin)
	})
}

func TestManager_ChangeSettings(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:       PermissionReadWrite,
			BcryptCost:          bcrypt.MinCost,
			QueueWriterInterval: 1500 * time.Millisecond,
		}
		a := newTestManagerFromConfig(t, newManager, conf)
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))

		// No settings
		u, err := a.User("ben")
		require.Nil(t, err)
		require.Nil(t, u.Prefs.Subscriptions)
		require.Nil(t, u.Prefs.Notification)
		require.Nil(t, u.Prefs.Language)

		// Save with new settings
		prefs := &Prefs{
			Language: util.String("de"),
			Notification: &NotificationPrefs{
				Sound:       util.String("ding"),
				MinPriority: util.Int(2),
			},
			Subscriptions: []*Subscription{
				{
					BaseURL:     "https://ntfy.sh",
					Topic:       "mytopic",
					DisplayName: util.String("My Topic"),
				},
			},
		}
		require.Nil(t, a.ChangeSettings(u.ID, prefs))

		// Read again
		u, err = a.User("ben")
		require.Nil(t, err)
		require.Equal(t, util.String("de"), u.Prefs.Language)
		require.Equal(t, util.String("ding"), u.Prefs.Notification.Sound)
		require.Equal(t, util.Int(2), u.Prefs.Notification.MinPriority)
		require.Nil(t, u.Prefs.Notification.DeleteAfter)
		require.Equal(t, "https://ntfy.sh", u.Prefs.Subscriptions[0].BaseURL)
		require.Equal(t, "mytopic", u.Prefs.Subscriptions[0].Topic)
		require.Equal(t, util.String("My Topic"), u.Prefs.Subscriptions[0].DisplayName)
	})
}

func TestManager_Tier_Create_Update_List_Delete(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		// Create tier and user
		require.Nil(t, a.AddTier(&Tier{
			Code:                     "supporter",
			Name:                     "Supporter",
			MessageLimit:             1,
			MessageExpiryDuration:    time.Second,
			EmailLimit:               1,
			ReservationLimit:         1,
			AttachmentFileSizeLimit:  1,
			AttachmentTotalSizeLimit: 1,
			AttachmentExpiryDuration: time.Second,
			AttachmentBandwidthLimit: 1,
			StripeMonthlyPriceID:     "price_1",
		}))
		require.Nil(t, a.AddTier(&Tier{
			Code:                     "pro",
			Name:                     "Pro",
			MessageLimit:             123,
			MessageExpiryDuration:    86400 * time.Second,
			EmailLimit:               32,
			ReservationLimit:         2,
			AttachmentFileSizeLimit:  1231231,
			AttachmentTotalSizeLimit: 123123,
			AttachmentExpiryDuration: 10800 * time.Second,
			AttachmentBandwidthLimit: 21474836480,
			StripeMonthlyPriceID:     "price_2",
		}))
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.ChangeTier("phil", "pro"))

		ti, err := a.Tier("pro")
		require.Nil(t, err)

		u, err := a.User("phil")
		require.Nil(t, err)

		// These are populated by different SQL queries
		require.Equal(t, ti, u.Tier)

		// Fields
		require.True(t, strings.HasPrefix(ti.ID, "ti_"))
		require.Equal(t, "pro", ti.Code)
		require.Equal(t, "Pro", ti.Name)
		require.Equal(t, int64(123), ti.MessageLimit)
		require.Equal(t, 86400*time.Second, ti.MessageExpiryDuration)
		require.Equal(t, int64(32), ti.EmailLimit)
		require.Equal(t, int64(2), ti.ReservationLimit)
		require.Equal(t, int64(1231231), ti.AttachmentFileSizeLimit)
		require.Equal(t, int64(123123), ti.AttachmentTotalSizeLimit)
		require.Equal(t, 10800*time.Second, ti.AttachmentExpiryDuration)
		require.Equal(t, int64(21474836480), ti.AttachmentBandwidthLimit)
		require.Equal(t, "price_2", ti.StripeMonthlyPriceID)

		// Update tier
		ti.EmailLimit = 999999
		require.Nil(t, a.UpdateTier(ti))

		// List tiers
		tiers, err := a.Tiers()
		require.Nil(t, err)
		require.Equal(t, 2, len(tiers))

		ti = tiers[0]
		require.Equal(t, "supporter", ti.Code)
		require.Equal(t, "Supporter", ti.Name)
		require.Equal(t, int64(1), ti.MessageLimit)
		require.Equal(t, time.Second, ti.MessageExpiryDuration)
		require.Equal(t, int64(1), ti.EmailLimit)
		require.Equal(t, int64(1), ti.ReservationLimit)
		require.Equal(t, int64(1), ti.AttachmentFileSizeLimit)
		require.Equal(t, int64(1), ti.AttachmentTotalSizeLimit)
		require.Equal(t, time.Second, ti.AttachmentExpiryDuration)
		require.Equal(t, int64(1), ti.AttachmentBandwidthLimit)
		require.Equal(t, "price_1", ti.StripeMonthlyPriceID)

		ti = tiers[1]
		require.Equal(t, "pro", ti.Code)
		require.Equal(t, "Pro", ti.Name)
		require.Equal(t, int64(123), ti.MessageLimit)
		require.Equal(t, 86400*time.Second, ti.MessageExpiryDuration)
		require.Equal(t, int64(999999), ti.EmailLimit) // Updatedd!
		require.Equal(t, int64(2), ti.ReservationLimit)
		require.Equal(t, int64(1231231), ti.AttachmentFileSizeLimit)
		require.Equal(t, int64(123123), ti.AttachmentTotalSizeLimit)
		require.Equal(t, 10800*time.Second, ti.AttachmentExpiryDuration)
		require.Equal(t, int64(21474836480), ti.AttachmentBandwidthLimit)
		require.Equal(t, "price_2", ti.StripeMonthlyPriceID)

		ti, err = a.TierByStripePrice("price_1")
		require.Nil(t, err)
		require.Equal(t, "supporter", ti.Code)
		require.Equal(t, "Supporter", ti.Name)
		require.Equal(t, int64(1), ti.MessageLimit)
		require.Equal(t, time.Second, ti.MessageExpiryDuration)
		require.Equal(t, int64(1), ti.EmailLimit)
		require.Equal(t, int64(1), ti.ReservationLimit)
		require.Equal(t, int64(1), ti.AttachmentFileSizeLimit)
		require.Equal(t, int64(1), ti.AttachmentTotalSizeLimit)
		require.Equal(t, time.Second, ti.AttachmentExpiryDuration)
		require.Equal(t, int64(1), ti.AttachmentBandwidthLimit)
		require.Equal(t, "price_1", ti.StripeMonthlyPriceID)

		// Cannot remove tier, since user has this tier
		require.Error(t, a.RemoveTier("pro"))

		// CAN remove this tier
		require.Nil(t, a.RemoveTier("supporter"))

		tiers, err = a.Tiers()
		require.Nil(t, err)
		require.Equal(t, 1, len(tiers))
		require.Equal(t, "pro", tiers[0].Code)
		require.Equal(t, "pro", tiers[0].Code)
	})
}

func TestAccount_Tier_Create_With_ID(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddTier(&Tier{
			ID:   "ti_123",
			Code: "pro",
		}))

		ti, err := a.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "ti_123", ti.ID)
	})
}

func TestManager_Tier_Change_And_Reset(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		// Create tier and user
		require.Nil(t, a.AddTier(&Tier{
			Code:             "supporter",
			Name:             "Supporter",
			ReservationLimit: 3,
		}))
		require.Nil(t, a.AddTier(&Tier{
			Code:             "pro",
			Name:             "Pro",
			ReservationLimit: 4,
		}))
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.ChangeTier("phil", "pro"))

		// Add 10 reservations (pro tier allows that)
		for i := 0; i < 4; i++ {
			require.Nil(t, a.AddReservation("phil", fmt.Sprintf("topic%d", i), PermissionWrite, 0))
		}

		// Downgrading will not work (too many reservations)
		require.Equal(t, ErrTooManyReservations, a.ChangeTier("phil", "supporter"))

		// Downgrade after removing a reservation
		require.Nil(t, a.RemoveReservations("phil", "topic0"))
		require.Nil(t, a.ChangeTier("phil", "supporter"))

		// Resetting will not work (too many reservations)
		require.Equal(t, ErrTooManyReservations, a.ResetTier("phil"))

		// Resetting after removing all reservations
		require.Nil(t, a.RemoveReservations("phil", "topic1", "topic2", "topic3"))
		require.Nil(t, a.ResetTier("phil"))
	})
}

func TestUser_PhoneNumberAddListRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		require.Nil(t, a.AddPhoneNumber(phil.ID, "+1234567890"))

		phoneNumbers, err := a.PhoneNumbers(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 1, len(phoneNumbers))
		require.Equal(t, "+1234567890", phoneNumbers[0])

		require.Nil(t, a.RemovePhoneNumber(phil.ID, "+1234567890"))
		phoneNumbers, err = a.PhoneNumbers(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(phoneNumbers))

		// Paranoia check: We do NOT want to keep phone numbers in there
		rows, err := testDB(a).Query(`SELECT * FROM user_phone`)
		require.Nil(t, err)
		require.False(t, rows.Next())
		require.Nil(t, rows.Close())
	})
}

func TestUser_PhoneNumberAdd_Multiple_Users_Same_Number(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		ben, err := a.User("ben")
		require.Nil(t, err)
		require.Nil(t, a.AddPhoneNumber(phil.ID, "+1234567890"))
		require.Nil(t, a.AddPhoneNumber(ben.ID, "+1234567890"))
	})
}

func TestUser_EmailAddListRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		require.Nil(t, a.AddEmail(phil.ID, "phil@example.com"))

		emails, err := a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 1, len(emails))
		require.Equal(t, "phil@example.com", emails[0])

		require.Nil(t, a.RemoveEmail(phil.ID, "phil@example.com"))
		emails, err = a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(emails))

		// Paranoia check: We do NOT want to keep emails in there
		rows, err := testDB(a).Query(`SELECT * FROM user_email`)
		require.Nil(t, err)
		require.False(t, rows.Next())
		require.Nil(t, rows.Close())
	})
}

func TestUser_EmailAdd_Multiple_Users_Same_Email(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		ben, err := a.User("ben")
		require.Nil(t, err)
		require.Nil(t, a.AddEmail(phil.ID, "shared@example.com"))
		require.Nil(t, a.AddEmail(ben.ID, "shared@example.com"))
	})
}

func TestUser_EmailAdd_Duplicate(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		require.Nil(t, a.AddEmail(phil.ID, "phil@example.com"))
		require.ErrorIs(t, a.AddEmail(phil.ID, "phil@example.com"), ErrEmailExists)
	})
}

func TestManager_Topic_Wildcard_With_Asterisk_Underscore(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AllowAccess(Everyone, "*_", PermissionRead))
		require.Nil(t, a.AllowAccess(Everyone, "__*_", PermissionRead))
		require.Nil(t, a.Authorize(nil, "allowed_", PermissionRead))
		require.Nil(t, a.Authorize(nil, "__allowed_", PermissionRead))
		require.Nil(t, a.Authorize(nil, "_allowed_", PermissionRead)) // The "%" in "%\_" matches the first "_"
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "notallowed", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "_notallowed", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "__notallowed", PermissionRead))
	})
}

func TestManager_Topic_Wildcard_With_Underscore(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AllowAccess(Everyone, "mytopic_", PermissionReadWrite))
		require.Nil(t, a.Authorize(nil, "mytopic_", PermissionRead))
		require.Nil(t, a.Authorize(nil, "mytopic_", PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "mytopicX", PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(nil, "mytopicX", PermissionWrite))
	})
}

func TestManager_WithProvisionedUsers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:    PermissionReadWrite,
			ProvisionEnabled: true,
			Users: []*User{
				{Name: "philuser", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
				{Name: "philadmin", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleAdmin},
			},
			Access: map[string][]*Grant{
				"philuser": {
					{TopicPattern: "stats", Permission: PermissionReadWrite},
					{TopicPattern: "secret", Permission: PermissionRead},
				},
			},
			Tokens: map[string][]*Token{
				"philuser": {
					{Value: "tk_op56p8lz5bf3cxkz9je99v9oc37lo", Label: "Alerts token"},
				},
			},
		}
		a := newTestManagerFromConfig(t, newManager, conf)

		// Manually add user
		require.Nil(t, a.AddUser("philmanual", "manual", RoleUser, false))

		// Check that the provisioned users are there
		users, err := a.Users()
		require.Nil(t, err)
		require.Len(t, users, 4)
		require.Equal(t, "philadmin", users[0].Name)
		require.Equal(t, RoleAdmin, users[0].Role)
		require.Equal(t, "philmanual", users[1].Name)
		require.Equal(t, RoleUser, users[1].Role)
		require.Equal(t, "philuser", users[2].Name)
		require.Equal(t, RoleUser, users[2].Role)
		require.Equal(t, "*", users[3].Name)
		provisionedUserID := users[2].ID // "philuser" is the provisioned user

		grants, err := a.Grants("philuser")
		require.Nil(t, err)
		require.Equal(t, 2, len(grants))
		require.Equal(t, "secret", grants[0].TopicPattern)
		require.Equal(t, PermissionRead, grants[0].Permission)
		require.Equal(t, "stats", grants[1].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[1].Permission)

		tokens, err := a.Tokens(provisionedUserID)
		require.Nil(t, err)
		require.Equal(t, 1, len(tokens))
		require.Equal(t, "tk_op56p8lz5bf3cxkz9je99v9oc37lo", tokens[0].Value)
		require.Equal(t, "Alerts token", tokens[0].Label)
		require.True(t, tokens[0].Provisioned)

		// Update the token last access time and origin (so we can check that it is persisted)
		lastAccessTime := time.Now().Add(time.Hour)
		lastOrigin := netip.MustParseAddr("1.1.9.9")
		a.EnqueueTokenUpdate(tokens[0].Value, &TokenUpdate{LastAccess: lastAccessTime, LastOrigin: lastOrigin})
		err = a.writeTokenUpdateQueue()
		require.Nil(t, err)

		// Re-open the DB (second app start)
		require.Nil(t, a.Close())
		conf.Users = []*User{
			{Name: "philuser", Hash: "$2a$10$AAAAU21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
		}
		conf.Access = map[string][]*Grant{
			"philuser": {
				{TopicPattern: "stats12", Permission: PermissionReadWrite},
				{TopicPattern: "secret12", Permission: PermissionRead},
			},
		}
		conf.Tokens = map[string][]*Token{
			"philuser": {
				{Value: "tk_op56p8lz5bf3cxkz9je99v9oc37lo", Label: "Alerts token updated"},
				{Value: "tk_u48wqendnkx9er21pqqcadlytbutx", Label: "Another token"},
			},
		}
		a = newTestManagerFromConfig(t, newManager, conf)

		// Check that the provisioned users are there
		users, err = a.Users()
		require.Nil(t, err)
		require.Len(t, users, 3)
		require.Equal(t, "philmanual", users[0].Name)
		require.Equal(t, "philuser", users[1].Name)
		require.Equal(t, RoleUser, users[1].Role)
		require.Equal(t, RoleUser, users[0].Role)
		require.Equal(t, "*", users[2].Name)

		grants, err = a.Grants("philuser")
		require.Nil(t, err)
		require.Equal(t, 2, len(grants))
		require.Equal(t, "secret12", grants[0].TopicPattern)
		require.Equal(t, PermissionRead, grants[0].Permission)
		require.Equal(t, "stats12", grants[1].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[1].Permission)

		tokens, err = a.Tokens(provisionedUserID)
		require.Nil(t, err)
		require.Equal(t, 2, len(tokens))
		require.Equal(t, "tk_op56p8lz5bf3cxkz9je99v9oc37lo", tokens[0].Value)
		require.Equal(t, "Alerts token updated", tokens[0].Label)
		require.Equal(t, lastAccessTime.Unix(), tokens[0].LastAccess.Unix())
		require.Equal(t, lastOrigin, tokens[0].LastOrigin)
		require.True(t, tokens[0].Provisioned)
		require.Equal(t, "tk_u48wqendnkx9er21pqqcadlytbutx", tokens[1].Value)
		require.Equal(t, "Another token", tokens[1].Label)

		// Try changing provisioned user's password
		require.Error(t, a.ChangePassword("philuser", "new-pass", false))

		// Re-open the DB again (third app start)
		require.Nil(t, a.Close())
		conf.Users = []*User{}
		conf.Access = map[string][]*Grant{}
		conf.Tokens = map[string][]*Token{}
		a = newTestManagerFromConfig(t, newManager, conf)

		// Check that the provisioned users are all gone
		users, err = a.Users()
		require.Nil(t, err)
		require.Len(t, users, 2)

		require.Equal(t, "philmanual", users[0].Name)
		require.Equal(t, RoleUser, users[0].Role)
		require.Equal(t, "*", users[1].Name)

		grants, err = a.Grants("philuser")
		require.Nil(t, err)
		require.Equal(t, 0, len(grants))

		tokens, err = a.Tokens(provisionedUserID)
		require.Nil(t, err)
		require.Equal(t, 0, len(tokens))

		// Verify no provisioned data remains
		for _, u := range users {
			require.False(t, u.Provisioned)
			userGrants, err := a.Grants(u.Name)
			require.Nil(t, err)
			for _, g := range userGrants {
				require.False(t, g.Provisioned)
			}
			userTokens, err := a.Tokens(u.ID)
			require.Nil(t, err)
			for _, tk := range userTokens {
				require.False(t, tk.Provisioned)
			}
		}
	})
}

func TestManager_WithProvisionedUsers_RemoveToken(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:    PermissionReadWrite,
			ProvisionEnabled: true,
			Users: []*User{
				{Name: "phil", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
			},
			Tokens: map[string][]*Token{
				"phil": {
					{Value: "tk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "Token A"},
					{Value: "tk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "Token B"},
				},
			},
		}
		a := newTestManagerFromConfig(t, newManager, conf)

		users, err := a.Users()
		require.Nil(t, err)
		philUserID := ""
		for _, u := range users {
			if u.Name == "phil" {
				philUserID = u.ID
			}
		}
		require.NotEmpty(t, philUserID)

		tokens, err := a.Tokens(philUserID)
		require.Nil(t, err)
		require.Equal(t, 2, len(tokens))

		// Re-open the DB: user stays, but Token B is removed from config
		require.Nil(t, a.Close())
		conf.Tokens = map[string][]*Token{
			"phil": {
				{Value: "tk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Label: "Token A"},
			},
		}
		a = newTestManagerFromConfig(t, newManager, conf)

		tokens, err = a.Tokens(philUserID)
		require.Nil(t, err)
		require.Equal(t, 1, len(tokens))
		require.Equal(t, "tk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", tokens[0].Value)
	})
}

func TestManager_UpdateNonProvisionedUsersToProvisionedUsers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		conf := &Config{
			DefaultAccess:    PermissionReadWrite,
			ProvisionEnabled: true,
			Users:            []*User{},
			Access: map[string][]*Grant{
				Everyone: {
					{TopicPattern: "food", Permission: PermissionRead},
				},
			},
		}
		a := newTestManagerFromConfig(t, newManager, conf)

		// Manually add user
		require.Nil(t, a.AddUser("philuser", "manual", RoleUser, false))
		require.Nil(t, a.AllowAccess("philuser", "stats", PermissionReadWrite))
		require.Nil(t, a.AllowAccess("philuser", "food", PermissionReadWrite))

		users, err := a.Users()
		require.Nil(t, err)
		require.Len(t, users, 2)
		require.Equal(t, "philuser", users[0].Name)
		require.Equal(t, RoleUser, users[0].Role)
		require.False(t, users[0].Provisioned) // Manually added

		grants, err := a.Grants("philuser")
		require.Nil(t, err)
		require.Equal(t, 2, len(grants))
		require.Equal(t, "stats", grants[0].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[0].Permission)
		require.False(t, grants[0].Provisioned) // Manually added
		require.Equal(t, "food", grants[1].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[1].Permission)
		require.False(t, grants[1].Provisioned) // Manually added

		grants, err = a.Grants(Everyone)
		require.Nil(t, err)
		require.Equal(t, 1, len(grants))
		require.Equal(t, "food", grants[0].TopicPattern)
		require.Equal(t, PermissionRead, grants[0].Permission)
		require.True(t, grants[0].Provisioned) // Provisioned entry

		// Re-open the DB (second app start)
		require.Nil(t, a.Close())
		conf.Users = []*User{
			{Name: "philuser", Hash: "$2a$10$AAAAU21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
		}
		conf.Access = map[string][]*Grant{
			"philuser": {
				{TopicPattern: "stats", Permission: PermissionReadWrite},
			},
		}
		a = newTestManagerFromConfig(t, newManager, conf)

		// Check that the user was "upgraded" to a provisioned user
		users, err = a.Users()
		require.Nil(t, err)
		require.Len(t, users, 2)
		require.Equal(t, "philuser", users[0].Name)
		require.Equal(t, RoleUser, users[0].Role)
		require.Equal(t, "$2a$10$AAAAU21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", users[0].Hash)
		require.True(t, users[0].Provisioned) // Updated to provisioned!

		grants, err = a.Grants("philuser")
		require.Nil(t, err)
		require.Equal(t, 2, len(grants))
		require.Equal(t, "stats", grants[0].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[0].Permission)
		require.True(t, grants[0].Provisioned) // Updated to provisioned!
		require.Equal(t, "food", grants[1].TopicPattern)
		require.Equal(t, PermissionReadWrite, grants[1].Permission)
		require.False(t, grants[1].Provisioned) // Manually added grants stay!

		grants, err = a.Grants(Everyone)
		require.Nil(t, err)
		require.Empty(t, grants)
	})
}

func TestManager_RemoveProvisionedOnEmptyConfig(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		// Start with provisioned users, access, and tokens
		conf := &Config{
			DefaultAccess:    PermissionReadWrite,
			ProvisionEnabled: true,
			BcryptCost:       bcrypt.MinCost,
			Users: []*User{
				{Name: "provuser", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
			},
			Access: map[string][]*Grant{
				"provuser": {
					{TopicPattern: "stats", Permission: PermissionReadWrite},
				},
			},
			Tokens: map[string][]*Token{
				"provuser": {
					{Value: "tk_op56p8lz5bf3cxkz9je99v9oc37lo", Label: "Provisioned token"},
				},
			},
		}
		a := newTestManagerFromConfig(t, newManager, conf)

		// Also add a manual (non-provisioned) user
		require.Nil(t, a.AddUser("manualuser", "manual", RoleUser, false))

		// Verify initial state
		users, err := a.Users()
		require.Nil(t, err)
		require.Len(t, users, 3) // provuser, manualuser, everyone

		// Re-open with empty provisioning config (simulates config change)
		require.Nil(t, a.Close())
		conf.Users = nil
		conf.Access = nil
		conf.Tokens = nil
		a = newTestManagerFromConfig(t, newManager, conf)

		// Provisioned user should be removed, manual user should remain
		users, err = a.Users()
		require.Nil(t, err)
		require.Len(t, users, 2)
		require.Equal(t, "manualuser", users[0].Name)
		require.False(t, users[0].Provisioned)
		require.Equal(t, "*", users[1].Name) // everyone
	})
}

func TestToFromSQLWildcard(t *testing.T) {
	require.Equal(t, "up%", toSQLWildcard("up*"))
	require.Equal(t, "up\\_%", toSQLWildcard("up_*"))
	require.Equal(t, "foo", toSQLWildcard("foo"))

	require.Equal(t, "up*", fromSQLWildcard("up%"))
	require.Equal(t, "up_*", fromSQLWildcard("up\\_%"))
	require.Equal(t, "foo", fromSQLWildcard("foo"))

	require.Equal(t, "up*", fromSQLWildcard(toSQLWildcard("up*")))
	require.Equal(t, "up_*", fromSQLWildcard(toSQLWildcard("up_*")))
	require.Equal(t, "foo", fromSQLWildcard(toSQLWildcard("foo")))
}

func TestMigrationFrom1(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "user.db")
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)

	// Create "version 1" schema
	_, err = db.Exec(`
		BEGIN;
		CREATE TABLE IF NOT EXISTS user (
			user TEXT NOT NULL PRIMARY KEY,
			pass TEXT NOT NULL,
			role TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS access (
			user TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			PRIMARY KEY (topic, user)
		);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO schemaVersion (id, version) VALUES (1, 1);
		COMMIT;
	`)
	require.Nil(t, err)

	// Insert a bunch of users and ACL entries
	_, err = db.Exec(`
		BEGIN;
		INSERT INTO user (user, pass, role) VALUES ('ben', '$2a$10$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy', 'user');
		INSERT INTO user (user, pass, role) VALUES ('phil', '$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C', 'admin');
		INSERT INTO access (user, topic, read, write) VALUES ('ben', 'stats', 1, 1);
		INSERT INTO access (user, topic, read, write) VALUES ('ben', 'secret', 1, 0);
		INSERT INTO access (user, topic, read, write) VALUES ('*', 'stats', 1, 0);
		COMMIT;
	`)
	require.Nil(t, err)

	// Create manager to trigger migration
	a := newTestManagerFromFile(t, filename, "", PermissionDenyAll, bcrypt.MinCost, DefaultUserStatsQueueWriterInterval)
	checkSchemaVersion(t, testDB(a))

	users, err := a.Users()
	require.Nil(t, err)
	require.Equal(t, 3, len(users))
	phil, ben, everyone := users[0], users[1], users[2]

	philGrants, err := a.Grants("phil")
	require.Nil(t, err)

	benGrants, err := a.Grants("ben")
	require.Nil(t, err)

	everyoneGrants, err := a.Grants(Everyone)
	require.Nil(t, err)

	require.True(t, strings.HasPrefix(phil.ID, "u_"))
	require.Equal(t, "phil", phil.Name)
	require.Equal(t, RoleAdmin, phil.Role)
	require.Equal(t, syncTopicLength, len(phil.SyncTopic))
	require.Equal(t, 0, len(philGrants))

	require.True(t, strings.HasPrefix(ben.ID, "u_"))
	require.NotEqual(t, phil.ID, ben.ID)
	require.Equal(t, "ben", ben.Name)
	require.Equal(t, RoleUser, ben.Role)
	require.Equal(t, syncTopicLength, len(ben.SyncTopic))
	require.NotEqual(t, ben.SyncTopic, phil.SyncTopic)
	require.Equal(t, 2, len(benGrants))
	require.Equal(t, "secret", benGrants[0].TopicPattern)
	require.Equal(t, PermissionRead, benGrants[0].Permission)
	require.Equal(t, "stats", benGrants[1].TopicPattern)
	require.Equal(t, PermissionReadWrite, benGrants[1].Permission)

	require.Equal(t, "u_everyone", everyone.ID)
	require.Equal(t, Everyone, everyone.Name)
	require.Equal(t, RoleAnonymous, everyone.Role)
	require.Equal(t, 1, len(everyoneGrants))
	require.Equal(t, "stats", everyoneGrants[0].TopicPattern)
	require.Equal(t, PermissionRead, everyoneGrants[0].Permission)
}

func TestMigrationFrom4(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "user.db")
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)

	// Create "version 4" schema
	_, err = db.Exec(`
		BEGIN;
		CREATE TABLE IF NOT EXISTS tier (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL,
			messages_limit INT NOT NULL,
			messages_expiry_duration INT NOT NULL,
			emails_limit INT NOT NULL,
			calls_limit INT NOT NULL,
			reservations_limit INT NOT NULL,
			attachment_file_size_limit INT NOT NULL,
			attachment_total_size_limit INT NOT NULL,
			attachment_expiry_duration INT NOT NULL,
			attachment_bandwidth_limit INT NOT NULL,
			stripe_monthly_price_id TEXT,
			stripe_yearly_price_id TEXT
		);
		CREATE UNIQUE INDEX idx_tier_code ON tier (code);
		CREATE UNIQUE INDEX idx_tier_stripe_monthly_price_id ON tier (stripe_monthly_price_id);
		CREATE UNIQUE INDEX idx_tier_stripe_yearly_price_id ON tier (stripe_yearly_price_id);
		CREATE TABLE IF NOT EXISTS user (
		    id TEXT PRIMARY KEY,
			tier_id TEXT,
			user TEXT NOT NULL,
			pass TEXT NOT NULL,
			role TEXT CHECK (role IN ('anonymous', 'admin', 'user')) NOT NULL,
			prefs JSON NOT NULL DEFAULT '{}',
			sync_topic TEXT NOT NULL,
			stats_messages INT NOT NULL DEFAULT (0),
			stats_emails INT NOT NULL DEFAULT (0),
			stats_calls INT NOT NULL DEFAULT (0),
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			stripe_subscription_status TEXT,
			stripe_subscription_interval TEXT,
			stripe_subscription_paid_until INT,
			stripe_subscription_cancel_at INT,
			created INT NOT NULL,
			deleted INT,
		    FOREIGN KEY (tier_id) REFERENCES tier (id)
		);
		CREATE UNIQUE INDEX idx_user ON user (user);
		CREATE UNIQUE INDEX idx_user_stripe_customer_id ON user (stripe_customer_id);
		CREATE UNIQUE INDEX idx_user_stripe_subscription_id ON user (stripe_subscription_id);
		CREATE TABLE IF NOT EXISTS user_access (
			user_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			owner_user_id INT,
			PRIMARY KEY (user_id, topic),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE,
		    FOREIGN KEY (owner_user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_token (
			user_id TEXT NOT NULL,
			token TEXT NOT NULL,
			label TEXT NOT NULL,
			last_access INT NOT NULL,
			last_origin TEXT NOT NULL,
			expires INT NOT NULL,
			PRIMARY KEY (user_id, token),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_phone (
			user_id TEXT NOT NULL,
			phone_number TEXT NOT NULL,
			PRIMARY KEY (user_id, phone_number),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO user (id, user, pass, role, sync_topic, created)
		VALUES ('u_everyone', '*', '', 'anonymous', '', UNIXEPOCH())
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO schemaVersion (id, version) VALUES (1, 4);
		COMMIT;
	`)
	require.Nil(t, err)

	// Insert a few ACL entries
	_, err = db.Exec(`
		BEGIN;
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'mytopic_', 1, 1);
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'up%', 1, 1);
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'down_%', 1, 1);
		COMMIT;
	`)
	require.Nil(t, err)

	// Create manager to trigger migration
	a := newTestManagerFromFile(t, filename, "", PermissionDenyAll, bcrypt.MinCost, DefaultUserStatsQueueWriterInterval)
	checkSchemaVersion(t, testDB(a))

	// Add another
	require.Nil(t, a.AllowAccess(Everyone, "left_*", PermissionReadWrite))

	// Check "external view" of grants
	everyoneGrants, err := a.Grants(Everyone)
	require.Nil(t, err)

	require.Equal(t, 4, len(everyoneGrants))
	require.Equal(t, "mytopic_", everyoneGrants[0].TopicPattern)
	require.Equal(t, "down_*", everyoneGrants[1].TopicPattern)
	require.Equal(t, "left_*", everyoneGrants[2].TopicPattern)
	require.Equal(t, "up*", everyoneGrants[3].TopicPattern)

	// Check they are stored correctly in the database
	rows, err := db.Query(`SELECT topic FROM user_access WHERE user_id = 'u_everyone' ORDER BY topic`)
	require.Nil(t, err)
	topicPatterns := make([]string, 0)
	for rows.Next() {
		var topicPattern string
		require.Nil(t, rows.Scan(&topicPattern))
		topicPatterns = append(topicPatterns, topicPattern)
	}
	require.Nil(t, rows.Close())
	require.Equal(t, 4, len(topicPatterns))
	require.Equal(t, "down\\_%", topicPatterns[0])
	require.Equal(t, "left\\_%", topicPatterns[1])
	require.Equal(t, "mytopic\\_", topicPatterns[2])
	require.Equal(t, "up%", topicPatterns[3])

	// Check that ACL works as excepted
	require.Nil(t, a.Authorize(nil, "down_123", PermissionRead))
	require.Equal(t, ErrUnauthorized, a.Authorize(nil, "downX123", PermissionRead))

	require.Nil(t, a.Authorize(nil, "left_abc", PermissionRead))
	require.Equal(t, ErrUnauthorized, a.Authorize(nil, "leftX123", PermissionRead))

	require.Nil(t, a.Authorize(nil, "mytopic_", PermissionRead))
	require.Equal(t, ErrUnauthorized, a.Authorize(nil, "mytopicX", PermissionRead))

	require.Nil(t, a.Authorize(nil, "up123", PermissionRead))
	require.Nil(t, a.Authorize(nil, "up", PermissionRead)) // % matches 0 or more characters
}

func checkSchemaVersion(t *testing.T, d *db.DB) {
	rows, err := d.Query(`SELECT version FROM schemaVersion`)
	require.Nil(t, err)
	require.True(t, rows.Next())

	var schemaVersion int
	require.Nil(t, rows.Scan(&schemaVersion))
	require.Equal(t, sqliteCurrentSchemaVersion, schemaVersion)
	require.Nil(t, rows.Close())
}

func newTestManager(t *testing.T, newManager newManagerFunc, defaultAccess Permission) *Manager {
	a := newManager(&Config{
		DefaultAccess:       defaultAccess,
		BcryptCost:          bcrypt.MinCost,
		QueueWriterInterval: DefaultUserStatsQueueWriterInterval,
	})
	t.Cleanup(func() { a.Close() })
	return a
}

func newTestManagerFromFile(t *testing.T, filename, startupQueries string, defaultAccess Permission, bcryptCost int, statsWriterInterval time.Duration) *Manager {
	a, err := NewSQLiteManager(filename, startupQueries, &Config{
		DefaultAccess:       defaultAccess,
		BcryptCost:          bcryptCost,
		QueueWriterInterval: statsWriterInterval,
	})
	require.Nil(t, err)
	return a
}

func newTestManagerFromConfig(t *testing.T, newManager newManagerFunc, conf *Config) *Manager {
	a := newManager(conf)
	t.Cleanup(func() { a.Close() })
	return a
}

func testDB(a *Manager) *db.DB {
	return a.db
}

func forEachStoreBackend(t *testing.T, f func(t *testing.T, manager *Manager)) {
	t.Run("sqlite", func(t *testing.T) {
		manager, err := NewSQLiteManager(filepath.Join(t.TempDir(), "user.db"), "", &Config{})
		require.Nil(t, err)
		t.Cleanup(func() { manager.Close() })
		f(t, manager)
	})
	t.Run("postgres", func(t *testing.T) {
		testDB := dbtest.CreateTestPostgres(t)
		manager, err := NewPostgresManager(testDB, &Config{})
		require.Nil(t, err)
		f(t, manager)
	})
}

func TestStoreAddUser(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)
		require.Equal(t, RoleUser, u.Role)
		require.False(t, u.Provisioned)
		require.NotEmpty(t, u.ID)
		require.NotEmpty(t, u.SyncTopic)
	})
}

func TestStoreAddUserAlreadyExists(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "pass1", RoleUser, false))
		require.Equal(t, ErrUserExists, manager.AddUser("phil", "pass2", RoleUser, false))
	})
}

func TestStoreRemoveUser(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)

		require.Nil(t, manager.RemoveUser("phil"))
		_, err = manager.User("phil")
		require.Equal(t, ErrUserNotFound, err)
	})
}

func TestStoreUserByID(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleAdmin, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		u2, err := manager.UserByID(u.ID)
		require.Nil(t, err)
		require.Equal(t, u.Name, u2.Name)
		require.Equal(t, u.ID, u2.ID)
	})
}

func TestStoreUserByToken(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		tk, err := manager.CreateToken(u.ID, "test token", time.Now().Add(24*time.Hour), netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)
		require.NotEmpty(t, tk.Value)

		u2, err := manager.userByToken(tk.Value)
		require.Nil(t, err)
		require.Equal(t, "phil", u2.Name)
	})
}

func TestStoreUserByStripeCustomer(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.ChangeBilling("phil", &Billing{
			StripeCustomerID:     "cus_test123",
			StripeSubscriptionID: "sub_test123",
		}))

		u, err := manager.UserByStripeCustomer("cus_test123")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)
		require.Equal(t, "cus_test123", u.Billing.StripeCustomerID)
	})
}

func TestStoreUsers(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddUser("ben", "benpass", RoleAdmin, false))

		users, err := manager.Users()
		require.Nil(t, err)
		require.True(t, len(users) >= 3) // phil, ben, and the everyone user
	})
}

func TestStoreUsersCount(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		count, err := manager.UsersCount()
		require.Nil(t, err)
		require.True(t, count >= 1) // At least the everyone user

		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		count2, err := manager.UsersCount()
		require.Nil(t, err)
		require.Equal(t, count+1, count2)
	})
}

func TestStoreChangePassword(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)
		require.NotEmpty(t, u.Hash)

		require.Nil(t, manager.ChangePassword("phil", "newpass", false))
		u, err = manager.User("phil")
		require.Nil(t, err)
		require.NotEmpty(t, u.Hash)
	})
}

func TestStoreChangeRole(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, RoleUser, u.Role)

		require.Nil(t, manager.ChangeRole("phil", RoleAdmin))
		u, err = manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, RoleAdmin, u.Role)
	})
}

func TestStoreTokens(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		expires := time.Now().Add(24 * time.Hour)
		origin := netip.MustParseAddr("9.9.9.9")

		tk, err := manager.CreateToken(u.ID, "my token", expires, origin, false)
		require.Nil(t, err)
		require.NotEmpty(t, tk.Value)
		require.Equal(t, "my token", tk.Label)

		// Get single token
		tk2, err := manager.Token(u.ID, tk.Value)
		require.Nil(t, err)
		require.Equal(t, tk.Value, tk2.Value)
		require.Equal(t, "my token", tk2.Label)

		// Get all tokens
		tokens, err := manager.Tokens(u.ID)
		require.Nil(t, err)
		require.Len(t, tokens, 1)
		require.Equal(t, tk.Value, tokens[0].Value)
	})
}

func TestStoreTokenChange(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		expires := time.Now().Add(time.Hour)
		tk, err := manager.CreateToken(u.ID, "old label", expires, netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)

		newLabel := "new label"
		newExpires := time.Now().Add(2 * time.Hour)
		tk2, err := manager.ChangeToken(u.ID, tk.Value, &newLabel, &newExpires)
		require.Nil(t, err)
		require.Equal(t, "new label", tk2.Label)
		require.Equal(t, newExpires.Unix(), tk2.Expires.Unix())
	})
}

func TestStoreTokenRemove(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		tk, err := manager.CreateToken(u.ID, "label", time.Now().Add(time.Hour), netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)

		require.Nil(t, manager.RemoveToken(u.ID, tk.Value))
		_, err = manager.Token(u.ID, tk.Value)
		require.Equal(t, ErrTokenNotFound, err)
	})
}

func TestStoreTokenRemoveExpired(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		// Create expired token and active token
		tkExpired, err := manager.CreateToken(u.ID, "expired", time.Now().Add(-time.Hour), netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)
		tkActive, err := manager.CreateToken(u.ID, "active", time.Now().Add(time.Hour), netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)

		require.Nil(t, manager.RemoveExpiredTokens())

		// Expired token should be gone
		_, err = manager.Token(u.ID, tkExpired.Value)
		require.Equal(t, ErrTokenNotFound, err)

		// Active token should still exist
		tk, err := manager.Token(u.ID, tkActive.Value)
		require.Nil(t, err)
		require.Equal(t, tkActive.Value, tk.Value)
	})
}

func TestStoreTokenUpdateLastAccess(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		tk, err := manager.CreateToken(u.ID, "label", time.Now().Add(time.Hour), netip.MustParseAddr("1.2.3.4"), false)
		require.Nil(t, err)

		newTime := time.Now().Add(5 * time.Minute)
		newOrigin := netip.MustParseAddr("5.5.5.5")
		manager.EnqueueTokenUpdate(tk.Value, &TokenUpdate{LastAccess: newTime, LastOrigin: newOrigin})
	})
}

func TestStoreAllowAccess(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))

		require.Nil(t, manager.AllowAccess("phil", "mytopic", PermissionReadWrite))
		grants, err := manager.Grants("phil")
		require.Nil(t, err)
		require.Len(t, grants, 1)
		require.Equal(t, "mytopic", grants[0].TopicPattern)
		require.True(t, grants[0].Permission.IsReadWrite())
	})
}

func TestStoreAllowAccessReadOnly(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))

		require.Nil(t, manager.AllowAccess("phil", "announcements", PermissionRead))
		grants, err := manager.Grants("phil")
		require.Nil(t, err)
		require.Len(t, grants, 1)
		require.True(t, grants[0].Permission.IsRead())
		require.False(t, grants[0].Permission.IsWrite())
	})
}

func TestStoreResetAccess(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AllowAccess("phil", "topic1", PermissionReadWrite))
		require.Nil(t, manager.AllowAccess("phil", "topic2", PermissionRead))

		grants, err := manager.Grants("phil")
		require.Nil(t, err)
		require.Len(t, grants, 2)

		require.Nil(t, manager.ResetAccess("phil", "topic1"))
		grants, err = manager.Grants("phil")
		require.Nil(t, err)
		require.Len(t, grants, 1)
		require.Equal(t, "topic2", grants[0].TopicPattern)
	})
}

func TestStoreResetAccessAll(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AllowAccess("phil", "topic1", PermissionReadWrite))
		require.Nil(t, manager.AllowAccess("phil", "topic2", PermissionRead))

		require.Nil(t, manager.ResetAccess("phil", ""))
		grants, err := manager.Grants("phil")
		require.Nil(t, err)
		require.Len(t, grants, 0)
	})
}

func TestStoreAuthorizeTopicAccess(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AllowAccess("phil", "mytopic", PermissionReadWrite))

		read, write, found, err := manager.authorizeTopicAccess("phil", "mytopic")
		require.Nil(t, err)
		require.True(t, found)
		require.True(t, read)
		require.True(t, write)
	})
}

func TestStoreAuthorizeTopicAccessNotFound(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))

		_, _, found, err := manager.authorizeTopicAccess("phil", "other")
		require.Nil(t, err)
		require.False(t, found)
	})
}

func TestStoreAuthorizeTopicAccessDenyAll(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AllowAccess("phil", "secret", PermissionDenyAll))

		read, write, found, err := manager.authorizeTopicAccess("phil", "secret")
		require.Nil(t, err)
		require.True(t, found)
		require.False(t, read)
		require.False(t, write)
	})
}

func TestStoreReservations(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddReservation("phil", "mytopic", PermissionRead, 0))

		reservations, err := manager.Reservations("phil")
		require.Nil(t, err)
		require.Len(t, reservations, 1)
		require.Equal(t, "mytopic", reservations[0].Topic)
		require.True(t, reservations[0].Owner.IsReadWrite())
		require.True(t, reservations[0].Everyone.IsRead())
		require.False(t, reservations[0].Everyone.IsWrite())
	})
}

func TestStoreReservationsCount(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddReservation("phil", "topic1", PermissionReadWrite, 0))
		require.Nil(t, manager.AddReservation("phil", "topic2", PermissionReadWrite, 0))

		count, err := manager.ReservationsCount("phil")
		require.Nil(t, err)
		require.Equal(t, int64(2), count)
	})
}

func TestStoreHasReservation(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddReservation("phil", "mytopic", PermissionReadWrite, 0))

		has, err := manager.HasReservation("phil", "mytopic")
		require.Nil(t, err)
		require.True(t, has)

		has, err = manager.HasReservation("phil", "other")
		require.Nil(t, err)
		require.False(t, has)
	})
}

func TestStoreReservationOwner(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddReservation("phil", "mytopic", PermissionReadWrite, 0))

		owner, err := manager.ReservationOwner("mytopic")
		require.Nil(t, err)
		require.NotEmpty(t, owner) // Returns the user ID

		owner, err = manager.ReservationOwner("unowned")
		require.Nil(t, err)
		require.Empty(t, owner)
	})
}

func TestStoreAddReservationWithLimit(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))

		// Adding reservations within limit succeeds
		require.Nil(t, manager.AddReservation("phil", "topic1", PermissionReadWrite, 2))
		require.Nil(t, manager.AddReservation("phil", "topic2", PermissionRead, 2))

		// Adding a third reservation exceeds the limit
		require.Equal(t, ErrTooManyReservations, manager.AddReservation("phil", "topic3", PermissionRead, 2))

		// Updating an existing reservation within the limit succeeds
		require.Nil(t, manager.AddReservation("phil", "topic1", PermissionRead, 2))

		reservations, err := manager.Reservations("phil")
		require.Nil(t, err)
		require.Len(t, reservations, 2)
	})
}

func TestStoreTiers(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		tier := &Tier{
			ID:                       "ti_test",
			Code:                     "pro",
			Name:                     "Pro",
			MessageLimit:             5000,
			MessageExpiryDuration:    24 * time.Hour,
			EmailLimit:               100,
			CallLimit:                10,
			ReservationLimit:         20,
			AttachmentFileSizeLimit:  10 * 1024 * 1024,
			AttachmentTotalSizeLimit: 100 * 1024 * 1024,
			AttachmentExpiryDuration: 48 * time.Hour,
			AttachmentBandwidthLimit: 500 * 1024 * 1024,
		}
		require.Nil(t, manager.AddTier(tier))

		// Get by code
		t2, err := manager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "ti_test", t2.ID)
		require.Equal(t, "pro", t2.Code)
		require.Equal(t, "Pro", t2.Name)
		require.Equal(t, int64(5000), t2.MessageLimit)
		require.Equal(t, int64(100), t2.EmailLimit)
		require.Equal(t, int64(10), t2.CallLimit)
		require.Equal(t, int64(20), t2.ReservationLimit)

		// List all tiers
		tiers, err := manager.Tiers()
		require.Nil(t, err)
		require.Len(t, tiers, 1)
		require.Equal(t, "pro", tiers[0].Code)
	})
}

func TestStoreTierUpdate(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		tier := &Tier{
			ID:   "ti_test",
			Code: "pro",
			Name: "Pro",
		}
		require.Nil(t, manager.AddTier(tier))

		tier.Name = "Professional"
		tier.MessageLimit = 9999
		require.Nil(t, manager.UpdateTier(tier))

		t2, err := manager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "Professional", t2.Name)
		require.Equal(t, int64(9999), t2.MessageLimit)
	})
}

func TestStoreTierRemove(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		tier := &Tier{
			ID:   "ti_test",
			Code: "pro",
			Name: "Pro",
		}
		require.Nil(t, manager.AddTier(tier))

		t2, err := manager.Tier("pro")
		require.Nil(t, err)
		require.Equal(t, "pro", t2.Code)

		require.Nil(t, manager.RemoveTier("pro"))
		_, err = manager.Tier("pro")
		require.Equal(t, ErrTierNotFound, err)
	})
}

func TestStoreTierByStripePrice(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		tier := &Tier{
			ID:                   "ti_test",
			Code:                 "pro",
			Name:                 "Pro",
			StripeMonthlyPriceID: "price_monthly",
			StripeYearlyPriceID:  "price_yearly",
		}
		require.Nil(t, manager.AddTier(tier))

		t2, err := manager.TierByStripePrice("price_monthly")
		require.Nil(t, err)
		require.Equal(t, "pro", t2.Code)

		t3, err := manager.TierByStripePrice("price_yearly")
		require.Nil(t, err)
		require.Equal(t, "pro", t3.Code)
	})
}

func TestStoreChangeTier(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		tier := &Tier{
			ID:   "ti_test",
			Code: "pro",
			Name: "Pro",
		}
		require.Nil(t, manager.AddTier(tier))
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.ChangeTier("phil", "pro"))

		u, err := manager.User("phil")
		require.Nil(t, err)
		require.NotNil(t, u.Tier)
		require.Equal(t, "pro", u.Tier.Code)
	})
}

func TestStorePhoneNumbers(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		require.Nil(t, manager.AddPhoneNumber(u.ID, "+1234567890"))
		require.Nil(t, manager.AddPhoneNumber(u.ID, "+0987654321"))

		numbers, err := manager.PhoneNumbers(u.ID)
		require.Nil(t, err)
		require.Len(t, numbers, 2)

		require.Nil(t, manager.RemovePhoneNumber(u.ID, "+1234567890"))
		numbers, err = manager.PhoneNumbers(u.ID)
		require.Nil(t, err)
		require.Len(t, numbers, 1)
		require.Equal(t, "+0987654321", numbers[0])
	})
}

func TestStoreEmails(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		require.Nil(t, manager.AddEmail(u.ID, "phil@example.com"))
		require.Nil(t, manager.AddEmail(u.ID, "phil2@example.com"))

		emails, err := manager.Emails(u.ID)
		require.Nil(t, err)
		require.Len(t, emails, 2)

		require.Nil(t, manager.RemoveEmail(u.ID, "phil@example.com"))
		emails, err = manager.Emails(u.ID)
		require.Nil(t, err)
		require.Len(t, emails, 1)
		require.Equal(t, "phil2@example.com", emails[0])
	})
}

func TestStoreChangeSettings(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		lang := "de"
		prefs := &Prefs{Language: &lang}
		require.Nil(t, manager.ChangeSettings(u.ID, prefs))

		u2, err := manager.User("phil")
		require.Nil(t, err)
		require.NotNil(t, u2.Prefs)
		require.Equal(t, "de", *u2.Prefs.Language)
	})
}

func TestStoreChangeBilling(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))

		billing := &Billing{
			StripeCustomerID:     "cus_123",
			StripeSubscriptionID: "sub_456",
		}
		require.Nil(t, manager.ChangeBilling("phil", billing))

		u, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, "cus_123", u.Billing.StripeCustomerID)
		require.Equal(t, "sub_456", u.Billing.StripeSubscriptionID)
	})
}

func TestStoreUpdateStats(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		manager.EnqueueUserStats(u.ID, &Stats{Messages: 42, Emails: 3, Calls: 1})
		require.Nil(t, manager.writeUserStatsQueue())

		u2, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(42), u2.Stats.Messages)
		require.Equal(t, int64(3), u2.Stats.Emails)
		require.Equal(t, int64(1), u2.Stats.Calls)
	})
}

func TestStoreResetStats(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		manager.EnqueueUserStats(u.ID, &Stats{Messages: 42, Emails: 3, Calls: 1})
		require.Nil(t, manager.writeUserStatsQueue())
		require.Nil(t, manager.ResetStats())

		u2, err := manager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(0), u2.Stats.Messages)
		require.Equal(t, int64(0), u2.Stats.Emails)
		require.Equal(t, int64(0), u2.Stats.Calls)
	})
}

func TestStoreMarkUserRemoved(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		require.Nil(t, manager.MarkUserRemoved(u))

		u2, err := manager.User("phil")
		require.Nil(t, err)
		require.True(t, u2.Deleted)
	})
}

func TestStoreRemoveDeletedUsers(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		u, err := manager.User("phil")
		require.Nil(t, err)

		require.Nil(t, manager.MarkUserRemoved(u))

		// RemoveDeletedUsers only removes users past the hard-delete duration (7 days).
		// Immediately after marking, the user should still exist.
		require.Nil(t, manager.RemoveDeletedUsers())
		u2, err := manager.User("phil")
		require.Nil(t, err)
		require.True(t, u2.Deleted)
	})
}

func TestStoreAllGrants(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddUser("ben", "benpass", RoleUser, false))
		phil, err := manager.User("phil")
		require.Nil(t, err)
		ben, err := manager.User("ben")
		require.Nil(t, err)

		require.Nil(t, manager.AllowAccess("phil", "topic1", PermissionReadWrite))
		require.Nil(t, manager.AllowAccess("ben", "topic2", PermissionRead))

		grants, err := manager.AllGrants()
		require.Nil(t, err)
		require.Contains(t, grants, phil.ID)
		require.Contains(t, grants, ben.ID)
	})
}

func TestStoreOtherAccessCount(t *testing.T) {
	forEachStoreBackend(t, func(t *testing.T, manager *Manager) {
		require.Nil(t, manager.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, manager.AddUser("ben", "benpass", RoleUser, false))
		require.Nil(t, manager.AddReservation("ben", "mytopic", PermissionReadWrite, 0))

		count, err := manager.otherAccessCount("phil", "mytopic")
		require.Nil(t, err)
		require.Equal(t, 2, count) // ben's owner entry + everyone entry
	})
}
