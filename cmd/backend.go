//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/cmd/client"
)

const (
	adminFallbackWarningBanner = "**********************************************************************"
	adminFallbackWarningText   = `DEPRECATION WARNING: %s, writing to the database
directly. A running server may not see this change right away.

Direct database access will be removed after 2026-12-30. After that, the
ntfy user/access/token/tier commands need a running server.
See https://docs.ntfy.sh/deprecations/ for details.`
	adminSocketDisabledCause = "The admin socket is disabled\n(listen-admin-unix is \"-\" or empty)"
)

// backend is what the user, access, token and tier commands need. client.Client implements it
// over the admin socket; dbBackend implements it against the database (the fallback).
type backend interface {
	Users() ([]*client.User, error)
	User(username string) (*client.User, error)
	AddUser(username, hash, tier, role string) error
	ChangePassword(username, hash string) error
	ChangeTier(username, tier string) error
	ChangeRole(username, role string) error
	RemoveUser(username string) error
	ResetPassword(username string, sendEmail bool) (*client.PasswordReset, error)
	AllowAccess(username, topic, permission string) error
	ResetAccess(username, topic string) error
	Tokens(username string) ([]*client.Token, error)
	AddToken(username, label string, expires time.Time) (*client.Token, error)
	RemoveToken(username, token string) error
	Tiers() ([]*client.Tier, error)
	Tier(code string) (*client.Tier, error)
	AddTier(t *client.Tier) error
	UpdateTier(t *client.Tier) error
	RemoveTier(code string) error
	Close() error
}

// newBackend picks the transport: the admin socket when a server answers on it, else the database
// directly, with a warning. A socket file that nobody answers on is treated as absent (stale file
// after a crash).
func newBackend(c *cli.Context) (backend, error) {
	authConfigured := c.String("auth-file") != "" || c.String("database-url") != ""
	socket := c.String("listen-admin-unix")
	if !authConfigured {
		return newDBBackend(c) // Fails with the message for the missing auth config
	} else if socket == "" || socket == "-" { // "-" disables it, as in the server
		printAdminFallbackWarning(c.App.ErrWriter, adminSocketDisabledCause)
		return newDBBackend(c)
	}
	if err := checkAdminSocketOwner(socket, c.String("auth-file")); err != nil {
		return nil, err
	}
	sc := client.New(socket)
	err := sc.Ping()
	var apiErr *client.Error
	switch {
	case err == nil:
		return sc, nil
	case errors.As(err, &apiErr) && apiErr.Code == client.ErrCodeNotFound:
		// The server runs without auth, i.e. it does not use this database; createUserManager decides
	case errors.Is(err, fs.ErrPermission):
		// Either this user may not manage the server, or the socket is some other server's (e.g. the
		// system's, while managing one's own instance). Only the database can tell the two apart.
		direct, directErr := newDBBackend(c)
		if directErr != nil {
			return nil, fmt.Errorf("permission denied on the admin socket %s, run this command as root or as a member of the server's group (usually ntfy)", socket)
		}
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
		return direct, nil
	case errors.Is(err, client.ErrUnreachable):
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
	default:
		return nil, err // The server answered with some other error; do not hide it behind a fallback
	}
	return newDBBackend(c)
}

// mapAPIError turns an admin API error into the message the CLI printed before it talked to the
// server, so scripts and tests see the same text no matter how the command reached the database.
// The wording lives only here.
func mapAPIError(err error, username, tier, token string) error {
	var apiErr *client.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Code {
	case client.ErrCodeUserNotFound:
		return fmt.Errorf("user %s does not exist", username)
	case client.ErrCodeUserExists:
		return fmt.Errorf("user %s already exists", username)
	case client.ErrCodeTierNotFound:
		return fmt.Errorf("tier %s does not exist", tier)
	case client.ErrCodeTierExists:
		return fmt.Errorf("tier %s already exists", tier)
	case client.ErrCodeTierInUse:
		return fmt.Errorf("tier %s is still assigned to users", tier)
	case client.ErrCodeTierReservationsLimit:
		return fmt.Errorf("user %s has more topic reservations than tier %s allows, remove reservations first", username, tier)
	case client.ErrCodeTokenNotFound:
		return fmt.Errorf("token %s for user %s does not exist", token, username)
	case client.ErrCodeAccessAdminUser:
		return fmt.Errorf("user %s is an admin user, access control entries have no effect", username)
	case client.ErrCodeProvisionedUserChange:
		return fmt.Errorf("user %s is provisioned in the config file and cannot be changed", username)
	case client.ErrCodeProvisionedTokenChange:
		return fmt.Errorf("token %s is provisioned in the config file and cannot be changed", token)
	case client.ErrCodeUserNoPrimaryEmail:
		return fmt.Errorf("user %s has no primary email; cannot send reset link (omit --send-email to just print it)", username)
	case client.ErrCodeBaseURLNotConfigured:
		return errors.New("base-url must be configured to generate a reset link")
	case client.ErrCodeRoleInvalid:
		return errors.New("role must be either 'user' or 'admin'")
	case client.ErrCodeTierCodeInvalid:
		return errors.New("tier code must consist only of numbers and letters, and the tier needs a name")
	case client.ErrCodePermissionInvalid:
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	case client.ErrCodePasswordHashInvalid:
		return errors.New("password hash must be a bcrypt hash with cost 10 or higher, use 'ntfy user hash' to generate")
	}
	return apiErr
}

// isAPIErrorCode reports whether err is an admin API error with the given ntfy error code
func isAPIErrorCode(err error, code int) bool {
	var apiErr *client.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// adminUnreachableCause is the start of the fallback warning when no server answered on the socket
func adminUnreachableCause(socket string, err error) string {
	return fmt.Sprintf("No ntfy server answered on the admin socket\n%s (%s)", socket, adminUnreachableReason(err))
}

// adminUnreachableReason is the system error behind a wrapped client.ErrUnreachable, e.g.
// "no such file or directory", for the fallback warning
func adminUnreachableReason(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	return err.Error()
}

// printAdminFallbackWarning tells the user, loudly, that the change bypasses the running server, and
// that this fallback is deprecated (see docs/deprecations.md)
func printAdminFallbackWarning(w io.Writer, cause string) {
	fmt.Fprintln(w, adminFallbackWarningBanner)
	fmt.Fprintf(w, adminFallbackWarningText+"\n", cause)
	fmt.Fprintln(w, adminFallbackWarningBanner)
}
