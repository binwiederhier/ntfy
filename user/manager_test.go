package user

import (
	"database/sql"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minBcryptTimingMillis = int64(50) // Ideally should be >100ms, but this should also run on a Raspberry Pi without massive resources

func TestManager_FullScenario_Default_DenyAll(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("phil", "phil", RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.AllowAccess("ben", "mytopic", PermissionReadWrite))
	require.Nil(t, a.AllowAccess("ben", "readme", PermissionRead))
	require.Nil(t, a.AllowAccess("ben", "writeme", PermissionWrite))
	require.Nil(t, a.AllowAccess("ben", "everyonewrite", PermissionDenyAll)) // How unfair!
	require.Nil(t, a.AllowAccess(Everyone, "announcements", PermissionRead))
	require.Nil(t, a.AllowAccess(Everyone, "everyonewrite", PermissionReadWrite))
	require.Nil(t, a.AllowAccess(Everyone, "up*", PermissionWrite)) // Everyone can write to /up*

	phil, err := a.Authenticate("phil", "phil")
	require.Nil(t, err)
	require.Equal(t, "phil", phil.Name)
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, RoleAdmin, phil.Role)

	philGrants, err := a.Grants("phil")
	require.Nil(t, err)
	require.Equal(t, []Grant{}, philGrants)

	ben, err := a.Authenticate("ben", "ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, RoleUser, ben.Role)

	benGrants, err := a.Grants("ben")
	require.Nil(t, err)
	require.Equal(t, []Grant{
		{"mytopic", PermissionReadWrite},
		{"writeme", PermissionWrite},
		{"readme", PermissionRead},
		{"everyonewrite", PermissionDenyAll},
	}, benGrants)

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
	a := newTestManager(t, PermissionDenyAll)
	require.Equal(t, ErrInvalidArgument, a.AddUser("  invalid  ", "pass", RoleAdmin))
	require.Equal(t, ErrInvalidArgument, a.AddUser("validuser", "pass", "invalid-role"))
}

func TestManager_AddUser_Timing(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	start := time.Now().UnixMilli()
	require.Nil(t, a.AddUser("user", "pass", RoleAdmin))
	require.GreaterOrEqual(t, time.Now().UnixMilli()-start, minBcryptTimingMillis)
}

func TestManager_Authenticate_Timing(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
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

func TestManager_MarkUserRemoved_RemoveDeletedUsers(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)

	// Create user, add reservations and token
	require.Nil(t, a.AddUser("user", "pass", RoleAdmin))
	require.Nil(t, a.AddReservation("user", "mytopic", PermissionRead))

	u, err := a.User("user")
	require.Nil(t, err)
	require.False(t, u.Deleted)

	token, err := a.CreateToken(u)
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

	_, err = a.db.Exec("UPDATE user SET deleted = ? WHERE id = ?", time.Now().Add(-1*(userHardDeleteAfterDuration+time.Hour)).Unix(), u.ID)
	require.Nil(t, err)
	require.Nil(t, a.RemoveDeletedUsers())

	_, err = a.User("user")
	require.Equal(t, ErrUserNotFound, err)
}

func TestManager_UserManagement(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("phil", "phil", RoleAdmin))
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
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
	require.True(t, strings.HasPrefix(phil.Hash, "$2a$10$"))
	require.Equal(t, RoleAdmin, phil.Role)

	philGrants, err := a.Grants("phil")
	require.Nil(t, err)
	require.Equal(t, []Grant{}, philGrants)

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, "ben", ben.Name)
	require.True(t, strings.HasPrefix(ben.Hash, "$2a$10$"))
	require.Equal(t, RoleUser, ben.Role)

	benGrants, err := a.Grants("ben")
	require.Nil(t, err)
	require.Equal(t, []Grant{
		{"mytopic", PermissionReadWrite},
		{"writeme", PermissionWrite},
		{"readme", PermissionRead},
		{"everyonewrite", PermissionDenyAll},
	}, benGrants)

	everyone, err := a.User(Everyone)
	require.Nil(t, err)
	require.Equal(t, "*", everyone.Name)
	require.Equal(t, "", everyone.Hash)
	require.Equal(t, RoleAnonymous, everyone.Role)

	everyoneGrants, err := a.Grants(Everyone)
	require.Nil(t, err)
	require.Equal(t, []Grant{
		{"everyonewrite", PermissionReadWrite},
		{"announcements", PermissionRead},
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
}

func TestManager_ChangePassword(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
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
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
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
}

func TestManager_Reservations(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.AddReservation("ben", "ztopic", PermissionDenyAll))
	require.Nil(t, a.AddReservation("ben", "readme", PermissionRead))
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
		Topic:    "ztopic",
		Owner:    PermissionReadWrite,
		Everyone: PermissionDenyAll,
	}, reservations[1])
}

func TestManager_ChangeRoleFromTierUserToAdmin(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.CreateTier(&Tier{
		Code:                     "pro",
		Name:                     "ntfy Pro",
		StripePriceID:            "price123",
		MessagesLimit:            5_000,
		MessagesExpiryDuration:   3 * 24 * time.Hour,
		EmailsLimit:              50,
		ReservationsLimit:        5,
		AttachmentFileSizeLimit:  52428800,
		AttachmentTotalSizeLimit: 524288000,
		AttachmentExpiryDuration: 24 * time.Hour,
	}))
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))
	require.Nil(t, a.ChangeTier("ben", "pro"))
	require.Nil(t, a.AddReservation("ben", "mytopic", PermissionDenyAll))

	ben, err := a.User("ben")
	require.Nil(t, err)
	require.Equal(t, RoleUser, ben.Role)
	require.Equal(t, "pro", ben.Tier.Code)
	require.Equal(t, int64(5000), ben.Tier.MessagesLimit)
	require.Equal(t, 3*24*time.Hour, ben.Tier.MessagesExpiryDuration)
	require.Equal(t, int64(50), ben.Tier.EmailsLimit)
	require.Equal(t, int64(5), ben.Tier.ReservationsLimit)
	require.Equal(t, int64(52428800), ben.Tier.AttachmentFileSizeLimit)
	require.Equal(t, int64(524288000), ben.Tier.AttachmentTotalSizeLimit)
	require.Equal(t, 24*time.Hour, ben.Tier.AttachmentExpiryDuration)

	benGrants, err := a.Grants("ben")
	require.Nil(t, err)
	require.Equal(t, 1, len(benGrants))
	require.Equal(t, PermissionReadWrite, benGrants[0].Allow)

	everyoneGrants, err := a.Grants(Everyone)
	require.Nil(t, err)
	require.Equal(t, 1, len(everyoneGrants))
	require.Equal(t, PermissionDenyAll, everyoneGrants[0].Allow)

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
}

func TestManager_Token_Valid(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
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
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	u, err := a.AuthenticateToken(strings.Repeat("x", 32)) // 32 == token length
	require.Nil(t, u)
	require.Equal(t, ErrUnauthenticated, err)

	u, err = a.AuthenticateToken("not long enough anyway")
	require.Nil(t, u)
	require.Equal(t, ErrUnauthenticated, err)
}

func TestManager_Token_Expire(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
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

func TestManager_Token_Extend(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	// Try to extend token for user without token
	u, err := a.User("ben")
	require.Nil(t, err)

	_, err = a.ExtendToken(u)
	require.Equal(t, errNoTokenProvided, err)

	// Create token for user
	token, err := a.CreateToken(u)
	require.Nil(t, err)
	require.NotEmpty(t, token.Value)

	userWithToken, err := a.AuthenticateToken(token.Value)
	require.Nil(t, err)

	time.Sleep(1100 * time.Millisecond)

	extendedToken, err := a.ExtendToken(userWithToken)
	require.Nil(t, err)
	require.Equal(t, token.Value, extendedToken.Value)
	require.True(t, token.Expires.Unix() < extendedToken.Expires.Unix())
}

func TestManager_Token_MaxCount_AutoDelete(t *testing.T) {
	a := newTestManager(t, PermissionDenyAll)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	// Try to extend token for user without token
	u, err := a.User("ben")
	require.Nil(t, err)

	// Tokens
	baseTime := time.Now().Add(24 * time.Hour)
	tokens := make([]string, 0)
	for i := 0; i < 12; i++ {
		token, err := a.CreateToken(u)
		require.Nil(t, err)
		require.NotEmpty(t, token.Value)
		tokens = append(tokens, token.Value)

		// Manually modify expiry date to avoid sorting issues (this is a hack)
		_, err = a.db.Exec(`UPDATE user_token SET expires=? WHERE token=?`, baseTime.Add(time.Duration(i)*time.Minute).Unix(), token.Value)
		require.Nil(t, err)
	}

	_, err = a.AuthenticateToken(tokens[0])
	require.Equal(t, ErrUnauthenticated, err)

	_, err = a.AuthenticateToken(tokens[1])
	require.Equal(t, ErrUnauthenticated, err)

	for i := 2; i < 12; i++ {
		userWithToken, err := a.AuthenticateToken(tokens[i])
		require.Nil(t, err, "token[%d]=%s failed", i, tokens[i])
		require.Equal(t, "ben", userWithToken.Name)
		require.Equal(t, tokens[i], userWithToken.Token)
	}

	var count int
	rows, err := a.db.Query(`SELECT COUNT(*) FROM user_token`)
	require.Nil(t, err)
	require.True(t, rows.Next())
	require.Nil(t, rows.Scan(&count))
	require.Equal(t, 10, count)
}

func TestManager_EnqueueStats(t *testing.T) {
	a, err := newManager(filepath.Join(t.TempDir(), "db"), "", PermissionReadWrite, 1500*time.Millisecond)
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

func TestManager_ChangeSettings(t *testing.T) {
	a, err := newManager(filepath.Join(t.TempDir(), "db"), "", PermissionReadWrite, 1500*time.Millisecond)
	require.Nil(t, err)
	require.Nil(t, a.AddUser("ben", "ben", RoleUser))

	// No settings
	u, err := a.User("ben")
	require.Nil(t, err)
	require.Nil(t, u.Prefs.Subscriptions)
	require.Nil(t, u.Prefs.Notification)
	require.Equal(t, "", u.Prefs.Language)

	// Save with new settings
	u.Prefs = &Prefs{
		Language: "de",
		Notification: &NotificationPrefs{
			Sound:       "ding",
			MinPriority: 2,
		},
		Subscriptions: []*Subscription{
			{
				ID:          "someID",
				BaseURL:     "https://ntfy.sh",
				Topic:       "mytopic",
				DisplayName: "My Topic",
			},
		},
	}
	require.Nil(t, a.ChangeSettings(u))

	// Read again
	u, err = a.User("ben")
	require.Nil(t, err)
	require.Equal(t, "de", u.Prefs.Language)
	require.Equal(t, "ding", u.Prefs.Notification.Sound)
	require.Equal(t, 2, u.Prefs.Notification.MinPriority)
	require.Equal(t, 0, u.Prefs.Notification.DeleteAfter)
	require.Equal(t, "someID", u.Prefs.Subscriptions[0].ID)
	require.Equal(t, "https://ntfy.sh", u.Prefs.Subscriptions[0].BaseURL)
	require.Equal(t, "mytopic", u.Prefs.Subscriptions[0].Topic)
	require.Equal(t, "My Topic", u.Prefs.Subscriptions[0].DisplayName)
}

func TestSqliteCache_Migration_From1(t *testing.T) {
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
	a := newTestManagerFromFile(t, filename, "", PermissionDenyAll, userStatsQueueWriterInterval)
	checkSchemaVersion(t, a.db)

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
	require.Equal(t, "stats", benGrants[0].TopicPattern)
	require.Equal(t, PermissionReadWrite, benGrants[0].Allow)
	require.Equal(t, "secret", benGrants[1].TopicPattern)
	require.Equal(t, PermissionRead, benGrants[1].Allow)

	require.Equal(t, "u_everyone", everyone.ID)
	require.Equal(t, Everyone, everyone.Name)
	require.Equal(t, RoleAnonymous, everyone.Role)
	require.Equal(t, 1, len(everyoneGrants))
	require.Equal(t, "stats", everyoneGrants[0].TopicPattern)
	require.Equal(t, PermissionRead, everyoneGrants[0].Allow)
}

func checkSchemaVersion(t *testing.T, db *sql.DB) {
	rows, err := db.Query(`SELECT version FROM schemaVersion`)
	require.Nil(t, err)
	require.True(t, rows.Next())

	var schemaVersion int
	require.Nil(t, rows.Scan(&schemaVersion))
	require.Equal(t, currentSchemaVersion, schemaVersion)
	require.Nil(t, rows.Close())
}

func newTestManager(t *testing.T, defaultAccess Permission) *Manager {
	return newTestManagerFromFile(t, filepath.Join(t.TempDir(), "user.db"), "", defaultAccess, userStatsQueueWriterInterval)
}

func newTestManagerFromFile(t *testing.T, filename, startupQueries string, defaultAccess Permission, statsWriterInterval time.Duration) *Manager {
	a, err := newManager(filename, startupQueries, defaultAccess, statsWriterInterval)
	require.Nil(t, err)
	return a
}
