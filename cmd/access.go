package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/auth"
	"heckel.io/ntfy/util"
)

const (
	userEveryone = "everyone"
)

var flagsAccess = append(
	userCommandFlags(),
	&cli.BoolFlag{Name: "reset", Aliases: []string{"r"}, Usage: "reset access for user (and topic)"},
)

var cmdAccess = &cli.Command{
	Name:      "access",
	Usage:     "Grant/revoke access to a topic, or show access",
	UsageText: "ntfy access [USERNAME [TOPIC [PERMISSION]]]",
	Flags:     flagsAccess,
	Before:    initConfigFileInputSource("config", flagsAccess),
	Action:    execUserAccess,
	Category:  categoryServer,
	Description: `Manage the access control list for the ntfy server.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined. Please also refer
to the related command 'ntfy user'.

The command allows you to show the access control list, as well as change it, depending on how
it is called.

Usage:
  ntfy access                            # Shows the entire access control list
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
  ntfy access                        
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
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	username := c.Args().Get(0)
	if username == userEveryone {
		username = auth.Everyone
	}
	topic := c.Args().Get(1)
	perms := c.Args().Get(2)
	reset := c.Bool("reset")
	if reset {
		if perms != "" {
			return errors.New("too many arguments, please check 'ntfy access --help' for usage details")
		}
		return resetAccess(c, manager, username, topic)
	} else if perms == "" {
		return showAccess(c, manager, username)
	}
	return changeAccess(c, manager, username, topic, perms)
}

func changeAccess(c *cli.Context, manager auth.Manager, username string, topic string, perms string) error {
	if !util.InStringList([]string{"", "read-write", "rw", "read-only", "read", "ro", "write-only", "write", "wo", "none", "deny"}, perms) {
		return errors.New("permission must be one of: read-write, read-only, write-only, or deny (or the aliases: read, ro, write, wo, none)")
	}
	read := util.InStringList([]string{"read-write", "rw", "read-only", "read", "ro"}, perms)
	write := util.InStringList([]string{"read-write", "rw", "write-only", "write", "wo"}, perms)
	if err := manager.AllowAccess(username, topic, read, write); err != nil {
		return err
	}
	if read && write {
		fmt.Fprintf(c.App.Writer, "Granted read-write access to topic %s\n\n", topic)
	} else if read {
		fmt.Fprintf(c.App.Writer, "Granted read-only access to topic %s\n\n", topic)
	} else if write {
		fmt.Fprintf(c.App.Writer, "Granted write-only access to topic %s\n\n", topic)
	} else {
		fmt.Fprintf(c.App.Writer, "Revoked all access to topic %s\n\n", topic)
	}
	return showUserAccess(c, manager, username)
}

func resetAccess(c *cli.Context, manager auth.Manager, username, topic string) error {
	if username == "" {
		return resetAllAccess(c, manager)
	} else if topic == "" {
		return resetUserAccess(c, manager, username)
	}
	return resetUserTopicAccess(c, manager, username, topic)
}

func resetAllAccess(c *cli.Context, manager auth.Manager) error {
	if err := manager.ResetAccess("", ""); err != nil {
		return err
	}
	fmt.Fprintln(c.App.Writer, "Reset access for all users")
	return nil
}

func resetUserAccess(c *cli.Context, manager auth.Manager, username string) error {
	if err := manager.ResetAccess(username, ""); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "Reset access for user %s\n\n", username)
	return showUserAccess(c, manager, username)
}

func resetUserTopicAccess(c *cli.Context, manager auth.Manager, username string, topic string) error {
	if err := manager.ResetAccess(username, topic); err != nil {
		return err
	}
	fmt.Fprintf(c.App.Writer, "Reset access for user %s and topic %s\n\n", username, topic)
	return showUserAccess(c, manager, username)
}

func showAccess(c *cli.Context, manager auth.Manager, username string) error {
	if username == "" {
		return showAllAccess(c, manager)
	}
	return showUserAccess(c, manager, username)
}

func showAllAccess(c *cli.Context, manager auth.Manager) error {
	users, err := manager.Users()
	if err != nil {
		return err
	}
	return showUsers(c, manager, users)
}

func showUserAccess(c *cli.Context, manager auth.Manager, username string) error {
	users, err := manager.User(username)
	if err != nil {
		return err
	}
	return showUsers(c, manager, []*auth.User{users})
}

func showUsers(c *cli.Context, manager auth.Manager, users []*auth.User) error {
	for _, user := range users {
		fmt.Fprintf(c.App.Writer, "User %s (%s)\n", user.Name, user.Role)
		if user.Role == auth.RoleAdmin {
			fmt.Fprintf(c.App.ErrWriter, "- read-write access to all topics (admin role)\n")
		} else if len(user.Grants) > 0 {
			for _, grant := range user.Grants {
				if grant.AllowRead && grant.AllowWrite {
					fmt.Fprintf(c.App.ErrWriter, "- read-write access to topic %s\n", grant.TopicPattern)
				} else if grant.AllowRead {
					fmt.Fprintf(c.App.ErrWriter, "- read-only access to topic %s\n", grant.TopicPattern)
				} else if grant.AllowWrite {
					fmt.Fprintf(c.App.ErrWriter, "- write-only access to topic %s\n", grant.TopicPattern)
				} else {
					fmt.Fprintf(c.App.ErrWriter, "- no access to topic %s\n", grant.TopicPattern)
				}
			}
		} else {
			fmt.Fprintf(c.App.ErrWriter, "- no topic-specific permissions\n")
		}
		if user.Name == auth.Everyone {
			defaultRead, defaultWrite := manager.DefaultAccess()
			if defaultRead && defaultWrite {
				fmt.Fprintln(c.App.ErrWriter, "- read-write access to all (other) topics (server config)")
			} else if defaultRead {
				fmt.Fprintln(c.App.ErrWriter, "- read-only access to all (other) topics (server config)")
			} else if defaultWrite {
				fmt.Fprintln(c.App.ErrWriter, "- write-only access to all (other) topics (server config)")
			} else {
				fmt.Fprintln(c.App.ErrWriter, "- no access to any (other) topics (server config)")
			}
		}
	}
	return nil
}
