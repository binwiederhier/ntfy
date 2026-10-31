//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"golang.org/x/term"
	"heckel.io/ntfy/v2/server"
)

const (
	adminFallbackWarningBanner = "**********************************************************************"
	adminFallbackWarningText   = `DEPRECATION WARNING: %s, writing to the database
directly. A running server may not see this change right away.

Direct database access will be removed after 2026-12-30. After that, the
ntfy user/access/token/tier commands need a running server.
See https://docs.ntfy.sh/deprecations/ for details.`
	adminSocketDisabledCause = "The admin socket is disabled\n(listen-admin-unix is \"-\" or empty)"
	adminWarningColorYellow  = "\033[1;33m"
	adminWarningColorReset   = "\033[0m"
)

var (
	// flagsBackend are the flags of the commands that use newBackend
	flagsBackend = []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-admin-unix", Aliases: []string{"listen_admin_unix"}, EnvVars: []string{"NTFY_LISTEN_ADMIN_UNIX"}, Value: server.DefaultListenAdminUnix(), DefaultText: "/run/ntfy/admin.sock on Linux, /var/run/ntfy/admin.sock on macOS/BSD", Usage: "admin socket of the local ntfy server; \"-\" disables it (then the database is used directly, which is deprecated)"}),
		&cli.StringFlag{Name: "server", Aliases: []string{"S"}, EnvVars: []string{"NTFY_ADMIN_SERVER"}, Usage: "manage a remote server over HTTPS instead (base URL, e.g. https://ntfy.example.com); requires --token of an admin user"},
		&cli.StringFlag{Name: "token", Aliases: []string{"k"}, EnvVars: []string{"NTFY_ADMIN_TOKEN"}, Usage: "admin access token for --server (better: NTFY_ADMIN_TOKEN, other users can see command line arguments)"},
	}

	// adminTargetFlags are the flags that choose which server a command talks to, see newBackend
	adminTargetFlags = []string{"--server", "-S", "--token", "-k", "--listen-admin-unix", "--listen_admin_unix", "--config", "-c"}
)

// backend is what the user, access, token and tier commands need. serverClient implements it
// over the admin socket or HTTP; dbBackend implements it against the database (the fallback).
type backend interface {
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

// newBackend picks the transport: an explicit --server (never falls back), else the admin
// socket when a server answers on it, else the database directly, with a warning. A socket file
// that nobody answers on is treated as absent (stale file after a crash).
func newBackend(c *cli.Context) (backend, error) {
	for _, arg := range c.Args().Slice() {
		if slices.Contains(adminTargetFlags, strings.SplitN(arg, "=", 2)[0]) {
			// The CLI stops parsing flags at the first argument, so this one would be silently ignored,
			// and the command would go to the local server instead of the one the user meant
			return nil, fmt.Errorf("%s must come before the command and its arguments, e.g. 'ntfy user --server=URL --token=TOKEN list'", arg)
		}
	}
	if server := c.String("server"); server != "" {
		token := c.String("token")
		if token == "" {
			return nil, errors.New("--token is required with --server")
		}
		if u, err := url.Parse(server); err == nil && u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
			fmt.Fprintf(c.App.ErrWriter, "warning: --server %s is not HTTPS, the token is sent unencrypted\n", server)
		}
		return newServerClient(server, token), nil
	}
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
	sc := newServerSocketClient(socket)
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
		direct, directErr := newDBBackend(c)
		if directErr != nil {
			return nil, fmt.Errorf("permission denied on the admin socket %s, run this command as root or as a member of the server's group (usually ntfy)", socket)
		}
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
		return direct, nil
	case errors.Is(err, errServerUnreachable):
		printAdminFallbackWarning(c.App.ErrWriter, adminUnreachableCause(socket, err))
	default:
		return nil, err // The server answered with some other error; do not hide it behind a fallback
	}
	return newDBBackend(c)
}

// withBackendFlags returns a copy of flags with flagsBackend, for the commands that use newBackend
func withBackendFlags(flags []cli.Flag) []cli.Flag {
	return append(append([]cli.Flag{}, flags...), flagsBackend...)
}

// isLoopbackHost reports whether host is this machine, so plain HTTP never leaves it
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// adminError turns an admin API error into the message the CLI printed before it talked to the
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
	case apiErrCodeAdminSocketOnly:
		return errors.New("only possible over the admin socket or directly in the database, not over --server: setting roles, changing admin users or their tokens, listing all users' tokens, and resetting the whole access control list")
	case apiErrCodeNotFound:
		return errors.New("the server does not support this command, upgrade the server or use direct database access")
	case apiErrCodeUnauthorized:
		return errors.New("unauthorized: --token must belong to an admin user")
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

// adminUnreachableReason extracts the cause from a wrapped errServerUnreachable, e.g.
// "no such file or directory", for the fallback warning
func adminUnreachableReason(err error) string {
	reason := strings.TrimPrefix(err.Error(), errServerUnreachable.Error()+": ")
	if i := strings.LastIndex(reason, ": "); i >= 0 {
		reason = reason[i+2:]
	}
	return reason
}

// printAdminFallbackWarning tells the user, loudly, that the change bypasses the running server, and
// that this fallback is deprecated (see docs/deprecations.md).
// It is yellow on a terminal, and plain text in logs and pipes, or if NO_COLOR is set.
func printAdminFallbackWarning(w io.Writer, cause string) {
	color := false
	if f, ok := w.(*os.File); ok && os.Getenv("NO_COLOR") == "" {
		color = term.IsTerminal(int(f.Fd()))
	}
	fmt.Fprint(w, adminFallbackWarning(cause, color))
}

func adminFallbackWarning(cause string, color bool) string {
	warning := adminFallbackWarningBanner + "\n" + fmt.Sprintf(adminFallbackWarningText, cause) + "\n" + adminFallbackWarningBanner
	if color {
		warning = adminWarningColorYellow + warning + adminWarningColorReset
	}
	return warning + "\n"
}
