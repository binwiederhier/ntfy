package user_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minBcryptTimingMillis = int64(50) // Ideally should be >100ms, but this should also run on a Raspberry Pi without massive resources

func TestSQLiteAuth_FullScenario_Default_DenyAll(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(user.Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(user.Everyone, "everyonewrite", true, true))
	require.Nil(t, a.AllowAccess(user.Everyone, "up*", false, true)) // Everyone can write to /up*

	phil, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, user.RoleAdmin, phil.Role)
	require.Equal(t, []user.Grant{}, phil.Grants)

	ben, err := a.Authenticate("ben", "ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, user.RoleUser, ben.Role)
	require.Equal(t, []user.Grant{
		{"mytopic", true, true},
		{"writeme", false, true},
		{"readme", true, false},
		{"everyonewrite", false, false},
	}, ben.Grants)

	notben, err := a.Authenticate("ben", "this is wrong")
	require.Nil(t, notben)
	require.Equal(t, user.ErrUnauthenticated, err)

	// Admin can do everything
	require.Nil(t, a.Authorize(phil, "sometopic", user.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "mytopic", user.PermissionRead))
	require.Nil(t, a.Authorize(phil, "readme", user.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "writeme", user.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "announcements", user.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "everyonewrite", user.PermissionWrite))

	// User cannot do everything
	require.Nil(t, a.Authorize(ben, "mytopic", user.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "mytopic", user.PermissionRead))
	require.Nil(t, a.Authorize(ben, "readme", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "readme", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "writeme", user.PermissionRead))
	require.Nil(t, a.Authorize(ben, "writeme", user.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "writeme", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "everyonewrite", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "everyonewrite", user.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "announcements", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "announcements", user.PermissionWrite))

	// Everyone else can do barely anything
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "mytopic", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "mytopic", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "readme", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "readme", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "writeme", user.PermissionRead))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "writeme", user.PermissionWrite))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(nil, "announcements", user.PermissionWrite))
	require.Nil(t, a.Authorize(nil, "announcements", user.PermissionRead))
	require.Nil(t, a.Authorize(nil, "everyonewrite", user.PermissionRead))
	require.Nil(t, a.Authorize(nil, "everyonewrite", user.PermissionWrite))
	require.Nil(t, a.Authorize(nil, "up1234", user.PermissionWrite)) // Wildcard permission
	require.Nil(t, a.Authorize(nil, "up5678", user.PermissionWrite))
}

func TestSQLiteAuth_AddUser_Invalid(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Equal(t, user.ErrInvalidArgument, a.AddUser("  invalid  ", "pass", user.RoleAdmin))
	require.Equal(t, user.ErrInvalidArgument, a.AddUser("validuser", "pass", "invalid-role"))
}

func TestSQLiteAuth_AddUser_Timing(t *testing.T) {
	a := newTestAuth(t, false, false)
	start := time.Now().UnixMilli()
	require.Nil(t, a.AddUser("user", "pass", user.RoleAdmin))
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestSQLiteAuth_Authenticate_Timing(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("user", "pass", user.RoleAdmin))

	// Timing a correct attempt
	start := time.Now().UnixMilli()
	_, err := a.Authenticate("user", "pass")
	require.Nil(t, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing an incorrect attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("user", "INCORRECT")
	require.Equal(t, user.ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing a non-existing user attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("DOES-NOT-EXIST", "hithere")
	require.Equal(t, user.ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestSQLiteAuth_UserManagement(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(user.Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(user.Everyone, "everyonewrite", true, true))

	// Query user details
	phil, err := a.User("phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, user.RoleAdmin, phil.Role)
	require.Equal(t, []user.Grant{}, phil.Grants)

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, user.RoleUser, ben.Role)
	require.Equal(t, []user.Grant{
		{"mytopic", true, true},
		{"writeme", false, true},
		{"readme", true, false},
		{"everyonewrite", false, false},
	}, ben.Grants)

	everyone, err := a.User(user.Everyone)
	require.Nil(t, err)
	require.Equal(t, "*", everyone.Name)
	require.Equal(t, "", everyone.Hash)
	require.Equal(t, user.RoleAnonymous, everyone.Role)
	require.Equal(t, []user.Grant{
		{"everyonewrite", true, true},
		{"announcements", true, false},
	}, everyone.Grants)

	// Ben: Before revoking
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true)) // Overwrite!
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.Authorize(ben, "mytopic", user.PermissionRead))
	require.Nil(t, a.Authorize(ben, "mytopic", user.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "readme", user.PermissionRead))
	require.Nil(t, a.Authorize(ben, "writeme", user.PermissionWrite))

	// Revoke access for "ben" to "mytopic", then check again
	require.Nil(t, a.ResetAccess("ben", "mytopic"))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "mytopic", user.PermissionWrite)) // Revoked
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "mytopic", user.PermissionRead))  // Revoked
	require.Nil(t, a.Authorize(ben, "readme", user.PermissionRead))                           // Unchanged
	require.Nil(t, a.Authorize(ben, "writeme", user.PermissionWrite))                         // Unchanged

	// Revoke rest of the access
	require.Nil(t, a.ResetAccess("ben", ""))
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "readme", user.PermissionRead))    // Revoked
	require.Equal(t, user.ErrUnauthorized, a.Authorize(ben, "wrtiteme", user.PermissionWrite)) // Revoked

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
	require.Equal(t, user.ErrNotFound, err)

	users, err = a.Users()
	require.Nil(t, err)
	require.Equal(t, 2, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "*", users[1].Name)
}

func TestSQLiteAuth_ChangePassword(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", user.RoleAdmin))

	_, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)

	require.Nil(t, a.ChangePassword("phil", "newpass"))
	_, err = a.Authenticate("phil", "phil")
	require.Equal(t, user.ErrUnauthenticated, err)
	_, err = a.Authenticate("phil", "newpass")
	require.Nil(t, err)
}

func TestSQLiteAuth_ChangeRole(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, user.RoleUser, ben.Role)
	require.Equal(t, 2, len(ben.Grants))

	require.Nil(t, a.ChangeRole("ben", user.RoleAdmin))

	ben, err = a.User("ben")
	require.Nil(t, err)
	require.Equal(t, user.RoleAdmin, ben.Role)
	require.Equal(t, 0, len(ben.Grants))
}

func newTestAuth(t *testing.T, defaultRead, defaultWrite bool) *user.Manager {
	filename := filepath.Join(t.TempDir(), "user.db")
	a, err := user.NewManager(filename, defaultRead, defaultWrite)
	require.Nil(t, err)
	return a
}
