//go:build !noserver

package cmd

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

const (
	tierReset = "-"
)

func init() {
	commands = append(commands, cmdUser)
}

var flagsUser = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: server.DefaultConfigFile, DefaultText: server.DefaultConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file", "H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file used for access control"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-default-access", Aliases: []string{"auth_default_access", "p"}, EnvVars: []string{"NTFY_AUTH_DEFAULT_ACCESS"}, Value: "read-write", Usage: "default permissions if no matching entries in the auth database are found"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "database-url", Aliases: []string{"database_url"}, EnvVars: []string{"NTFY_DATABASE_URL"}, Usage: "PostgreSQL connection string for database-backed stores"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "base-url", Aliases: []string{"base_url", "B"}, EnvVars: []string{"NTFY_BASE_URL"}, Usage: "externally visible base URL for this host (e.g. https://ntfy.sh)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-addr", Aliases: []string{"smtp_sender_addr"}, EnvVars: []string{"NTFY_SMTP_SENDER_ADDR"}, Usage: "SMTP server address (host:port) for outgoing emails"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-user", Aliases: []string{"smtp_sender_user"}, EnvVars: []string{"NTFY_SMTP_SENDER_USER"}, Usage: "SMTP user (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-pass", Aliases: []string{"smtp_sender_pass"}, EnvVars: []string{"NTFY_SMTP_SENDER_PASS"}, Usage: "SMTP password (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-from", Aliases: []string{"smtp_sender_from"}, EnvVars: []string{"NTFY_SMTP_SENDER_FROM"}, Usage: "SMTP sender address (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "admin-socket", Aliases: []string{"admin_socket"}, EnvVars: []string{"NTFY_ADMIN_SOCKET"}, Value: server.DefaultAdminSocket(), DefaultText: "/run/ntfy/admin.sock", Usage: "admin socket of the local ntfy server; empty forces direct database access"}),
	&cli.StringFlag{Name: "server", Aliases: []string{"S"}, EnvVars: []string{"NTFY_ADMIN_SERVER"}, Usage: "manage a remote server over HTTPS instead (base URL, e.g. https://ntfy.example.com); requires --token of an admin user"},
	&cli.StringFlag{Name: "token", Aliases: []string{"k"}, EnvVars: []string{"NTFY_ADMIN_TOKEN"}, Usage: "admin access token for --server"},
)

var cmdUser = &cli.Command{
	Name:      "user",
	Usage:     "Manage/show users",
	UsageText: "ntfy user [list|add|remove|change-pass|reset-pass|change-role] ...",
	Flags:     flagsUser,
	Before:    initConfigFileInputSourceFunc("config", flagsUser, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "add",
			Aliases:   []string{"a"},
			Usage:     "Adds a new user",
			UsageText: "ntfy user add [--role=admin|user] USERNAME\nNTFY_PASSWORD=... ntfy user add [--role=admin|user] USERNAME\nNTFY_PASSWORD_HASH=... ntfy user add [--role=admin|user] USERNAME",
			Action:    execUserAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "role", Aliases: []string{"r"}, Value: string(user.RoleUser), Usage: "user role"},
				&cli.BoolFlag{Name: "ignore-exists", Usage: "if the user already exists, perform no action and exit"},
			},
			Description: `Add a new user to the ntfy user database.

A user can be either a regular user, or an admin. A regular user has no read or write access (unless
granted otherwise by the auth-default-access setting). An admin user has read and write access to all
topics.

Examples:
  ntfy user add phil                          # Add regular user phil
  ntfy user add --role=admin phil             # Add admin user phil
  NTFY_PASSWORD=... ntfy user add phil        # Add user, using env variable to set password (for scripts)
  NTFY_PASSWORD_HASH=... ntfy user add phil   # Add user, using env variable to set password hash (for scripts)

You may set the NTFY_PASSWORD environment variable to pass the password, or NTFY_PASSWORD_HASH to pass
directly the bcrypt hash. This is useful if you are creating users via scripts.
`,
		},
		{
			Name:      "remove",
			Aliases:   []string{"del", "rm"},
			Usage:     "Removes a user",
			UsageText: "ntfy user remove USERNAME",
			Action:    execUserDel,
			Description: `Remove a user from the ntfy user database.

Example:
  ntfy user del phil
`,
		},
		{
			Name:      "change-pass",
			Aliases:   []string{"chp"},
			Usage:     "Changes a user's password",
			UsageText: "ntfy user change-pass USERNAME\nNTFY_PASSWORD=... ntfy user change-pass USERNAME\nNTFY_PASSWORD_HASH=... ntfy user change-pass USERNAME",
			Action:    execUserChangePass,
			Description: `Change the password for the given user.

The new password will be read from STDIN, and it'll be confirmed by typing
it twice. 

Example:
  ntfy user change-pass phil
  NTFY_PASSWORD=.. ntfy user change-pass phil
  NTFY_PASSWORD_HASH=.. ntfy user change-pass phil

You may set the NTFY_PASSWORD environment variable to pass the new password or NTFY_PASSWORD_HASH to pass
directly the bcrypt hash. This is useful if you are updating users via scripts.
`,
		},
		{
			Name:      "reset-pass",
			Aliases:   []string{"rp"},
			Usage:     "Generates a password reset link for a user",
			UsageText: "ntfy user reset-pass [--send-email] USERNAME",
			Action:    execUserResetPass,
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "send-email", Aliases: []string{"e"}, Usage: "also email the reset link to the user's primary email"},
			},
			Description: `Generate a password reset link for the given user and print it to stdout.

The user completes the reset by opening the link in a browser and choosing a new password;
the admin never learns or chooses the new password. The link is single-use and expires after
one hour. This is an admin override of the self-service reset flow -- unlike self-service, it
does not require the user to have a verified primary email (the token is bound to the user).

With --send-email, the link is additionally emailed to the user's primary email address (this
requires SMTP to be configured and the user to have a verified primary email).

Example:
  ntfy user reset-pass phil               # Print a reset link for user phil
  ntfy user reset-pass --send-email phil  # Print and email the reset link
`,
		},
		{
			Name:      "change-role",
			Aliases:   []string{"chr"},
			Usage:     "Changes the role of a user",
			UsageText: "ntfy user change-role USERNAME ROLE",
			Action:    execUserChangeRole,
			Description: `Change the role for the given user to admin or user.

This command can be used to change the role of a user either from a regular user
to an admin user, or the other way around:

- admin: an admin has read/write access to all topics
- user: a regular user only has access to what was explicitly granted via 'ntfy access'

When changing the role of a user to "admin", all access control entries for that 
user are removed, since they are no longer necessary.

Example:
  ntfy user change-role phil admin   # Make user phil an admin 
  ntfy user change-role phil user    # Remove admin role from user phil 
`,
		},
		{
			Name:      "change-tier",
			Aliases:   []string{"cht"},
			Usage:     "Changes the tier of a user",
			UsageText: "ntfy user change-tier USERNAME (TIER|-)",
			Action:    execUserChangeTier,
			Description: `Change the tier for the given user.

This command can be used to change the tier of a user. Tiers define usage limits, such
as messages per day, attachment file sizes, etc.

Example:
  ntfy user change-tier phil pro   # Change tier to "pro" for user "phil"  
  ntfy user change-tier phil -     # Remove tier from user "phil" entirely 
`,
		},
		{
			Name:      "hash",
			Usage:     "Create password hash for a predefined user",
			UsageText: "ntfy user hash",
			Action:    execUserHash,
			Description: `Asks for a password and creates a bcrypt password hash.

This command is useful to create a password hash for a user, which can then be used
for predefined users in the server config file, in auth-users.

Example:
  $ ntfy user hash
  (asks for password and confirmation)
  $2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C
`,
		},
		{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "Shows a list of users",
			Action:  execUserList,
			Description: `Shows a list of all configured users, including the everyone ('*') user.

This command is an alias to calling 'ntfy access' (display access control list).

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead.
`,
		},
	},
	Description: `Manage users of the ntfy server.

The command allows you to add/remove/change users in the ntfy user database, as well as change 
passwords or roles.

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead. Please also refer
to the related command 'ntfy access'.

Examples:
  ntfy user list                               # Shows list of users (alias: 'ntfy access')                      
  ntfy user add phil                           # Add regular user phil  
  NTFY_PASSWORD=... ntfy user add phil         # As above, using env variable to set password (for scripts)
  ntfy user add --role=admin phil              # Add admin user phil
  ntfy user del phil                           # Delete user phil
  ntfy user change-pass phil                   # Change password for user phil
  NTFY_PASSWORD=.. ntfy user change-pass phil  # As above, using env variable to set password (for scripts)
  ntfy user change-role phil admin             # Make user phil an admin 

For the 'ntfy user add' and 'ntfy user change-pass' commands, you may set the NTFY_PASSWORD environment
variable to pass the new password. This is useful if you are creating/updating users via scripts.
`,
}

func execUserAdd(c *cli.Context) error {
	username := c.Args().Get(0)
	role := user.Role(c.String("role"))
	if username == "" {
		return errors.New("username expected, type 'ntfy user add --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	} else if !user.AllowedRole(role) {
		return errors.New("role must be either 'user' or 'admin'")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if _, err := api.User(username); err == nil {
		if c.Bool("ignore-exists") {
			fmt.Fprintf(c.App.Writer, "user %s already exists (exited successfully)\n", username)
			return nil
		}
		return fmt.Errorf("user %s already exists", username)
	} else if !isAdminErrorCode(err, client.AdminErrCodeUserNotFound) {
		return adminError(err, username, "", "")
	}
	hash, err := readPasswordHash(c)
	if err != nil {
		return err
	}
	roleParam := ""
	if role != user.RoleUser {
		roleParam = string(role) // Only send the role if needed: over --server, the API refuses it
	}
	if err := api.AddUser(username, hash, "", roleParam); err != nil {
		return adminError(err, username, "", "")
	}
	fmt.Fprintf(c.App.Writer, "user %s added with role %s\n", username, role)
	return nil
}

func execUserDel(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == "" {
		return errors.New("username expected, type 'ntfy user del --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if err := api.RemoveUser(username); err != nil {
		return adminError(err, username, "", "")
	}
	fmt.Fprintf(c.App.Writer, "user %s removed\n", username)
	return nil
}

func execUserChangePass(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == "" {
		return errors.New("username expected, type 'ntfy user change-pass --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if _, err := api.User(username); err != nil {
		return adminError(err, username, "", "")
	}
	hash, err := readPasswordHash(c)
	if err != nil {
		return err
	}
	if err := api.ChangePassword(username, hash); err != nil {
		return adminError(err, username, "", "")
	}
	fmt.Fprintf(c.App.Writer, "changed password for user %s\n", username)
	return nil
}

func execUserResetPass(c *cli.Context) error {
	username := c.Args().Get(0)
	sendEmail := c.Bool("send-email")
	if username == "" {
		return errors.New("username expected, type 'ntfy user reset-pass --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	reset, err := api.ResetPassword(username, sendEmail)
	if err != nil {
		return adminError(err, username, "", "")
	}
	fmt.Fprintln(c.App.Writer, reset.Link)
	if reset.Email != "" {
		fmt.Fprintf(c.App.ErrWriter, "reset link emailed to %s\n", reset.Email)
	}
	return nil
}

func execUserChangeRole(c *cli.Context) error {
	username := c.Args().Get(0)
	role := user.Role(c.Args().Get(1))
	if username == "" || !user.AllowedRole(role) {
		return errors.New("username and new role expected, type 'ntfy user change-role --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if err := api.ChangeRole(username, string(role)); err != nil {
		return adminError(err, username, "", "")
	}
	fmt.Fprintf(c.App.Writer, "changed role for user %s to %s\n", username, role)
	return nil
}

func execUserHash(c *cli.Context) error {
	password, err := readPasswordAndConfirm(c)
	if err != nil {
		return err
	}
	hash, err := user.HashPassword(password, user.DefaultUserPasswordBcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	fmt.Fprintln(c.App.Writer, hash)
	return nil
}

func execUserChangeTier(c *cli.Context) error {
	username := c.Args().Get(0)
	tier := c.Args().Get(1)
	if username == "" {
		return errors.New("username and new tier expected, type 'ntfy user change-tier --help' for help")
	} else if !user.AllowedTier(tier) && tier != tierReset {
		return errors.New("invalid tier, must be tier code, or - to reset")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if tier == tierReset {
		if err := api.ChangeTier(username, client.AdminTierReset); err != nil {
			return adminError(err, username, tier, "")
		}
		fmt.Fprintf(c.App.Writer, "removed tier from user %s\n", username)
	} else {
		if err := api.ChangeTier(username, tier); err != nil {
			return adminError(err, username, tier, "")
		}
		fmt.Fprintf(c.App.Writer, "changed tier for user %s to %s\n", username, tier)
	}
	return nil
}

func execUserList(c *cli.Context) error {
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	users, err := api.Users()
	if err != nil {
		return adminError(err, "", "", "")
	}
	return showUsers(c, users)
}

func createUserManager(c *cli.Context) (*user.Manager, error) {
	authFile := c.String("auth-file")
	authStartupQueries := c.String("auth-startup-queries")
	authDefaultAccess := c.String("auth-default-access")
	databaseURL := c.String("database-url")
	authDefault, err := user.ParsePermission(authDefaultAccess)
	if err != nil {
		return nil, errors.New("if set, auth-default-access must start set to 'read-write', 'read-only', 'write-only' or 'deny-all'")
	}
	authConfig := &user.Config{
		DefaultAccess:       authDefault,
		ProvisionEnabled:    false, // Hack: Do not re-provision users on manager initialization
		BcryptCost:          user.DefaultUserPasswordBcryptCost,
		QueueWriterInterval: user.DefaultUserStatsQueueWriterInterval,
		AccessCacheEnabled:  false, // Do not cache for CLI commands
	}
	if databaseURL != "" {
		host, dbErr := pg.Open(databaseURL)
		if dbErr != nil {
			return nil, dbErr
		}
		return user.NewPostgresManager(db.New(host, nil), authConfig)
	} else if authFile != "" {
		if !util.FileExists(authFile) {
			return nil, errors.New("auth-file does not exist; please start the server at least once to create it")
		}
		return user.NewSQLiteManager(authFile, authStartupQueries, authConfig)
	}
	return nil, errors.New("option database-url or auth-file not set; auth is unconfigured for this server")
}

func readPasswordAndConfirm(c *cli.Context) (string, error) {
	fmt.Fprint(c.App.ErrWriter, "password: ")
	password, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
	} else if len(password) == 0 {
		return "", errors.New("password cannot be empty")
	}
	fmt.Fprintf(c.App.ErrWriter, "\r%s\rconfirm: ", strings.Repeat(" ", 25))
	confirm, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(c.App.ErrWriter, "\r%s\r", strings.Repeat(" ", 25))
	if subtle.ConstantTimeCompare(confirm, password) != 1 {
		return "", errors.New("passwords do not match: try it again, but this time type slooowwwlly")
	}
	return string(password), nil
}

// readPasswordHash returns the bcrypt hash of the new password: NTFY_PASSWORD_HASH as is, or
// NTFY_PASSWORD or the prompted password, hashed here. The server never sees the plain password.
func readPasswordHash(c *cli.Context) (string, error) {
	if hash, ok := os.LookupEnv("NTFY_PASSWORD_HASH"); ok {
		return hash, nil
	}
	password := os.Getenv("NTFY_PASSWORD")
	if password == "" {
		var err error
		if password, err = readPasswordAndConfirm(c); err != nil {
			return "", err
		}
	}
	return user.HashPassword(password, user.DefaultUserPasswordBcryptCost)
}

func isAdminErrorCode(err error, code int) bool {
	var apiErr *client.AdminError
	return errors.As(err, &apiErr) && apiErr.Code == code
}
