//go:build !noserver

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/mail"
	"heckel.io/ntfy/v2/user"
)

func TestDirectAdminAPI_Users(t *testing.T) {
	d := newTestDirectAdminAPI(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddTier(&client.AdminTier{Code: "pro", Name: "Pro"}))
	require.Nil(t, d.AddUser("ben", hash, "pro", ""))
	require.Nil(t, d.AddUser("phil", hash, "", "admin"))
	requireAdminErrorCode(t, client.AdminErrCodeUserExists, d.AddUser("ben", hash, "", ""))
	requireAdminErrorCode(t, client.AdminErrCodeRoleInvalid, d.AddUser("emma", hash, "", "superuser"))
	requireAdminErrorCode(t, client.AdminErrCodeTierNotFound, d.AddUser("emma", hash, "nope", ""))
	requireAdminErrorCode(t, client.AdminErrCodePasswordHashInvalid, d.AddUser("emma", "not-a-hash", "", ""))

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
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, err)

	// Change tier, role, password; remove
	require.Nil(t, d.ChangeTier("ben", client.AdminTierReset))
	requireAdminErrorCode(t, client.AdminErrCodeTierNotFound, d.ChangeTier("ben", "nope"))
	require.Nil(t, d.ChangeRole("phil", "user"))
	requireAdminErrorCode(t, client.AdminErrCodeRoleInvalid, d.ChangeRole("phil", "superuser"))
	require.Nil(t, d.ChangePassword("phil", hash))
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, d.ChangePassword("nobody", hash))
	require.Nil(t, d.RemoveUser("phil"))
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, d.RemoveUser("phil"))
	require.Error(t, d.RemoveUser(user.Everyone))
}

func TestDirectAdminAPI_Access(t *testing.T) {
	d := newTestDirectAdminAPI(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddUser("ben", hash, "", ""))
	require.Nil(t, d.AddUser("phil", hash, "", "admin"))

	require.Nil(t, d.AllowAccess("ben", "gold", "read-only"))
	require.Nil(t, d.AllowAccess(user.Everyone, "public", "read-write"))
	requireAdminErrorCode(t, client.AdminErrCodeAccessAdminUser, d.AllowAccess("phil", "gold", "read-only"))
	requireAdminErrorCode(t, client.AdminErrCodePermissionInvalid, d.AllowAccess("ben", "gold", "everything"))
	ben, err := d.User("ben")
	require.Nil(t, err)
	require.Equal(t, []*client.AdminGrant{{Topic: "gold", Permission: "read-only"}}, ben.Grants)

	require.Nil(t, d.ResetAccess("ben", ""))
	ben, err = d.User("ben")
	require.Nil(t, err)
	require.Empty(t, ben.Grants)
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, d.ResetAccess("nobody", ""))
	require.Nil(t, d.ResetAccess("", ""))
	everyone, err := d.User(user.Everyone)
	require.Nil(t, err)
	require.Empty(t, everyone.Grants)
}

func TestDirectAdminAPI_Tokens(t *testing.T) {
	d := newTestDirectAdminAPI(t)
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
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, err)

	tokens, err := d.Tokens("")
	require.Nil(t, err)
	require.Len(t, tokens, 2) // Admin tokens included, as over the socket
	tokens, err = d.Tokens("ben")
	require.Nil(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "label", tokens[0].Label)

	require.Nil(t, d.RemoveToken("ben", forever.Token))
	requireAdminErrorCode(t, client.AdminErrCodeTokenNotFound, d.RemoveToken("ben", forever.Token))
}

func TestDirectAdminAPI_Tiers(t *testing.T) {
	d := newTestDirectAdminAPI(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)

	require.Nil(t, d.AddTier(&client.AdminTier{Code: "pro", Name: "Pro", MessagesExpiryDuration: 3600}))
	requireAdminErrorCode(t, client.AdminErrCodeTierExists, d.AddTier(&client.AdminTier{Code: "pro", Name: "Pro"}))
	requireAdminErrorCode(t, client.AdminErrCodeTierCodeInvalid, d.AddTier(&client.AdminTier{Code: "not valid", Name: "X"}))
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
	requireAdminErrorCode(t, client.AdminErrCodeTierNotFound, d.UpdateTier(&client.AdminTier{Code: "nope", Name: "Nope"}))

	requireAdminErrorCode(t, client.AdminErrCodeTierInUse, d.RemoveTier("pro"))
	require.Nil(t, d.ChangeTier("ben", client.AdminTierReset))
	require.Nil(t, d.RemoveTier("pro"))
	requireAdminErrorCode(t, client.AdminErrCodeTierNotFound, d.RemoveTier("pro"))
}

func TestDirectAdminAPI_ResetPassword(t *testing.T) {
	d := newTestDirectAdminAPI(t)
	hash, err := user.HashPassword("ben", user.DefaultUserPasswordBcryptCost)
	require.Nil(t, err)
	require.Nil(t, d.AddUser("ben", hash, "", ""))

	reset, err := d.ResetPassword("ben", false)
	require.Nil(t, err)
	require.Regexp(t, `^https://ntfy\.example\.com/account/password/reset/.+`, reset.Link)
	_, err = d.ResetPassword("ben", true) // No SMTP configured
	require.Error(t, err)
	_, err = d.ResetPassword("nobody", false)
	requireAdminErrorCode(t, client.AdminErrCodeUserNotFound, err)

	d.smtpConf.SMTPAddr = "localhost:25"
	_, err = d.ResetPassword("ben", true)
	requireAdminErrorCode(t, client.AdminErrCodeUserNoPrimaryEmail, err)

	d.baseURL = ""
	_, err = d.ResetPassword("ben", false)
	requireAdminErrorCode(t, client.AdminErrCodeBaseURLNotConfigured, err)
}

func newTestDirectAdminAPI(t *testing.T) *directAdminAPI {
	manager, err := user.NewSQLiteManager(filepath.Join(t.TempDir(), "user.db"), "", &user.Config{
		DefaultAccess: user.PermissionDenyAll,
		BcryptCost:    user.DefaultUserPasswordBcryptCost,
	})
	require.Nil(t, err)
	d := &directAdminAPI{
		manager:  manager,
		baseURL:  "https://ntfy.example.com",
		smtpConf: &mail.Config{},
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func requireAdminErrorCode(t *testing.T, code int, err error) {
	require.True(t, isAdminErrorCode(err, code), "expected admin error %d, got %v", code, err)
}
