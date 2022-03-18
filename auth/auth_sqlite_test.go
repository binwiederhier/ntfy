package auth_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/auth"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minBcryptTimingMillis = int64(50) // Ideally should be >100ms, but this should also run on a Raspberry Pi without massive resources

func TestSQLiteAuth_FullScenario_Default_DenyAll(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", auth.RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", auth.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(auth.Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(auth.Everyone, "everyonewrite", true, true))
	require.Nil(t, a.AllowAccess(auth.Everyone, "up*", false, true)) // Everyone can write to /up*

	phil, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, auth.RoleAdmin, phil.Role)
	require.Equal(t, []auth.Grant{}, phil.Grants)

	ben, err := a.Authenticate("ben", "ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, auth.RoleUser, ben.Role)
	require.Equal(t, []auth.Grant{
		{"mytopic", true, true},
		{"readme", true, false},
		{"writeme", false, true},
		{"everyonewrite", false, false},
	}, ben.Grants)

	notben, err := a.Authenticate("ben", "this is wrong")
	require.Nil(t, notben)
	require.Equal(t, auth.ErrUnauthenticated, err)

	// Admin can do everything
	require.Nil(t, a.Authorize(phil, "sometopic", auth.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "mytopic", auth.PermissionRead))
	require.Nil(t, a.Authorize(phil, "readme", auth.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "writeme", auth.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "announcements", auth.PermissionWrite))
	require.Nil(t, a.Authorize(phil, "everyonewrite", auth.PermissionWrite))

	// User cannot do everything
	require.Nil(t, a.Authorize(ben, "mytopic", auth.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "mytopic", auth.PermissionRead))
	require.Nil(t, a.Authorize(ben, "readme", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "readme", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "writeme", auth.PermissionRead))
	require.Nil(t, a.Authorize(ben, "writeme", auth.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "writeme", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "everyonewrite", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "everyonewrite", auth.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "announcements", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "announcements", auth.PermissionWrite))

	// Everyone else can do barely anything
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "sometopicnotinthelist", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "mytopic", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "mytopic", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "readme", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "readme", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "writeme", auth.PermissionRead))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "writeme", auth.PermissionWrite))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(nil, "announcements", auth.PermissionWrite))
	require.Nil(t, a.Authorize(nil, "announcements", auth.PermissionRead))
	require.Nil(t, a.Authorize(nil, "everyonewrite", auth.PermissionRead))
	require.Nil(t, a.Authorize(nil, "everyonewrite", auth.PermissionWrite))
	require.Nil(t, a.Authorize(nil, "up1234", auth.PermissionWrite)) // Wildcard permission
	require.Nil(t, a.Authorize(nil, "up5678", auth.PermissionWrite))
}

func TestSQLiteAuth_AddUser_Invalid(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Equal(t, auth.ErrInvalidArgument, a.AddUser("  invalid  ", "pass", auth.RoleAdmin))
	require.Equal(t, auth.ErrInvalidArgument, a.AddUser("validuser", "pass", "invalid-role"))
}

func TestSQLiteAuth_AddUser_Timing(t *testing.T) {
	a := newTestAuth(t, false, false)
	start := time.Now().UnixMilli()
	require.Nil(t, a.AddUser("user", "pass", auth.RoleAdmin))
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestSQLiteAuth_Authenticate_Timing(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("user", "pass", auth.RoleAdmin))

	// Timing a correct attempt
	start := time.Now().UnixMilli()
	_, err := a.Authenticate("user", "pass")
	require.Nil(t, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing an incorrect attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("user", "INCORRECT")
	require.Equal(t, auth.ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing a non-existing user attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("DOES-NOT-EXIST", "hithere")
	require.Equal(t, auth.ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestSQLiteAuth_UserManagement(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", auth.RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", auth.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(auth.Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(auth.Everyone, "everyonewrite", true, true))

	// Query user details
	phil, err := a.User("phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, auth.RoleAdmin, phil.Role)
	require.Equal(t, []auth.Grant{}, phil.Grants)

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, auth.RoleUser, ben.Role)
	require.Equal(t, []auth.Grant{
		{"mytopic", true, true},
		{"readme", true, false},
		{"writeme", false, true},
		{"everyonewrite", false, false},
	}, ben.Grants)

	everyone, err := a.User(auth.Everyone)
	require.Nil(t, err)
	require.Equal(t, "*", everyone.Name)
	require.Equal(t, "", everyone.Hash)
	require.Equal(t, auth.RoleAnonymous, everyone.Role)
	require.Equal(t, []auth.Grant{
		{"announcements", true, false},
		{"everyonewrite", true, true},
	}, everyone.Grants)

	// Ben: Before revoking
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.Authorize(ben, "mytopic", auth.PermissionRead))
	require.Nil(t, a.Authorize(ben, "mytopic", auth.PermissionWrite))
	require.Nil(t, a.Authorize(ben, "readme", auth.PermissionRead))
	require.Nil(t, a.Authorize(ben, "writeme", auth.PermissionWrite))

	// Revoke access for "ben" to "mytopic", then check again
	require.Nil(t, a.ResetAccess("ben", "mytopic"))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "mytopic", auth.PermissionWrite)) // Revoked
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "mytopic", auth.PermissionRead))  // Revoked
	require.Nil(t, a.Authorize(ben, "readme", auth.PermissionRead))                           // Unchanged
	require.Nil(t, a.Authorize(ben, "writeme", auth.PermissionWrite))                         // Unchanged

	// Revoke rest of the access
	require.Nil(t, a.ResetAccess("ben", ""))
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "readme", auth.PermissionRead))    // Revoked
	require.Equal(t, auth.ErrUnauthorized, a.Authorize(ben, "wrtiteme", auth.PermissionWrite)) // Revoked

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
	require.Equal(t, auth.ErrNotFound, err)

	users, err = a.Users()
	require.Nil(t, err)
	require.Equal(t, 2, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "*", users[1].Name)
}

func TestSQLiteAuth_ChangePassword(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", auth.RoleAdmin))

	_, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)

	require.Nil(t, a.ChangePassword("phil", "newpass"))
	_, err = a.Authenticate("phil", "phil")
	require.Equal(t, auth.ErrUnauthenticated, err)
	_, err = a.Authenticate("phil", "newpass")
	require.Nil(t, err)
}

func TestSQLiteAuth_ChangeRole(t *testing.T) {
	a := newTestAuth(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", auth.RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, auth.RoleUser, ben.Role)
	require.Equal(t, 2, len(ben.Grants))

	require.Nil(t, a.ChangeRole("ben", auth.RoleAdmin))

	ben, err = a.User("ben")
	require.Nil(t, err)
	require.Equal(t, auth.RoleAdmin, ben.Role)
	require.Equal(t, 0, len(ben.Grants))
}

func newTestAuth(t *testing.T, defaultRead, defaultWrite bool) *auth.SQLiteAuth {
	filename := filepath.Join(t.TempDir(), "user.db")
	a, err := auth.NewSQLiteAuth(filename, defaultRead, defaultWrite)
	require.Nil(t, err)
	return a
}
