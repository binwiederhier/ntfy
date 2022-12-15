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
	"heckel.io/ntfy/auth"
	"heckel.io/ntfy/util"
)

func init() {
	commands = append(commands, cmdUser)
}

var flagsUser = append(
	flagsDefault,
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: defaultServerConfigFile, DefaultText: defaultServerConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file", "H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file used for access control"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-default-access", Aliases: []string{"auth_default_access", "p"}, EnvVars: []string{"NTFY_AUTH_DEFAULT_ACCESS"}, Value: "read-write", Usage: "default permissions if no matching entries in the auth database are found"}),
)

var cmdUser = &cli.Command{
	Name:      "user",
	Usage:     "Manage/show users",
	UsageText: "ntfy user [list|add|remove|change-pass|change-role] ...",
	Flags:     flagsUser,
	Before:    initConfigFileInputSourceFunc("config", flagsUser, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "add",
			Aliases:   []string{"a"},
			Usage:     "Adds a new user",
			UsageText: "ntfy user add [--role=admin|user] USERNAME\nNTFY_PASSWORD=... ntfy user add [--role=admin|user] USERNAME",
			Action:    execUserAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "role", Aliases: []string{"r"}, Value: string(auth.RoleUser), Usage: "user role"},
			},
			Description: `Add a new user to the ntfy user database.

A user can be either a regular user, or an admin. A regular user has no read or write access (unless
granted otherwise by the auth-default-access setting). An admin user has read and write access to all
topics.

Examples:
  ntfy user add phil                     # Add regular user phil  
  ntfy user add --role=admin phil        # Add admin user phil
  NTFY_PASSWORD=... ntfy user add phil   # Add user, using env variable to set password (for scripts)

You may set the NTFY_PASSWORD environment variable to pass the password. This is useful if 
you are creating users via scripts.
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
			UsageText: "ntfy user change-pass USERNAME\nNTFY_PASSWORD=... ntfy user change-pass USERNAME",
			Action:    execUserChangePass,
			Description: `Change the password for the given user.

The new password will be read from STDIN, and it'll be confirmed by typing
it twice. 

Example:
  ntfy user change-pass phil
  NTFY_PASSWORD=.. ntfy user change-pass phil

You may set the NTFY_PASSWORD environment variable to pass the new password. This is 
useful if you are updating users via scripts.

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
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "Shows a list of users",
			Action:  execUserList,
			Description: `Shows a list of all configured users, including the everyone ('*') user.

This is a server-only command. It directly reads from the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined. 

This command is an alias to calling 'ntfy access' (display access control list).
`,
		},
	},
	Description: `Manage users of the ntfy server.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined. Please also refer
to the related command 'ntfy access'.

The command allows you to add/remove/change users in the ntfy user database, as well as change 
passwords or roles.

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
	role := auth.Role(c.String("role"))
	password := os.Getenv("NTFY_PASSWORD")
	if username == "" {
		return errors.New("username expected, type 'ntfy user add --help' for help")
	} else if username == userEveryone {
		return errors.New("username not allowed")
	} else if !auth.AllowedRole(role) {
		return errors.New("role must be either 'user' or 'admin'")
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if user, _ := manager.User(username); user != nil {
		return fmt.Errorf("user %s already exists", username)
	}
	if password == "" {
		p, err := readPasswordAndConfirm(c)
		if err != nil {
			return err
		}

		password = p
	}
	if err := manager.AddUser(username, password, role); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "user %s added with role %s\n", username, role)
	return nil
}

func execUserDel(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == "" {
		return errors.New("username expected, type 'ntfy user del --help' for help")
	} else if username == userEveryone {
		return errors.New("username not allowed")
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); err == auth.ErrNotFound {
		return fmt.Errorf("user %s does not exist", username)
	}
	if err := manager.RemoveUser(username); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "user %s removed\n", username)
	return nil
}

func execUserChangePass(c *cli.Context) error {
	username := c.Args().Get(0)
	password := os.Getenv("NTFY_PASSWORD")
	if username == "" {
		return errors.New("username expected, type 'ntfy user change-pass --help' for help")
	} else if username == userEveryone {
		return errors.New("username not allowed")
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); err == auth.ErrNotFound {
		return fmt.Errorf("user %s does not exist", username)
	}
	if password == "" {
		password, err = readPasswordAndConfirm(c)
		if err != nil {
			return err
		}
	}
	if err := manager.ChangePassword(username, password); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "changed password for user %s\n", username)
	return nil
}

func execUserChangeRole(c *cli.Context) error {
	username := c.Args().Get(0)
	role := auth.Role(c.Args().Get(1))
	if username == "" || !auth.AllowedRole(role) {
		return errors.New("username and new role expected, type 'ntfy user change-role --help' for help")
	} else if username == userEveryone {
		return errors.New("username not allowed")
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.User(username); err == auth.ErrNotFound {
		return fmt.Errorf("user %s does not exist", username)
	}
	if err := manager.ChangeRole(username, role); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "changed role for user %s to %s\n", username, role)
	return nil
}

func execUserList(c *cli.Context) error {
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	users, err := manager.Users()
	if err != nil {
		return err
	}
	return showUsers(c, manager, users)
}

func createAuthManager(c *cli.Context) (auth.Manager, error) {
	authFile := c.String("auth-file")
	authDefaultAccess := c.String("auth-default-access")
	if authFile == "" {
		return nil, errors.New("option auth-file not set; auth is unconfigured for this server")
	} else if !util.FileExists(authFile) {
		return nil, errors.New("auth-file does not exist; please start the server at least once to create it")
	} else if !util.Contains([]string{"read-write", "read-only", "write-only", "deny-all"}, authDefaultAccess) {
		return nil, errors.New("if set, auth-default-access must start set to 'read-write', 'read-only' or 'deny-all'")
	}
	authDefaultRead := authDefaultAccess == "read-write" || authDefaultAccess == "read-only"
	authDefaultWrite := authDefaultAccess == "read-write" || authDefaultAccess == "write-only"
	return auth.NewSQLiteAuthManager(authFile, authDefaultRead, authDefaultWrite)
}

func readPasswordAndConfirm(c *cli.Context) (string, error) {
	fmt.Fprint(c.App.ErrWriter, "password: ")
	password, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
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
