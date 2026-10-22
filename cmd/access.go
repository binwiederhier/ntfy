//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func init() {
	commands = append(commands, cmdAccess)
}

const (
	userEveryone = "everyone"
)

var flagsAccess = append(
	append([]cli.Flag{}, flagsUser...),
	&cli.BoolFlag{Name: "reset", Aliases: []string{"r"}, Usage: "reset access for user (and topic)"},
)

var cmdAccess = &cli.Command{
	Name:      "access",
	Usage:     "Grant/revoke access to a topic, or show access",
	UsageText: "ntfy access [USERNAME [TOPIC [PERMISSION]]]",
	Flags:     flagsAccess,
	Before:    initConfigFileInputSourceFunc("config", flagsAccess, initLogFunc),
	Action:    execUserAccess,
	Category:  categoryServer,
	Description: `Manage the access control list for the ntfy server.

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead. Please also refer
to the related command 'ntfy user'.

The command allows you to show the access control list, as well as change it, depending on how
it is called.

Usage:
  ntfy access                            # Shows access control list (alias: 'ntfy user list')
  ntfy access USERNAME                   # Shows access control entries for USERNAME
  ntfy access USERNAME TOPIC PERMISSION  # Allow/deny access for USERNAME to TOPIC

Arguments:
  USERNAME     an existing user, as created with 'ntfy user add', or "everyone"/"*"
               to define access rules for anonymous/unauthenticated clients
  TOPIC        name of a topic with optional wildcards, e.g. "mytopic*"
  PERMISSION   one of the following:
               - read-write (alias: rw) 
               - read-only (aliases: read, ro)
               - write-only (aliases: write, wo)
               - deny (alias: none)

Examples:
  ntfy access                        # Shows access control list (alias: 'ntfy user list')
  ntfy access phil                   # Shows access for user phil
  ntfy access phil mytopic rw        # Allow read-write access to mytopic for user phil
  ntfy access everyone mytopic rw    # Allow anonymous read-write access to mytopic
  ntfy access everyone "up*" write   # Allow anonymous write-only access to topics "up..." 
  ntfy access --reset                # Reset entire access control list
  ntfy access --reset phil           # Reset all access for user phil
  ntfy access --reset phil mytopic   # Reset access for user phil and topic mytopic
`,
}

func execUserAccess(c *cli.Context) error {
	if c.NArg() > 3 {
		return errors.New("too many arguments, please check 'ntfy access --help' for usage details")
	}
	username := c.Args().Get(0)
	if username == userEveryone {
		username = user.Everyone
	}
	topic := c.Args().Get(1)
	perms := c.Args().Get(2)
	reset := c.Bool("reset")
	if reset && perms != "" {
		return errors.New("too many arguments, please check 'ntfy access --help' for usage details")
	} else if !reset && perms == "" && topic != "" {
		return errors.New("invalid syntax, please check 'ntfy access --help' for usage details")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if reset {
		return resetAccess(c, api, username, topic)
	} else if perms == "" {
		return showAccess(c, api, username)
	}
	return changeAccess(c, api, username, topic, perms)
}

func changeAccess(c *cli.Context, api adminAPI, username string, topic string, perms string) error {
	if !util.Contains([]string{"", "read-write", "rw", "read-only", "read", "ro", "write-only", "write", "wo", "none", "deny"}, perms) {
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	}
	permission, err := user.ParsePermission(perms)
	if err != nil {
		return err
	}
	if err := api.AllowAccess(username, topic, permission.String()); err != nil {
		return adminError(err, username, "", "")
	}
	if permission.IsReadWrite() {
		fmt.Fprintf(c.App.Writer, "granted read-write access to topic %s\n\n", topic)
	} else if permission.IsRead() {
		fmt.Fprintf(c.App.Writer, "granted read-only access to topic %s\n\n", topic)
	} else if permission.IsWrite() {
		fmt.Fprintf(c.App.Writer, "granted write-only access to topic %s\n\n", topic)
	} else {
		fmt.Fprintf(c.App.Writer, "revoked all access to topic %s\n\n", topic)
	}
	return showUserAccess(c, api, username)
}

func resetAccess(c *cli.Context, api adminAPI, username, topic string) error {
	if err := api.ResetAccess(username, topic); err != nil {
		return adminError(err, username, "", "")
	}
	if username == "" {
		fmt.Fprintln(c.App.Writer, "reset access for all users")
		return nil
	} else if topic == "" {
		fmt.Fprintf(c.App.Writer, "reset access for user %s\n\n", username)
	} else {
		fmt.Fprintf(c.App.Writer, "reset access for user %s and topic %s\n\n", username, topic)
	}
	return showUserAccess(c, api, username)
}

func showAccess(c *cli.Context, api adminAPI, username string) error {
	if username == "" {
		return showAllAccess(c, api)
	}
	return showUserAccess(c, api, username)
}

func showAllAccess(c *cli.Context, api adminAPI) error {
	users, err := api.Users()
	if err != nil {
		return adminError(err, "", "", "")
	}
	return showUsers(c, users)
}

func showUserAccess(c *cli.Context, api adminAPI, username string) error {
	u, err := api.User(username)
	if err != nil {
		return adminError(err, username, "", "")
	}
	return showUsers(c, []*client.AdminUser{u})
}

func showUsers(c *cli.Context, users []*client.AdminUser) error {
	for _, u := range users {
		tier := "none"
		if u.TierName != "" {
			tier = u.TierName
		}
		provisioned := ""
		if u.Provisioned {
			provisioned = ", server config"
		}
		fmt.Fprintf(c.App.Writer, "user %s (role: %s, tier: %s%s)\n", u.Username, u.Role, tier, provisioned)
		if u.Role == string(user.RoleAdmin) {
			fmt.Fprintf(c.App.Writer, "- read-write access to all topics (admin role)\n")
		} else if len(u.Grants) > 0 {
			for _, grant := range u.Grants {
				permission, err := user.ParsePermission(grant.Permission)
				if err != nil {
					return err
				}
				grantProvisioned := ""
				if grant.Provisioned {
					grantProvisioned = " (server config)"
				}
				if permission.IsReadWrite() {
					fmt.Fprintf(c.App.Writer, "- read-write access to topic %s%s\n", grant.Topic, grantProvisioned)
				} else if permission.IsRead() {
					fmt.Fprintf(c.App.Writer, "- read-only access to topic %s%s\n", grant.Topic, grantProvisioned)
				} else if permission.IsWrite() {
					fmt.Fprintf(c.App.Writer, "- write-only access to topic %s%s\n", grant.Topic, grantProvisioned)
				} else {
					fmt.Fprintf(c.App.Writer, "- no access to topic %s%s\n", grant.Topic, grantProvisioned)
				}
			}
		} else {
			fmt.Fprintf(c.App.Writer, "- no topic-specific permissions\n")
		}
		if u.Username == user.Everyone {
			access, err := user.ParsePermission(u.DefaultAccess)
			if err != nil {
				return err
			}
			if access.IsReadWrite() {
				fmt.Fprintln(c.App.Writer, "- read-write access to all (other) topics (server config)")
			} else if access.IsRead() {
				fmt.Fprintln(c.App.Writer, "- read-only access to all (other) topics (server config)")
			} else if access.IsWrite() {
				fmt.Fprintln(c.App.Writer, "- write-only access to all (other) topics (server config)")
			} else {
				fmt.Fprintln(c.App.Writer, "- no access to any (other) topics (server config)")
			}
		}
	}
	return nil
}
