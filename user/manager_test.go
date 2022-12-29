package user

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minBcryptTimingMillis = int64(50) // Ideally should be >100ms, but this should also run on a Raspberry Pi without massive resources

func TestManager_FullScenario_Default_DenyAll(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(Everyone, "everyonewrite", true, true))
	require.Nil(t, a.AllowAccess(Everyone, "up*", false, true)) // Everyone can write to /up*

	phil, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, RoleAdmin, phil.Role)
	require.Equal(t, []Grant{}, phil.Grants)

	ben, err := a.Authenticate("ben", "ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, RoleUser, ben.Role)
	require.Equal(t, []Grant{
		{"mytopic", true, true},
		{"writeme", false, true},
		{"readme", true, false},
		{"everyonewrite", false, false},
	}, ben.Grants)

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
}

func TestManager_AddUser_Invalid(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Equal(t, ErrInvalidArgument, a.AddUser("  invalid  ", "pass", RoleAdmin))
	require.Equal(t, ErrInvalidArgument, a.AddUser("validuser", "pass", "invalid-role"))
}

func TestManager_AddUser_Timing(t *testing.T) {
	a := newTestManager(t, false, false)
	start := time.Now().UnixMilli()
	require.Nil(t, a.AddUser("user", "pass", RoleAdmin))
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestManager_Authenticate_Timing(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("user", "pass", RoleAdmin))

	// Timing a correct attempt
	start := time.Now().UnixMilli()
	_, err := a.Authenticate("user", "pass")
	require.Nil(t, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing an incorrect attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("user", "INCORRECT")
	require.Equal(t, ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)

	// Timing a non-existing user attempt
	start = time.Now().UnixMilli()
	_, err = a.Authenticate("DOES-NOT-EXIST", "hithere")
	require.Equal(t, ErrUnauthenticated, err)
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestManager_UserManagement(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", false, false)) // How unfair!
	require.Nil(t, a.AllowAccess(Everyone, "announcements", true, false))
	require.Nil(t, a.AllowAccess(Everyone, "everyonewrite", true, true))

	// Query user details
	phil, err := a.User("phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, RoleAdmin, phil.Role)
	require.Equal(t, []Grant{}, phil.Grants)

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, RoleUser, ben.Role)
	require.Equal(t, []Grant{
		{"mytopic", true, true},
		{"writeme", false, true},
		{"readme", true, false},
		{"everyonewrite", false, false},
	}, ben.Grants)

	everyone, err := a.User(Everyone)
	require.Nil(t, err)
	require.Equal(t, "*", everyone.Name)
	require.Equal(t, "", everyone.Hash)
	require.Equal(t, RoleAnonymous, everyone.Role)
	require.Equal(t, []Grant{
		{"everyonewrite", true, true},
		{"announcements", true, false},
	}, everyone.Grants)

	// Ben: Before revoking
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true)) // Overwrite!
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))
	require.Nil(t, a.AllowAccess("ben", "writeme", false, true))
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
	require.Equal(t, ErrNotFound, err)

	users, err = a.Users()
	require.Nil(t, err)
	require.Equal(t, 2, len(users))
	require.Equal(t, "phil", users[0].Name)
	require.Equal(t, "*", users[1].Name)
}

func TestManager_ChangePassword(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("phil", "phil", RoleAdmin))

	_, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)

	require.Nil(t, a.ChangePassword("phil", "newpass"))
	_, err = a.Authenticate("phil", "phil")
	require.Equal(t, ErrUnauthenticated, err)
	_, err = a.Authenticate("phil", "newpass")
	require.Nil(t, err)
}

func TestManager_ChangeRole(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", true, true))
	require.Nil(t, a.AllowAccess("ben", "readme", true, false))

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, RoleUser, ben.Role)
	require.Equal(t, 2, len(ben.Grants))

	require.Nil(t, a.ChangeRole("ben", RoleAdmin))

	ben, err = a.User("ben")
	require.Nil(t, err)
	require.Equal(t, RoleAdmin, ben.Role)
	require.Equal(t, 0, len(ben.Grants))
}

func TestManager_Token_Valid(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	u, err := a.User("ben")
	require.Nil(t, err)

	// Create token for user
	token, err := a.CreateToken(u)
	require.Nil(t, err)
	require.NotEmpty(t, token.Value)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token.Expires.Unix())

	u2, err := a.AuthenticateToken(token.Value)
	require.Nil(t, err)
	require.Equal(t, u.Name, u2.Name)
	require.Equal(t, token.Value, u2.Token)

	// Remove token and auth again
	require.Nil(t, a.RemoveToken(u2))
	u3, err := a.AuthenticateToken(token.Value)
	require.Equal(t, ErrUnauthenticated, err)
	require.Nil(t, u3)
}

func TestManager_Token_Invalid(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	u, err := a.AuthenticateToken(strings.Repeat("x", 32)) // 32 == token length
	require.Nil(t, u)
	require.Equal(t, ErrUnauthenticated, err)

	u, err = a.AuthenticateToken("not long enough anyway")
	require.Nil(t, u)
	require.Equal(t, ErrUnauthenticated, err)
}

func TestManager_Token_Expire(t *testing.T) {
	a := newTestManager(t, false, false)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	u, err := a.User("ben")
	require.Nil(t, err)

	// Create tokens for user
	token1, err := a.CreateToken(u)
	require.Nil(t, err)
	require.NotEmpty(t, token1.Value)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token1.Expires.Unix())

	token2, err := a.CreateToken(u)
	require.Nil(t, err)
	require.NotEmpty(t, token2.Value)
	require.NotEqual(t, token1.Value, token2.Value)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token2.Expires.Unix())

	// See that tokens work
	_, err = a.AuthenticateToken(token1.Value)
	require.Nil(t, err)

	_, err = a.AuthenticateToken(token2.Value)
	require.Nil(t, err)

	// Modify token expiration in database
	_, err = a.db.Exec("UPDATE user_token SET expires = 1 WHERE token = ?", token1.Value)
	require.Nil(t, err)

	// Now token1 shouldn't work anymore
	_, err = a.AuthenticateToken(token1.Value)
	require.Equal(t, ErrUnauthenticated, err)

	result, err := a.db.Query("SELECT * from user_token WHERE token = ?", token1.Value)
	require.Nil(t, err)
	require.True(t, result.Next()) // Still a matching row
	require.Nil(t, result.Close())

	// Expire tokens and check database rows
	require.Nil(t, a.RemoveExpiredTokens())

	result, err = a.db.Query("SELECT * from user_token WHERE token = ?", token1.Value)
	require.Nil(t, err)
	require.False(t, result.Next()) // No matching row!
	require.Nil(t, result.Close())
}

func TestManager_EnqueueStats(t *testing.T) {
	a, err := newManager(filepath.Join(t.TempDir(), "db"), true, true, time.Hour, 1500*time.Millisecond)
	require.Nil(t, err)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	// Baseline: No messages or emails
	u, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, int64(0), u.Stats.Messages)
	require.Equal(t, int64(0), u.Stats.Emails)

	u.Stats.Messages = 11
	u.Stats.Emails = 2
	a.EnqueueStats(u)

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
}

func newTestManager(t *testing.T, defaultRead, defaultWrite bool) *Manager {
	a, err := NewManager(filepath.Join(t.TempDir(), "db"), defaultRead, defaultWrite)
	require.Nil(t, err)
	return a
}
