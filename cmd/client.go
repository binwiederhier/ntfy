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

// adminClient is what the user, access, token and tier commands need. socketClient implements it
// over the admin socket; databaseClient implements it against the database (the fallback).
type adminClient interface {
	Users() ([]*apiUser, error)
	User(username string) (*apiUser, error)
	AddUser(username, hash, tier, role string) error
	ChangePassword(username, hash string) error
	ChangeTier(username, tier string) error
	ChangeRole(username, role string) error
	RemoveUser(username string) error
	ResetPassword(username string, sendEmail bool) (*apiPasswordReset, error)
	AllowAccess(username, topic, permission string) error
	ResetAccess(username, topic string) error
	Tokens(username string) ([]*apiToken, error)
	AddToken(username, label string, expires time.Time) (*apiToken, error)
	RemoveToken(username, token string) error
	Tiers() ([]*apiTier, error)
	Tier(code string) (*apiTier, error)
	AddTier(t *apiTier) error
	UpdateTier(t *apiTier) error
	RemoveTier(code string) error
	Close() error
}

// newAdminClient picks the transport: the admin socket when a server answers on it, else the database
// directly, with a warning. A socket file that nobody answers on is treated as absent (stale file
// after a crash).
func newAdminClient(c *cli.Context) (adminClient, error) {
	authConfigured := c.String("auth-file") != "" || c.String("database-url") != ""
	socket := c.String("listen-admin-unix")
	if !authConfigured {
		return newDatabaseClient(c) // Fails with the message for the missing auth config
	} else if socket == "" || socket == "-" { // "-" disables it, as in the server
		printAdminFallbackWarning(c.App.ErrWriter, adminSocketDisabledCause)
		return newDatabaseClient(c)
	}
	if err := checkAdminSocketOwner(socket, c.String("auth-file")); err != nil {
		return nil, err
	}
	sc := newSocketClient(socket)
	err := sc.Ping()
	var apiErr *apiError
	switch {
	case err == nil:
		return sc, nil
	case errors.As(err, &apiErr) && apiErr.Code == apiErrCodeNotFound:
		// The server runs without auth, i.e. it does not use this database; createUserManager decides
	case errors.Is(err, fs.ErrPermission):
		// Either this user may not manage the server, or the socket is some other server's (e.g. the
		// system's, while managing one's own instance). Only the database can tell the two apart.
		direct, directErr := newDatabaseClient(c)
		if directErr != nil {
			return nil, fmt.Errorf("permission denied on the admin socket %s, run this command as root or as a member of the server's group (usually ntfy)", socket)
		}
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
		return direct, nil
	case errors.Is(err, errSocketUnreachable):
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
	default:
		return nil, err // The server answered with some other error; do not hide it behind a fallback
	}
	return newDatabaseClient(c)
}

// mapAPIError turns an admin API error into the message the CLI printed before it talked to the
// server, so scripts and tests see the same text no matter how the command reached the database.
// The wording lives only here.
func mapAPIError(err error, username, tier, token string) error {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Code {
	case apiErrCodeUserNotFound:
		return fmt.Errorf("user %s does not exist", username)
	case apiErrCodeUserExists:
		return fmt.Errorf("user %s already exists", username)
	case apiErrCodeTierNotFound:
		return fmt.Errorf("tier %s does not exist", tier)
	case apiErrCodeTierExists:
		return fmt.Errorf("tier %s already exists", tier)
	case apiErrCodeTierInUse:
		return fmt.Errorf("tier %s is still assigned to users", tier)
	case apiErrCodeTierReservationsLimit:
		return fmt.Errorf("user %s has more topic reservations than tier %s allows, remove reservations first", username, tier)
	case apiErrCodeTokenNotFound:
		return fmt.Errorf("token %s for user %s does not exist", token, username)
	case apiErrCodeAccessAdminUser:
		return fmt.Errorf("user %s is an admin user, access control entries have no effect", username)
	case apiErrCodeProvisionedUserChange:
		return fmt.Errorf("user %s is provisioned in the config file and cannot be changed", username)
	case apiErrCodeProvisionedTokenChange:
		return fmt.Errorf("token %s is provisioned in the config file and cannot be changed", token)
	case apiErrCodeUserNoPrimaryEmail:
		return fmt.Errorf("user %s has no primary email; cannot send reset link (omit --send-email to just print it)", username)
	case apiErrCodeBaseURLNotConfigured:
		return errors.New("base-url must be configured to generate a reset link")
	case apiErrCodeRoleInvalid:
		return errors.New("role must be either 'user' or 'admin'")
	case apiErrCodeTierCodeInvalid:
		return errors.New("tier code must consist only of numbers and letters, and the tier needs a name")
	case apiErrCodePermissionInvalid:
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	case apiErrCodePasswordHashInvalid:
		return errors.New("password hash must be a bcrypt hash with cost 10 or higher, use 'ntfy user hash' to generate")
	}
	return apiErr
}

// isAPIErrorCode reports whether err is an admin API error with the given ntfy error code
func isAPIErrorCode(err error, code int) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// adminUnreachableCause is the start of the fallback warning when no server answered on the socket
func adminUnreachableCause(socket string, err error) string {
	return fmt.Sprintf("No ntfy server answered on the admin socket\n%s (%s)", socket, adminUnreachableReason(err))
}

// adminUnreachableReason is the system error behind a wrapped errSocketUnreachable, e.g.
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
