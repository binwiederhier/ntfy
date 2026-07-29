package user

import (
	"database/sql"
	"errors"
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
	"heckel.io/ntfy/v2/db/schema"
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

		// User has full access to their own sync topic, even under deny-all,
		// but not to another user's sync topic (#733)
		require.Nil(t, a.Authorize(ben, ben.SyncTopic, PermissionRead))
		require.Nil(t, a.Authorize(ben, ben.SyncTopic, PermissionWrite))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, john.SyncTopic, PermissionRead))
		require.Equal(t, ErrUnauthorized, a.Authorize(ben, john.SyncTopic, PermissionWrite))

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
		require.Equal(t, "phil@example.com", emails[0].Address)

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

// testPostgresV6Schema is the PostgreSQL schema exactly as created by the version that first
// shipped Postgres support (schema version 6), taken from the code at that time; used to
// verify the migration chain from its oldest supported version.
const testPostgresV6Schema = `
	CREATE TABLE IF NOT EXISTS tier (
		id TEXT PRIMARY KEY,
		code TEXT NOT NULL,
		name TEXT NOT NULL,
		messages_limit BIGINT NOT NULL,
		messages_expiry_duration BIGINT NOT NULL,
		emails_limit BIGINT NOT NULL,
		calls_limit BIGINT NOT NULL,
		reservations_limit BIGINT NOT NULL,
		attachment_file_size_limit BIGINT NOT NULL,
		attachment_total_size_limit BIGINT NOT NULL,
		attachment_expiry_duration BIGINT NOT NULL,
		attachment_bandwidth_limit BIGINT NOT NULL,
		stripe_monthly_price_id TEXT,
		stripe_yearly_price_id TEXT,
		UNIQUE(code),
		UNIQUE(stripe_monthly_price_id),
		UNIQUE(stripe_yearly_price_id)
	);
	CREATE TABLE IF NOT EXISTS "user" (
	    id TEXT PRIMARY KEY,
		tier_id TEXT REFERENCES tier(id),
		user_name TEXT NOT NULL UNIQUE,
		pass TEXT NOT NULL,
		role TEXT NOT NULL CHECK (role IN ('anonymous', 'admin', 'user')),
		prefs JSONB NOT NULL DEFAULT '{}',
		sync_topic TEXT NOT NULL,
		provisioned BOOLEAN NOT NULL,
		stats_messages BIGINT NOT NULL DEFAULT 0,
		stats_emails BIGINT NOT NULL DEFAULT 0,
		stats_calls BIGINT NOT NULL DEFAULT 0,
		stripe_customer_id TEXT UNIQUE,
		stripe_subscription_id TEXT UNIQUE,
		stripe_subscription_status TEXT,
		stripe_subscription_interval TEXT,
		stripe_subscription_paid_until BIGINT,
		stripe_subscription_cancel_at BIGINT,
		created BIGINT NOT NULL,
		deleted BIGINT
	);
	CREATE TABLE IF NOT EXISTS user_access (
		user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
		topic TEXT NOT NULL,
		read BOOLEAN NOT NULL,
		write BOOLEAN NOT NULL,
		owner_user_id TEXT REFERENCES "user"(id) ON DELETE CASCADE,
		provisioned BOOLEAN NOT NULL,
		PRIMARY KEY (user_id, topic)
	);
	CREATE TABLE IF NOT EXISTS user_token (
		user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
		token TEXT NOT NULL UNIQUE,
		label TEXT NOT NULL,
		last_access BIGINT NOT NULL,
		last_origin TEXT NOT NULL,
		expires BIGINT NOT NULL,
		provisioned BOOLEAN NOT NULL,
		PRIMARY KEY (user_id, token)
	);
	CREATE TABLE IF NOT EXISTS user_phone (
		user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
		phone_number TEXT NOT NULL,
		PRIMARY KEY (user_id, phone_number)
	);
	CREATE TABLE IF NOT EXISTS schema_version (
		store TEXT PRIMARY KEY,
		version INT NOT NULL
	);
	INSERT INTO "user" (id, user_name, pass, role, sync_topic, provisioned, created)
	VALUES ('u_everyone', '*', '', 'anonymous', '', false, EXTRACT(EPOCH FROM NOW())::BIGINT)
	ON CONFLICT (id) DO NOTHING;
`

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

	checkMigratedSqliteSchema(t, filename)
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

	// Insert a few ACL entries, and phone numbers: one for a live user, one orphaned (its user
	// is gone; the broken pre-v9 foreign key never cascade-deleted it)
	_, err = db.Exec(`
		BEGIN;
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'mytopic_', 1, 1);
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'up%', 1, 1);
		INSERT INTO user_access (user_id, topic, read, write) values ('u_everyone', 'down_%', 1, 1);
		INSERT INTO user_phone (user_id, phone_number) VALUES ('u_everyone', '+12223334444');
		INSERT INTO user_phone (user_id, phone_number) VALUES ('u_gone', '+15556667777');
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

	// The 8 -> 9 repair kept the live user's phone number and dropped the orphaned row
	phoneNumbers := make([]string, 0)
	rows, err = db.Query(`SELECT phone_number FROM user_phone ORDER BY phone_number`)
	require.Nil(t, err)
	for rows.Next() {
		var phoneNumber string
		require.Nil(t, rows.Scan(&phoneNumber))
		phoneNumbers = append(phoneNumbers, phoneNumber)
	}
	require.Nil(t, rows.Close())
	require.Equal(t, []string{"+12223334444"}, phoneNumbers)

	checkMigratedSqliteSchema(t, filename)
}

// TestMigrationFrom6Postgres tests the Postgres migration chain from its oldest supported
// version (6, the version PostgreSQL support first shipped with).
func TestMigrationFrom6Postgres(t *testing.T) {
	testDB := dbtest.CreateTestPostgres(t)
	_, err := testDB.Exec(testPostgresV6Schema)
	require.Nil(t, err)
	_, err = testDB.Exec(`INSERT INTO schema_version (store, version) VALUES ('user', 6)`)
	require.Nil(t, err)
	// Create manager to trigger migration
	a, err := NewPostgresManager(testDB, &Config{DefaultAccess: PermissionDenyAll, BcryptCost: bcrypt.MinCost, QueueWriterInterval: DefaultUserStatsQueueWriterInterval})
	require.Nil(t, err)
	var version int
	require.Nil(t, testDB.QueryRow(`SELECT version FROM schema_version WHERE store = 'user'`).Scan(&version))
	require.Equal(t, postgresCurrentSchemaVersion, version)
	// The manager works against the migrated schema
	require.Nil(t, a.AddUser("phil", "mypass", RoleUser, false))
	u, err := a.User("phil")
	require.Nil(t, err)
	require.Nil(t, a.AddEmail(u.ID, "phil@example.com"))
	// The migrated database must be structurally identical to a freshly created one
	freshDB := dbtest.CreateTestPostgres(t)
	_, err = NewPostgresManager(freshDB, &Config{DefaultAccess: PermissionDenyAll, BcryptCost: bcrypt.MinCost, QueueWriterInterval: DefaultUserStatsQueueWriterInterval})
	require.Nil(t, err)
	require.Equal(t, dbtest.PostgresSchema(t, freshDB), dbtest.PostgresSchema(t, testDB))
}

// checkMigratedSqliteSchema verifies that a migrated database is structurally identical to a
// freshly created one (this pins, among other things, that the foreign keys of the tables
// rebuilt in migration 5 -> 6 still point at "user", not at a dropped "user_old"), and that
// its data passes SQLite's foreign key consistency check.
func checkMigratedSqliteSchema(t *testing.T, filename string) {
	t.Helper()
	freshFile := filepath.Join(t.TempDir(), "fresh.db")
	fresh := newTestManagerFromFile(t, freshFile, "", PermissionDenyAll, bcrypt.MinCost, DefaultUserStatsQueueWriterInterval)
	defer fresh.Close()
	freshDB, err := sql.Open("sqlite3", freshFile)
	require.Nil(t, err)
	defer freshDB.Close()
	migratedDB, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)
	defer migratedDB.Close()
	require.Equal(t, dbtest.SQLiteSchema(t, freshDB), dbtest.SQLiteSchema(t, migratedDB))
	rows, err := migratedDB.Query(`PRAGMA foreign_key_check`)
	require.Nil(t, err)
	defer rows.Close()
	require.False(t, rows.Next(), "foreign_key_check reported violations in the migrated database")
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

// TestAuthorizeTopicAccess_CacheAndDirectDBAgree wires up two Managers on the
// same backend storage -- one with AccessCacheEnabled=true (in-memory cache
// path) and one with AccessCacheEnabled=false (direct SQL path) -- then runs
// an identical battery of authorizeTopicAccess queries against both and
// asserts byte-identical (read, write, found) responses for every query.
// This protects the in-memory implementation from drifting away from the
// SQL behavior it is meant to mirror.
func TestAuthorizeTopicAccess_CacheAndDirectDBAgree(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		// Seed via a Manager with the cache enabled. Writes go to the shared
		// backend; both Managers will see them after the writes commit.
		writer := newManager(&Config{
			DefaultAccess:      PermissionDenyAll,
			BcryptCost:         bcrypt.MinCost,
			AccessCacheEnabled: true,
		})
		t.Cleanup(func() { writer.Close() })

		require.Nil(t, writer.AddUser("phil", "mypass", RoleAdmin, false))
		require.Nil(t, writer.AddUser("ben", "mypass", RoleUser, false))
		require.Nil(t, writer.AddUser("alice", "mypass", RoleUser, false))

		// A mix that exercises every branch of the priority logic:
		//   - exact and wildcard rules for the same user
		//   - exact and wildcard rules under Everyone
		//   - Everyone rules that are longer than the matching user rule
		//   - literal underscores (stored as "\_")
		//   - deny-all permissions
		require.Nil(t, writer.AllowAccess("ben", "mytopic", PermissionReadWrite))
		require.Nil(t, writer.AllowAccess("ben", "readme", PermissionRead))
		require.Nil(t, writer.AllowAccess("ben", "writeme", PermissionWrite))
		require.Nil(t, writer.AllowAccess("ben", "ben_topic", PermissionReadWrite))
		require.Nil(t, writer.AllowAccess("ben", "mytopic*", PermissionRead))
		require.Nil(t, writer.AllowAccess("alice", "alice_*", PermissionWrite))
		require.Nil(t, writer.AllowAccess("alice", "secret", PermissionDenyAll))
		require.Nil(t, writer.AllowAccess(Everyone, "announcements", PermissionRead))
		require.Nil(t, writer.AllowAccess(Everyone, "up*", PermissionWrite))
		require.Nil(t, writer.AllowAccess(Everyone, "mytopic", PermissionDenyAll))

		// Build a reader Manager with the cache OFF, pointing at the same backend.
		reader := newManager(&Config{
			DefaultAccess:      PermissionDenyAll,
			BcryptCost:         bcrypt.MinCost,
			AccessCacheEnabled: false,
		})
		t.Cleanup(func() { reader.Close() })

		// Probe matrix: every (user, topic) pair that exercises some branch.
		cases := []struct {
			user, topic string
		}{
			// Anonymous reads.
			{Everyone, "announcements"},
			{Everyone, "up42"},
			{Everyone, "up"},
			{Everyone, "downstream"},
			{Everyone, "mytopic"},
			{Everyone, "nope"},
			// Specific user, only-user rules.
			{"ben", "mytopic"},
			{"ben", "readme"},
			{"ben", "writeme"},
			{"ben", "ben_topic"},
			{"ben", "benXtopic"}, // underscore in rule means "X" must NOT match
			// Specific user falls through to Everyone.
			{"ben", "announcements"},
			{"ben", "up5"},
			{"alice", "announcements"},
			// Wildcards with literal underscores.
			{"alice", "alice_anything"},
			{"alice", "alice_"},
			{"alice", "aliceX"}, // does NOT match alice_*
			// Exact-vs-wildcard overlap for the same user (ben has both
			// "mytopic" exact and "mytopic*" wildcard).
			{"ben", "mytopic"},   // exact wins on length
			{"ben", "mytopicX"},  // only wildcard matches
			{"ben", "mytopicYZ"}, // only wildcard matches
			// Deny-all override.
			{"alice", "secret"},
			// No matching rule anywhere.
			{"ben", "completely_unmatched"},
			{"alice", "completely_unmatched"},
			{Everyone, "completely_unmatched"},
		}

		// Sanity: the two Managers must agree on every probe.
		for _, tc := range cases {
			cRead, cWrite, cFound, cErr := writer.authorizeTopicAccess(tc.user, tc.topic)
			dRead, dWrite, dFound, dErr := reader.authorizeTopicAccess(tc.user, tc.topic)
			require.Nil(t, cErr, "cache path errored for (%s, %s)", tc.user, tc.topic)
			require.Nil(t, dErr, "direct-DB path errored for (%s, %s)", tc.user, tc.topic)
			require.Equal(t, dFound, cFound, "found mismatch for (%s, %s)", tc.user, tc.topic)
			require.Equal(t, dRead, cRead, "read mismatch for (%s, %s)", tc.user, tc.topic)
			require.Equal(t, dWrite, cWrite, "write mismatch for (%s, %s)", tc.user, tc.topic)
		}
	})
}

// TestAccessCacheReloadInterval_PicksUpExternalWrite proves that the
// background reloader actually closes the cross-process coherence gap: a
// write made through a *different* Manager on the same backend becomes
// visible to a cache-enabled Manager within roughly one reload interval,
// without that Manager being told about the write.
func TestAccessCacheReloadInterval_PicksUpExternalWrite(t *testing.T) {
	const interval = 25 * time.Millisecond
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		// reader holds the cache and polls; writer plays the role of an
		// out-of-band process (e.g. `ntfy access` CLI) writing to the same
		// backend.
		reader := newManager(&Config{
			DefaultAccess:             PermissionDenyAll,
			BcryptCost:                bcrypt.MinCost,
			AccessCacheEnabled:        true,
			AccessCacheReloadInterval: interval,
		})
		t.Cleanup(func() { reader.Close() })

		writer := newManager(&Config{
			DefaultAccess:      PermissionDenyAll,
			BcryptCost:         bcrypt.MinCost,
			AccessCacheEnabled: false,
		})
		t.Cleanup(func() { writer.Close() })

		require.Nil(t, writer.AddUser("phil", "mypass", RoleUser, false))
		// Sanity: before the write, the reader sees no rule for this topic.
		_, _, found, err := reader.authorizeTopicAccess("phil", "via-poller")
		require.Nil(t, err)
		require.False(t, found)

		// Write through the second Manager. reader's cache is unaware.
		require.Nil(t, writer.AllowAccess("phil", "via-poller", PermissionReadWrite))

		// Wait for the poller to catch up. The interval is 25ms; allow a
		// generous multiple to keep this test from flaking on slow CI.
		require.Eventually(t, func() bool {
			read, write, found, err := reader.authorizeTopicAccess("phil", "via-poller")
			return err == nil && found && read && write
		}, 2*time.Second, 10*time.Millisecond, "reader's cache never observed the external write")
	})
}

// TestAccessCache_RemoveExcessReservationsInvalidatesCache models finding #1:
// RemoveExcessReservations deletes user_access rows but must also refresh the
// in-memory cache. Otherwise the owner keeps cached read/write access to a
// reservation that was removed (e.g. on a tier downgrade) until the next
// periodic reload -- and if another user re-reserves the freed topic in the
// meantime, the former owner can read/write the new owner's reserved topic.
func TestAccessCache_RemoveExcessReservationsInvalidatesCache(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		// A deliberately long reload interval ensures the background poller
		// cannot mask a missing synchronous invalidation: the mutation itself
		// must refresh the cache.
		a := newTestManagerFromConfig(t, newManager, &Config{
			DefaultAccess:             PermissionDenyAll,
			BcryptCost:                bcrypt.MinCost,
			AccessCacheEnabled:        true,
			AccessCacheReloadInterval: time.Hour,
		})
		require.Nil(t, a.AddUser("ben", "mypass", RoleUser, false))
		require.Nil(t, a.AddReservation("ben", "topic1", PermissionDenyAll, 2))
		require.Nil(t, a.AddReservation("ben", "topic2", PermissionDenyAll, 2))

		// Both reservations grant ben full read/write; confirm the cache agrees.
		for _, topic := range []string{"topic1", "topic2"} {
			read, write, found, err := a.authorizeTopicAccess("ben", topic)
			require.Nil(t, err)
			require.True(t, found)
			require.True(t, read)
			require.True(t, write)
		}

		// Downgrade ben to a single reservation; one topic is removed from the DB.
		removed, err := a.RemoveExcessReservations("ben", 1)
		require.Nil(t, err)
		require.Len(t, removed, 1)

		// The removed reservation's grant must be gone from the cache, not just
		// from the database.
		read, write, found, err := a.authorizeTopicAccess("ben", removed[0])
		require.Nil(t, err)
		require.False(t, found, "stale ACL for removed reservation %q still served from cache", removed[0])
		require.False(t, read)
		require.False(t, write)

		// The surviving reservation must still be served from the cache.
		survivor := "topic1"
		if removed[0] == "topic1" {
			survivor = "topic2"
		}
		read, write, found, err = a.authorizeTopicAccess("ben", survivor)
		require.Nil(t, err)
		require.True(t, found)
		require.True(t, read)
		require.True(t, write)
	})
}

// TestAccessCache_FullReloadDoesNotClobberConcurrentRevoke models finding #2:
// a periodic full reload scans the whole user_access table outside the cache
// lock. If a local ACL mutation revokes a grant and refreshes that user's slice
// while the scan is in flight, applying the now-stale full snapshot must not
// resurrect the revoked grant. The testHookReloadScanned seam injects the revoke
// into exactly that race window.
func TestAccessCache_FullReloadDoesNotClobberConcurrentRevoke(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManagerFromConfig(t, newManager, &Config{
			DefaultAccess:             PermissionDenyAll,
			BcryptCost:                bcrypt.MinCost,
			AccessCacheEnabled:        true,
			AccessCacheReloadInterval: time.Hour, // keep the background poller out of this test
		})
		require.Nil(t, a.AddUser("phil", "mypass", RoleUser, false))
		require.Nil(t, a.AllowAccess("phil", "secret", PermissionReadWrite))

		// Sanity: the grant is served from the cache.
		_, _, found, err := a.authorizeTopicAccess("phil", "secret")
		require.Nil(t, err)
		require.True(t, found)

		// Arm the seam: when the full reload below finishes scanning (and still
		// sees the grant), revoke it via a per-user reload before the full reload
		// applies its now-stale snapshot. The re-entrant per-user reload that
		// ResetAccess triggers is a no-op here (fired guard), and the whole thing
		// runs single-threaded in this goroutine.
		fired := false
		testHookReloadScanned = func() {
			if fired {
				return
			}
			fired = true
			require.Nil(t, a.ResetAccess("phil", "secret"))
		}
		defer func() { testHookReloadScanned = nil }()

		// Trigger the full reload. Without the seq guard it would swap in its
		// stale snapshot and resurrect the grant.
		require.Nil(t, a.maybeReloadAccessCache())

		_, _, found, err = a.authorizeTopicAccess("phil", "secret")
		require.Nil(t, err)
		require.False(t, found, "stale full reload resurrected a revoked grant")
	})
}

// TestAuthorizeTopicAccess_TopicMatchingIsCaseSensitive guards against ACL
// topic matching being case-insensitive. SQLite's LIKE is case-insensitive for
// ASCII by default, which would let a request for "SECRET" match an ACL rule
// for "secret" -- a security hole. PostgreSQL's LIKE is already case-sensitive.
// NewSQLiteManager opens the database with case_sensitive_like enabled to close
// this gap. This exercises the direct-DB path (cache disabled), which is the
// path that runs the LIKE query; the in-memory cache is independently
// case-sensitive (Go map keys / case-sensitive regex).
func TestAuthorizeTopicAccess_TopicMatchingIsCaseSensitive(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newManager(&Config{
			DefaultAccess:      PermissionDenyAll,
			BcryptCost:         bcrypt.MinCost,
			AccessCacheEnabled: false, // exercise the direct-DB LIKE path
		})
		t.Cleanup(func() { a.Close() })

		require.Nil(t, a.AddUser("ben", "mypass", RoleUser, false))
		require.Nil(t, a.AllowAccess("ben", "secret", PermissionReadWrite)) // exact rule
		require.Nil(t, a.AllowAccess("ben", "team*", PermissionReadWrite))  // wildcard rule, stored as "team%"

		// The exact rule is honored verbatim.
		read, write, found, err := a.authorizeTopicAccess("ben", "secret")
		require.Nil(t, err)
		require.True(t, found)
		require.True(t, read)
		require.True(t, write)

		// Case variants of the exact rule must NOT match.
		for _, topic := range []string{"SECRET", "Secret", "sEcReT"} {
			_, _, found, err := a.authorizeTopicAccess("ben", topic)
			require.Nil(t, err)
			require.False(t, found, "ACL rule for \"secret\" must not match %q (case-insensitive match is a security hole)", topic)
		}

		// The wildcard rule is honored for the matching case.
		_, _, found, err = a.authorizeTopicAccess("ben", "team-rocket")
		require.Nil(t, err)
		require.True(t, found)

		// Case variants of the wildcard prefix must NOT match.
		for _, topic := range []string{"TEAM-rocket", "Team-rocket", "TEAMING"} {
			_, _, found, err := a.authorizeTopicAccess("ben", topic)
			require.Nil(t, err)
			require.False(t, found, "wildcard rule for \"team*\" must not match %q", topic)
		}
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
		require.Equal(t, "phil2@example.com", emails[0].Address)
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

// addVerifyLink stores an email-verification magic link and returns the raw token so the test
// can "click" it via VerifyEmail.
func addVerifyLink(t *testing.T, a *Manager, userID, email string, ttl time.Duration) string {
	raw, err := a.AddMagicLink(MagicLinkKindEmailVerify, userID, email, ttl)
	require.Nil(t, err)
	return raw
}

func TestUser_MagicLink_VerifyEmail_SetsPrimary(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw := addVerifyLink(t, a, phil.ID, "phil@example.com", 24*time.Hour)

		// Before verifying: pending, not yet verified, no primary
		pending, err := a.PendingEmails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"phil@example.com"}, pending)
		emails, err := a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(emails))
		primary, err := a.PrimaryEmail(phil.ID)
		require.Nil(t, err)
		require.Equal(t, "", primary)

		// Verify: the first verified email auto-becomes primary
		m, err := a.VerifyEmail(raw)
		require.Nil(t, err)
		require.Equal(t, "phil@example.com", m.Email)

		emails, err = a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"phil@example.com"}, emails.Strings())
		primary, err = a.PrimaryEmail(phil.ID)
		require.Nil(t, err)
		require.Equal(t, "phil@example.com", primary)
		pending, err = a.PendingEmails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(pending))

		// Reset-by-email lookup resolves to the account
		userID, err := a.UserIDByPrimaryEmail("phil@example.com")
		require.Nil(t, err)
		require.Equal(t, phil.ID, userID)
	})
}

func TestUser_MagicLink_VerifyEmail_SecondStaysSecondary(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw1 := addVerifyLink(t, a, phil.ID, "first@example.com", 24*time.Hour)
		_, err = a.VerifyEmail(raw1)
		require.Nil(t, err)

		raw2 := addVerifyLink(t, a, phil.ID, "second@example.com", 24*time.Hour)
		_, err = a.VerifyEmail(raw2)
		require.Nil(t, err)

		// Both verified, but primary is still the first
		emails, err := a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"first@example.com", "second@example.com"}, emails.Strings())
		primary, err := a.PrimaryEmail(phil.ID)
		require.Nil(t, err)
		require.Equal(t, "first@example.com", primary)
	})
}

func TestUser_MagicLink_PrimaryGlobalUniqueness(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		ben, err := a.User("ben")
		require.Nil(t, err)

		// phil verifies shared@ first -> becomes his primary
		_, err = a.VerifyEmail(addVerifyLink(t, a, phil.ID, "shared@example.com", 24*time.Hour))
		require.Nil(t, err)
		primary, err := a.PrimaryEmail(phil.ID)
		require.Nil(t, err)
		require.Equal(t, "shared@example.com", primary)

		// ben verifies the same address -> allowed as secondary, but NOT his primary
		_, err = a.VerifyEmail(addVerifyLink(t, a, ben.ID, "shared@example.com", 24*time.Hour))
		require.Nil(t, err)
		emails, err := a.Emails(ben.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"shared@example.com"}, emails.Strings())
		primary, err = a.PrimaryEmail(ben.ID)
		require.Nil(t, err)
		require.Equal(t, "", primary)

		// Explicitly promoting ben's copy to primary collides with phil's
		require.ErrorIs(t, a.SetPrimaryEmail(ben.ID, "shared@example.com"), ErrEmailPrimaryElsewhere)
		// ...and phil keeps his primary (the failed promotion rolled back ben's clear)
		primary, err = a.PrimaryEmail(phil.ID)
		require.Nil(t, err)
		require.Equal(t, "shared@example.com", primary)
	})
}

func TestManager_Authenticate_ByPrimaryEmail(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		// phil verifies phil@example.com -> becomes his primary (recovery) email
		_, err = a.VerifyEmail(addVerifyLink(t, a, phil.ID, "phil@example.com", 24*time.Hour))
		require.Nil(t, err)

		// Login by username still works
		u, err := a.Authenticate("phil", "phil")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)

		// Login by primary email works and resolves to the same account
		u, err = a.Authenticate("phil@example.com", "phil")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)

		// Login by primary email with the wrong password fails
		u, err = a.Authenticate("phil@example.com", "wrong")
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)

		// An unknown email fails
		u, err = a.Authenticate("nobody@example.com", "phil")
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)
	})
}

func TestManager_Authenticate_BySecondaryEmailDenied(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "ben", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		ben, err := a.User("ben")
		require.Nil(t, err)

		// phil verifies shared@ first -> his primary; ben verifies it too -> only secondary for ben
		_, err = a.VerifyEmail(addVerifyLink(t, a, phil.ID, "shared@example.com", 24*time.Hour))
		require.Nil(t, err)
		_, err = a.VerifyEmail(addVerifyLink(t, a, ben.ID, "shared@example.com", 24*time.Hour))
		require.Nil(t, err)

		// Login by the shared address resolves to the primary owner (phil), never the secondary (ben)
		u, err := a.Authenticate("shared@example.com", "phil")
		require.Nil(t, err)
		require.Equal(t, "phil", u.Name)

		// ben's password must not authenticate via the shared address (it is not his primary)
		u, err = a.Authenticate("shared@example.com", "ben")
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)
	})
}

func TestManager_Authenticate_UsernameLookalikeEmailPrecedence(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)

		// The collision: a squatter whose USERNAME is literally "phil@example.com" (usernames may
		// contain '@' and '.'), and a different account (ben) that owns "phil@example.com" as its
		// verified primary email. Both are reachable; nothing links usernames to email addresses.
		require.Nil(t, a.AddUser("phil@example.com", "squatterpass", RoleUser, false))
		require.Nil(t, a.AddUser("ben", "benpass", RoleUser, false))
		ben, err := a.User("ben")
		require.Nil(t, err)
		_, err = a.VerifyEmail(addVerifyLink(t, a, ben.ID, "phil@example.com", 24*time.Hour))
		require.Nil(t, err)

		// Login resolves the ambiguous identifier username-FIRST (the ORDER BY CASE in the query):
		// the squatter owns the login, and returns deterministically even though both rows match.
		squatter, err := a.Authenticate("phil@example.com", "squatterpass")
		require.Nil(t, err)
		require.Equal(t, "phil@example.com", squatter.Name)

		// Consequently the email owner's password does NOT authenticate via the colliding identifier
		// at login, but the owner is not locked out: their real username still works.
		u, err := a.Authenticate("phil@example.com", "benpass")
		require.Nil(t, u)
		require.Equal(t, ErrUnauthenticated, err)
		u, err = a.Authenticate("ben", "benpass")
		require.Nil(t, err)
		require.Equal(t, "ben", u.Name)

		// The inverse: password reset (UserByEmailOrUsername) resolves the SAME identifier email-FIRST,
		// so the reset link goes to the verified email owner (ben), never the look-alike username. The
		// two flows deliberately use opposite precedence.
		loginUser, err := a.userByNameOrEmail("phil@example.com")
		require.Nil(t, err)
		require.Equal(t, "phil@example.com", loginUser.Name) // username owner (squatter)
		resetUser, err := a.UserByEmailOrUsername("phil@example.com")
		require.Nil(t, err)
		require.Equal(t, "ben", resetUser.Name) // email owner
	})
}

func TestUser_MagicLink_SetPrimary_NotVerified(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)
		require.ErrorIs(t, a.SetPrimaryEmail(phil.ID, "nope@example.com"), ErrEmailNotFound)
	})
}

func TestUser_MagicLink_Expired(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw := addVerifyLink(t, a, phil.ID, "phil@example.com", -time.Minute)
		_, err = a.VerifyEmail(raw)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)

		// Nothing got verified
		emails, err := a.Emails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(emails))
	})
}

func TestUser_MagicLink_SingleUse(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw := addVerifyLink(t, a, phil.ID, "phil@example.com", 24*time.Hour)
		_, err = a.VerifyEmail(raw)
		require.Nil(t, err)
		// Second click: token already consumed
		_, err = a.VerifyEmail(raw)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)
	})
}

func TestUser_MagicLink_ReplaceOnReRequest(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw1 := addVerifyLink(t, a, phil.ID, "phil@example.com", 24*time.Hour)
		raw2 := addVerifyLink(t, a, phil.ID, "phil@example.com", 24*time.Hour)

		// Only one pending row remains; the old token no longer works
		pending, err := a.PendingEmails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"phil@example.com"}, pending)
		_, err = a.MagicLinkByToken(raw1)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)

		m, err := a.MagicLinkByToken(raw2)
		require.Nil(t, err)
		require.Equal(t, "phil@example.com", m.Email)
	})
}

func TestUser_MagicLink_PasswordReset_RoundTrip(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw, err := a.AddMagicLink(MagicLinkKindPasswordReset, phil.ID, "", time.Hour)
		require.Nil(t, err)

		m, err := a.MagicLinkByToken(raw)
		require.Nil(t, err)
		require.Equal(t, MagicLinkKindPasswordReset, m.Kind)
		require.Equal(t, phil.ID, m.UserID)
		require.Equal(t, "", m.Email) // reset rows carry no email

		// Reset rows do not appear as pending emails
		pending, err := a.PendingEmails(phil.ID)
		require.Nil(t, err)
		require.Equal(t, 0, len(pending))

		// New request replaces the old token
		raw2, err := a.AddMagicLink(MagicLinkKindPasswordReset, phil.ID, "", time.Hour)
		require.Nil(t, err)
		_, err = a.MagicLinkByToken(raw)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)

		// Single use: deleting consumes it
		require.Nil(t, a.DeleteMagicLinkByToken(raw2))
		_, err = a.MagicLinkByToken(raw2)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)
	})
}

func TestUser_MagicLink_Reaper(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		expired := addVerifyLink(t, a, phil.ID, "expired@example.com", -time.Hour)
		valid := addVerifyLink(t, a, phil.ID, "valid@example.com", time.Hour)

		require.Nil(t, a.deleteExpiredMagicLinks())

		_, err = a.MagicLinkByToken(expired)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)
		m, err := a.MagicLinkByToken(valid)
		require.Nil(t, err)
		require.Equal(t, "valid@example.com", m.Email)
	})
}

// TestUser_MagicLink_ReaperLoop proves the background reap goroutine actually runs on its
// configured interval: an expired link inserted into a manager with a tiny reap interval is
// deleted without anyone calling deleteExpiredMagicLinks directly. Mirrors the loop-coverage
// pattern of TestAccessCacheReloadInterval_PicksUpExternalWrite.
func TestUser_MagicLink_ReaperLoop(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManagerFromConfig(t, newManager, &Config{
			DefaultAccess:                PermissionDenyAll,
			BcryptCost:                   bcrypt.MinCost,
			ExpiredMagicLinkReapInterval: 25 * time.Millisecond,
		})
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		expired := addVerifyLink(t, a, phil.ID, "expired@example.com", -time.Hour)
		valid := addVerifyLink(t, a, phil.ID, "valid@example.com", time.Hour)

		// The background loop (not a direct call) must reap the expired link within a few intervals
		require.Eventually(t, func() bool {
			_, err := a.MagicLinkByToken(expired)
			return errors.Is(err, ErrMagicLinkNotFound)
		}, 2*time.Second, 10*time.Millisecond, "reaper loop never deleted the expired magic link")

		// The unexpired link must survive
		m, err := a.MagicLinkByToken(valid)
		require.Nil(t, err)
		require.Equal(t, "valid@example.com", m.Email)
	})
}

func TestUser_MagicLink_ResetPassword(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "oldpass", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw, err := a.AddMagicLink(MagicLinkKindPasswordReset, phil.ID, "", time.Hour)
		require.Nil(t, err)

		// Old password works before reset
		_, err = a.Authenticate("phil", "oldpass")
		require.Nil(t, err)

		require.Nil(t, a.ResetPassword(raw, "newpass"))

		// New password works, old does not
		_, err = a.Authenticate("phil", "newpass")
		require.Nil(t, err)
		_, err = a.Authenticate("phil", "oldpass")
		require.ErrorIs(t, err, ErrUnauthenticated)

		// Token is single-use
		require.ErrorIs(t, a.ResetPassword(raw, "againpass"), ErrMagicLinkNotFound)
	})
}

func TestUser_MagicLink_ResetPassword_WrongKindRejected(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "oldpass", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		// An email-verification token must not be usable for password reset...
		verifyToken := addVerifyLink(t, a, phil.ID, "phil@example.com", time.Hour)
		require.ErrorIs(t, a.ResetPassword(verifyToken, "newpass"), ErrMagicLinkNotFound)

		// ...and a reset token must not be usable for email verification
		resetToken, err := a.AddMagicLink(MagicLinkKindPasswordReset, phil.ID, "", time.Hour)
		require.Nil(t, err)
		_, err = a.VerifyEmail(resetToken)
		require.ErrorIs(t, err, ErrMagicLinkNotFound)

		// Old password unchanged
		_, err = a.Authenticate("phil", "oldpass")
		require.Nil(t, err)
	})
}

func TestUser_MagicLink_VerifyEmail_ProvisionedGetsPrimary(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManagerFromConfig(t, newManager, &Config{
			DefaultAccess:    PermissionDenyAll,
			ProvisionEnabled: true,
			Users: []*User{
				{Name: "prov", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
			},
		})
		prov, err := a.User("prov")
		require.Nil(t, err)

		// A provisioned user's first verified email becomes their primary, just like a regular user
		// (the primary is also the X-Email: yes target; password reset stays blocked separately).
		_, err = a.VerifyEmail(addVerifyLink(t, a, prov.ID, "prov@example.com", time.Hour))
		require.Nil(t, err)

		emails, err := a.Emails(prov.ID)
		require.Nil(t, err)
		require.Equal(t, []string{"prov@example.com"}, emails.Strings())
		primary, err := a.PrimaryEmail(prov.ID)
		require.Nil(t, err)
		require.Equal(t, "prov@example.com", primary)
	})
}

func TestUser_MagicLink_ResetPassword_ProvisionedRejected(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		// Provisioned users come from the config file (ProvisionEnabled), not AddUser
		a := newTestManagerFromConfig(t, newManager, &Config{
			DefaultAccess:    PermissionDenyAll,
			ProvisionEnabled: true,
			Users: []*User{
				{Name: "prov", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: RoleUser},
			},
		})
		prov, err := a.User("prov")
		require.Nil(t, err)
		require.True(t, prov.Provisioned)

		// A reset token can be created, but consuming it must be rejected for a provisioned user
		// (their password comes from the config file, like change-pass).
		raw, err := a.AddMagicLink(MagicLinkKindPasswordReset, prov.ID, "", time.Hour)
		require.Nil(t, err)
		require.ErrorIs(t, a.ResetPassword(raw, "newpass"), ErrProvisionedUserChange)
	})
}

func TestUser_MagicLink_ResetPassword_Expired(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "oldpass", RoleUser, false))
		phil, err := a.User("phil")
		require.Nil(t, err)

		raw, err := a.AddMagicLink(MagicLinkKindPasswordReset, phil.ID, "", -time.Minute)
		require.Nil(t, err)
		require.ErrorIs(t, a.ResetPassword(raw, "newpass"), ErrMagicLinkNotFound)
		_, err = a.Authenticate("phil", "oldpass")
		require.Nil(t, err)
	})
}

func TestUser_MagicLink_UserIDByPrimaryEmail_NotFound(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		_, err := a.UserIDByPrimaryEmail("ghost@example.com")
		require.ErrorIs(t, err, ErrUserNotFound)
	})
}

func TestManager_Emails_PrimaryFlagAndHelpers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newManager newManagerFunc) {
		a := newTestManager(t, newManager, PermissionDenyAll)
		require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
		u, err := a.User("phil")
		require.Nil(t, err)
		require.Nil(t, a.AddEmail(u.ID, "a@example.com"))
		require.Nil(t, a.AddEmail(u.ID, "b@example.com"))
		require.Nil(t, a.SetPrimaryEmail(u.ID, "b@example.com"))

		// Emails() carries the primary flag, so a separate PrimaryEmail() call is unnecessary.
		emails, err := a.Emails(u.ID)
		require.Nil(t, err)
		require.Len(t, emails, 2)
		require.Equal(t, "a@example.com", emails[0].Address) // ORDER BY email
		require.False(t, emails[0].Primary)
		require.Equal(t, "b@example.com", emails[1].Address)
		require.True(t, emails[1].Primary)

		// Helper methods for the address-only callers
		require.Equal(t, []string{"a@example.com", "b@example.com"}, emails.Strings())
		require.True(t, emails.Contains("a@example.com"))
		require.False(t, emails.Contains("c@example.com"))
	})
}

// openReplicaTestSQLite opens a fresh SQLite database file with the user schema applied.
func openReplicaTestSQLite(t *testing.T, filename string) *sql.DB {
	d, err := sql.Open("sqlite3", filename+"?_case_sensitive_like=on")
	require.Nil(t, err)
	require.Nil(t, schema.Migrate(d, schema.SQLite, schemaStore, sqliteCurrentSchemaVersion, sqliteCreateTables, sqliteMigrations))
	return d
}

// TestManager_AccountReadsUsePrimary verifies that the per-user reads backing the GET /account
// endpoint read from the primary, not from a read replica. The sync event ("something changed")
// is published immediately after a write, so a replica that lags would make the account view
// stale right after the user changes it. These reads must therefore be read-your-writes consistent.
//
// The test wires up a primary and a deliberately-empty replica (simulating replication lag),
// forces the replica healthy so ReadOnly() would route to it, writes everything to the primary,
// and asserts the reads still observe the fresh primary data.
func TestManager_AccountReadsUsePrimary(t *testing.T) {
	dir := t.TempDir()
	primaryDB := openReplicaTestSQLite(t, filepath.Join(dir, "primary.db"))
	replicaDB := openReplicaTestSQLite(t, filepath.Join(dir, "replica.db")) // intentionally left empty

	pool := db.New(&db.Host{DB: primaryDB}, []*db.Host{{DB: replicaDB}})
	pool.MarkReplicasHealthyForTest() // force ReadOnly() to route to the stale replica
	a, err := newManager(pool, sqliteQueries, &Config{BcryptCost: bcrypt.MinCost})
	require.Nil(t, err)
	t.Cleanup(func() { a.Close() })

	// All writes below go to the primary; the replica stays empty.
	require.Nil(t, a.AddUser("phil", "phil", RoleUser, false))
	u, err := a.User("phil")
	require.Nil(t, err)
	_, err = a.CreateToken(u.ID, "test token", time.Now().Add(time.Hour), netip.IPv4Unspecified(), false)
	require.Nil(t, err)
	require.Nil(t, a.AddReservation("phil", "mytopic", PermissionDenyAll, 10))
	require.Nil(t, a.AddPhoneNumber(u.ID, "+12223334444"))
	require.Nil(t, a.AddEmail(u.ID, "phil@example.com"))
	require.Nil(t, a.SetPrimaryEmail(u.ID, "phil@example.com"))
	_, err = a.AddMagicLink(MagicLinkKindEmailVerify, u.ID, "pending@example.com", time.Hour)
	require.Nil(t, err)

	// Each read must observe the just-written primary data, NOT the empty replica.
	tokens, err := a.Tokens(u.ID)
	require.Nil(t, err)
	require.Len(t, tokens, 1)

	reservations, err := a.Reservations("phil")
	require.Nil(t, err)
	require.Len(t, reservations, 1)

	phoneNumbers, err := a.PhoneNumbers(u.ID)
	require.Nil(t, err)
	require.Len(t, phoneNumbers, 1)

	emails, err := a.Emails(u.ID)
	require.Nil(t, err)
	require.Len(t, emails, 1)
	require.Equal(t, "phil@example.com", emails[0].Address)
	require.True(t, emails[0].Primary)

	primaryEmail, err := a.PrimaryEmail(u.ID)
	require.Nil(t, err)
	require.Equal(t, "phil@example.com", primaryEmail)

	pendingEmails, err := a.PendingEmails(u.ID)
	require.Nil(t, err)
	require.Len(t, pendingEmails, 1)
}
