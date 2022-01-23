package cmd

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/auth"
	"heckel.io/ntfy/util"
	"strings"
)

/*

---
dabbling for CLI
	ntfy user allow phil mytopic
	ntfy user allow phil mytopic --read-only
	ntfy user deny phil mytopic
	ntfy user list
	   phil (admin)
	   - read-write access to everything
	   ben (user)
	   - read-write access to a topic alerts
	   - read access to
       everyone (no user)
       - read-only access to topic announcements

*/

var flagsUser = []cli.Flag{
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: "/etc/ntfy/server.yml", DefaultText: "/etc/ntfy/server.yml", Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file used for access control"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-default-permissions", Aliases: []string{"p"}, EnvVars: []string{"NTFY_AUTH_DEFAULT_PERMISSIONS"}, Value: "read-write", Usage: "default permissions if no matching entries in the auth database are found"}),
}

var cmdUser = &cli.Command{
	Name:      "user",
	Usage:     "Manage users and access to topics",
	UsageText: "ntfy user [add|del|...] ...",
	Flags:     flagsUser,
	Before:    initConfigFileInputSource("config", flagsUser),
	Subcommands: []*cli.Command{
		{
			Name:    "add",
			Aliases: []string{"a"},
			Usage:   "add user to auth database",
			Action:  execUserAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "role", Aliases: []string{"r"}, Value: string(auth.RoleUser), Usage: "user role"},
			},
		},
		{
			Name:    "remove",
			Aliases: []string{"del", "rm"},
			Usage:   "remove user from auth database",
			Action:  execUserDel,
		},
		{
			Name:    "change-pass",
			Aliases: []string{"ch"},
			Usage:   "change user password",
			Action:  execUserChangePass,
		},
	},
}

func execUserAdd(c *cli.Context) error {
	role := c.String("role")
	if c.NArg() == 0 {
		return errors.New("username expected, type 'ntfy user add --help' for help")
	} else if role != string(auth.RoleUser) && role != string(auth.RoleAdmin) {
		return errors.New("role must be either 'user' or 'admin'")
	}
	username := c.Args().Get(0)
	password, err := readPassword(c)
	if err != nil {
		return err
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if err := manager.AddUser(username, password, auth.Role(role)); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "User %s added with role %s\n", username, role)
	return nil
}

func execUserDel(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("username expected, type 'ntfy user del --help' for help")
	}
	username := c.Args().Get(0)
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if err := manager.RemoveUser(username); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "User %s removed\n", username)
	return nil
}

func execUserChangePass(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("username expected, type 'ntfy user change-pass --help' for help")
	}
	username := c.Args().Get(0)
	password, err := readPassword(c)
	if err != nil {
		return err
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if err := manager.ChangePassword(username, password); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "Changed password for user %s\n", username)
	return nil
}

func createAuthManager(c *cli.Context) (auth.Manager, error) {
	authFile := c.String("auth-file")
	authDefaultPermissions := c.String("auth-default-permissions")
	if authFile == "" {
		return nil, errors.New("option auth-file not set; auth is unconfigured for this server")
	} else if !util.FileExists(authFile) {
		return nil, errors.New("auth-file does not exist; please start the server at least once to create it")
	} else if !util.InStringList([]string{"read-write", "read-only", "deny-all"}, authDefaultPermissions) {
		return nil, errors.New("if set, auth-default-permissions must start set to 'read-write', 'read-only' or 'deny-all'")
	}
	authDefaultRead := authDefaultPermissions == "read-write" || authDefaultPermissions == "read-only"
	authDefaultWrite := authDefaultPermissions == "read-write"
	return auth.NewSQLiteAuth(authFile, authDefaultRead, authDefaultWrite)
}

func readPassword(c *cli.Context) (string, error) {
	fmt.Fprint(c.App.ErrWriter, "Enter Password: ")
	password, err := util.ReadPassword(c.App.Reader)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(c.App.ErrWriter, "\r%s\rConfirm: ", strings.Repeat(" ", 25))
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
