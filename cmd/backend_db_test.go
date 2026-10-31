//go:build !noserver

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cmd/client"
	"heckel.io/ntfy/v2/mail"
	"heckel.io/ntfy/v2/user"
)

func TestDBBackend_Users(t *testing.T) {
	d := newTestDBBackend(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddTier(&client.Tier{Code: "pro", Name: "Pro"}))
	require.Nil(t, d.AddUser("ben", hash, "pro", ""))
	require.Nil(t, d.AddUser("phil", hash, "", "admin"))
	requireAPIErrorCode(t, client.ErrCodeUserExists, d.AddUser("ben", hash, "", ""))
	requireAPIErrorCode(t, client.ErrCodeRoleInvalid, d.AddUser("emma", hash, "", "superuser"))
	requireAPIErrorCode(t, client.ErrCodeTierNotFound, d.AddUser("emma", hash, "nope", ""))
	requireAPIErrorCode(t, client.ErrCodePasswordHashInvalid, d.AddUser("emma", "not-a-hash", "", ""))

	// Users and User
	users, err := d.Users()
	require.Nil(t, err)
	require.Len(t, users, 3)
	ben, err := d.User("ben")
	require.Nil(t, err)
	require.Equal(t, "Pro", ben.TierName)
	everyone, err := d.User(user.Everyone)
	require.Nil(t, err)
	require.Equal(t, "deny-all", everyone.DefaultAccess)
	_, err = d.User("nobody")
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, err)

	// Change tier, role, password; remove
	require.Nil(t, d.ChangeTier("ben", tierReset))
	requireAPIErrorCode(t, client.ErrCodeTierNotFound, d.ChangeTier("ben", "nope"))
	require.Nil(t, d.ChangeRole("phil", "user"))
	requireAPIErrorCode(t, client.ErrCodeRoleInvalid, d.ChangeRole("phil", "superuser"))
	require.Nil(t, d.ChangePassword("phil", hash))
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, d.ChangePassword("nobody", hash))
	require.Nil(t, d.RemoveUser("phil"))
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, d.RemoveUser("phil"))
	require.Error(t, d.RemoveUser(user.Everyone))
}

func TestDBBackend_Access(t *testing.T) {
	d := newTestDBBackend(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddUser("ben", hash, "", ""))
	require.Nil(t, d.AddUser("phil", hash, "", "admin"))

	require.Nil(t, d.AllowAccess("ben", "gold", "read-only"))
	require.Nil(t, d.AllowAccess(user.Everyone, "public", "read-write"))
	requireAPIErrorCode(t, client.ErrCodeAccessAdminUser, d.AllowAccess("phil", "gold", "read-only"))
	requireAPIErrorCode(t, client.ErrCodePermissionInvalid, d.AllowAccess("ben", "gold", "everything"))
	ben, err := d.User("ben")
	require.Nil(t, err)
	require.Equal(t, []*client.Grant{{Topic: "gold", Permission: "read-only"}}, ben.Grants)

	require.Nil(t, d.ResetAccess("ben", ""))
	ben, err = d.User("ben")
	require.Nil(t, err)
	require.Empty(t, ben.Grants)
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, d.ResetAccess("nobody", ""))
	require.Nil(t, d.ResetAccess("", ""))
	everyone, err := d.User(user.Everyone)
	require.Nil(t, err)
	require.Empty(t, everyone.Grants)
}

func TestDBBackend_Tokens(t *testing.T) {
	d := newTestDBBackend(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddUser("ben", hash, "", ""))
	require.Nil(t, d.AddUser("phil", hash, "", "admin"))

	forever, err := d.AddToken("ben", "label", time.Time{})
	require.Nil(t, err)
	require.Equal(t, int64(0), forever.Expires)
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	expiring, err := d.AddToken("phil", "", expires)
	require.Nil(t, err)
	require.Equal(t, expires.Unix(), expiring.Expires)
	_, err = d.AddToken("nobody", "", time.Time{})
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, err)

	tokens, err := d.Tokens("")
	require.Nil(t, err)
	require.Len(t, tokens, 2) // Admin tokens included, as over the socket
	tokens, err = d.Tokens("ben")
	require.Nil(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "label", tokens[0].Label)

	require.Nil(t, d.RemoveToken("ben", forever.Token))
	requireAPIErrorCode(t, client.ErrCodeTokenNotFound, d.RemoveToken("ben", forever.Token))
}

func TestDBBackend_Tiers(t *testing.T) {
	d := newTestDBBackend(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)

	require.Nil(t, d.AddTier(&client.Tier{Code: "pro", Name: "Pro", MessagesExpiryDuration: 3600}))
	requireAPIErrorCode(t, client.ErrCodeTierExists, d.AddTier(&client.Tier{Code: "pro", Name: "Pro"}))
	requireAPIErrorCode(t, client.ErrCodeTierCodeInvalid, d.AddTier(&client.Tier{Code: "not valid", Name: "X"}))
	require.Nil(t, d.AddUser("ben", hash, "pro", ""))

	tier, err := d.Tier("pro")
	require.Nil(t, err)
	require.Equal(t, int64(3600), tier.MessagesExpiryDuration)
	require.Equal(t, int64(1), tier.Users)
	tier.Name = "Pro Plus"
	require.Nil(t, d.UpdateTier(tier))
	tiers, err := d.Tiers()
	require.Nil(t, err)
	require.Equal(t, "Pro Plus", tiers[0].Name)
	requireAPIErrorCode(t, client.ErrCodeTierNotFound, d.UpdateTier(&client.Tier{Code: "nope", Name: "Nope"}))

	requireAPIErrorCode(t, client.ErrCodeTierInUse, d.RemoveTier("pro"))
	require.Nil(t, d.ChangeTier("ben", tierReset))
	require.Nil(t, d.RemoveTier("pro"))
	requireAPIErrorCode(t, client.ErrCodeTierNotFound, d.RemoveTier("pro"))
}

func TestDBBackend_ResetPassword(t *testing.T) {
	d := newTestDBBackend(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddUser("ben", hash, "", ""))

	reset, err := d.ResetPassword("ben", false)
	require.Nil(t, err)
	require.Regexp(t, `^https://ntfy\.example\.com/account/password/reset/.+`, reset.Link)
	_, err = d.ResetPassword("ben", true) // No SMTP configured
	require.Error(t, err)
	_, err = d.ResetPassword("nobody", false)
	requireAPIErrorCode(t, client.ErrCodeUserNotFound, err)

	d.smtpConf.SMTPAddr = "localhost:25"
	_, err = d.ResetPassword("ben", true)
	requireAPIErrorCode(t, client.ErrCodeUserNoPrimaryEmail, err)

	d.baseURL = ""
	_, err = d.ResetPassword("ben", false)
	requireAPIErrorCode(t, client.ErrCodeBaseURLNotConfigured, err)
}

func newTestDBBackend(t *testing.T) *dbBackend {
	userDB, err := user.OpenSQLite(filepath.Join(t.TempDir(), "user.db"))
	require.Nil(t, err)
	manager, err := user.NewSQLiteManager(userDB, "", &user.Config{
		DefaultAccess: user.PermissionDenyAll,
		BcryptCost:    user.DefaultUserPasswordBcryptCost,
	})
	require.Nil(t, err)
	d := &dbBackend{
		manager:  manager,
		baseURL:  "https://ntfy.example.com",
		smtpConf: &mail.Config{},
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func requireAPIErrorCode(t *testing.T, code int, err error) {
	require.True(t, isAPIErrorCode(err, code), "expected admin error %d, got %v", code, err)
}
