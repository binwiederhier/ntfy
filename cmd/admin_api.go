//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/client"
)

const (
	adminFallbackWarning = "warning: no ntfy server answered on the admin socket %s (%s), writing to the database directly\n"
)

// adminAPI is what the user, access, token and tier commands need. client.Admin implements it
// over the admin socket or HTTP; directAdminAPI implements it against the database (the fallback).
type adminAPI interface {
	Users() ([]*client.AdminUser, error)
	User(username string) (*client.AdminUser, error)
	AddUser(username, hash, tier, role string) error
	ChangePassword(username, hash string) error
	ChangeTier(username, tier string) error
	ChangeRole(username, role string) error
	RemoveUser(username string) error
	ResetPassword(username string, sendEmail bool) (*client.AdminPasswordReset, error)
	AllowAccess(username, topic, permission string) error
	ResetAccess(username, topic string) error
	Tokens(username string) ([]*client.AdminToken, error)
	AddToken(username, label string, expires time.Time) (*client.AdminToken, error)
	RemoveToken(username, token string) error
	Tiers() ([]*client.AdminTier, error)
	Tier(code string) (*client.AdminTier, error)
	AddTier(t *client.AdminTier) error
	UpdateTier(t *client.AdminTier) error
	RemoveTier(code string) error
	Close() error
}

// newAdminAPI picks the transport: an explicit --server (never falls back), else the admin
// socket when a server answers on it, else the database directly, with a warning. A socket file
// that nobody answers on is treated as absent (stale file after a crash).
func newAdminAPI(c *cli.Context) (adminAPI, error) {
	if server := c.String("server"); server != "" {
		token := c.String("token")
		if token == "" {
			return nil, errors.New("--token is required with --server")
		}
		return client.NewAdmin(server, token), nil
	}
	authConfigured := c.String("auth-file") != "" || c.String("database-url") != ""
	if socket := c.String("admin-socket"); socket != "" && authConfigured {
		admin := client.NewAdminSocket(socket)
		err := admin.Ping()
		var apiErr *client.AdminError
		switch {
		case err == nil:
			return admin, nil
		case errors.As(err, &apiErr) && apiErr.Code == client.AdminErrCodeNotFound:
			// The server runs without auth, i.e. it does not use this database; createUserManager decides
		case errors.Is(err, fs.ErrPermission):
			// Either this user may not manage the server, or the socket is some other server's (e.g. the
			// system's, while managing one's own instance). Only the database can tell the two apart.
			direct, directErr := newDirectAdminAPI(c)
			if directErr != nil {
				return nil, fmt.Errorf("permission denied on the admin socket %s, run this command as root or as a member of the server's group (usually ntfy)", socket)
			}
			fmt.Fprintf(c.App.ErrWriter, adminFallbackWarning, socket, adminUnreachableReason(err))
			return direct, nil
		case errors.Is(err, client.ErrAdminUnreachable):
			fmt.Fprintf(c.App.ErrWriter, adminFallbackWarning, socket, adminUnreachableReason(err))
		default:
			return nil, err // The server answered with some other error; do not hide it behind a fallback
		}
	}
	return newDirectAdminAPI(c)
}

// adminError turns an admin API error into the message the CLI printed before it talked to the
// server, so scripts and tests see the same text no matter how the command reached the database.
// The wording lives only here.
func adminError(err error, username, tier, token string) error {
	var apiErr *client.AdminError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Code {
	case client.AdminErrCodeUserNotFound:
		return fmt.Errorf("user %s does not exist", username)
	case client.AdminErrCodeUserExists:
		return fmt.Errorf("user %s already exists", username)
	case client.AdminErrCodeTierNotFound:
		return fmt.Errorf("tier %s does not exist", tier)
	case client.AdminErrCodeTierExists:
		return fmt.Errorf("tier %s already exists", tier)
	case client.AdminErrCodeTierInUse:
		return fmt.Errorf("tier %s is still assigned to users", tier)
	case client.AdminErrCodeTierReservationsLimit:
		return fmt.Errorf("user %s has more topic reservations than tier %s allows, remove reservations first", username, tier)
	case client.AdminErrCodeTokenNotFound:
		return fmt.Errorf("token %s for user %s does not exist", token, username)
	case client.AdminErrCodeAccessAdminUser:
		return fmt.Errorf("user %s is an admin user, access control entries have no effect", username)
	case client.AdminErrCodeProvisionedUserChange:
		return fmt.Errorf("user %s is provisioned in the config file and cannot be changed", username)
	case client.AdminErrCodeProvisionedTokenChange:
		return fmt.Errorf("token %s is provisioned in the config file and cannot be changed", token)
	case client.AdminErrCodeUserNoPrimaryEmail:
		return fmt.Errorf("user %s has no primary email; cannot send reset link (omit --send-email to just print it)", username)
	case client.AdminErrCodeBaseURLNotConfigured:
		return errors.New("base-url must be configured to generate a reset link")
	case client.AdminErrCodeRoleInvalid:
		return errors.New("role must be either 'user' or 'admin'")
	case client.AdminErrCodeTierCodeInvalid:
		return errors.New("tier code must consist only of numbers and letters, and the tier needs a name")
	case client.AdminErrCodePermissionInvalid:
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	case client.AdminErrCodePasswordHashInvalid:
		return errors.New("password hash must be a bcrypt hash with cost 10 or higher, use 'ntfy user hash' to generate")
	case client.AdminErrCodeAdminSocketOnly:
		return errors.New("changing roles or admin users is only possible over the admin socket or directly in the database, not over --server")
	case client.AdminErrCodeNotFound:
		return errors.New("the server does not support this command, upgrade the server or use direct database access")
	case client.AdminErrCodeUnauthorized:
		return errors.New("unauthorized: --token must belong to an admin user")
	}
	return apiErr
}

// adminUnreachableReason extracts the cause from a wrapped client.ErrAdminUnreachable, e.g.
// "no such file or directory", for the fallback warning
func adminUnreachableReason(err error) string {
	reason := strings.TrimPrefix(err.Error(), client.ErrAdminUnreachable.Error()+": ")
	if i := strings.LastIndex(reason, ": "); i >= 0 {
		reason = reason[i+2:]
	}
	return reason
}
