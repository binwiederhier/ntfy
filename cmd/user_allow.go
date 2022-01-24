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

var flagsAllow = append(
	userCommandFlags(),
	&cli.BoolFlag{Name: "reset", Aliases: []string{"r"}, Usage: "reset access for user (and topic)"},
)

var cmdAllow = &cli.Command{
	Name:      "allow",
	Usage:     "Grant a user access to a topic",
	UsageText: "ntfy allow USERNAME TOPIC [read-write|read-only|write-only|none]",
	Flags:     flagsAllow,
	Before:    initConfigFileInputSource("config", flagsAllow),
	Action:    execUserAllow,
	Category:  categoryServer,
}

func execUserAllow(c *cli.Context) error {
	username := c.Args().Get(0)
	topic := c.Args().Get(1)
	perms := c.Args().Get(2)
	reset := c.Bool("reset")
	if username == "" {
		return errors.New("username expected, type 'ntfy allow --help' for help")
	} else if !reset && topic == "" {
		return errors.New("topic expected, type 'ntfy allow --help' for help")
	} else if !util.InStringList([]string{"", "read-write", "rw", "read-only", "read", "ro", "write-only", "write", "wo", "none"}, perms) {
		return errors.New("permission must be one of: read-write, read-only, write-only, or none (or the aliases: read, ro, write, wo)")
	}
	if username == userEveryone {
		username = ""
	}
	read := util.InStringList([]string{"", "read-write", "rw", "read-only", "read", "ro"}, perms)
	write := util.InStringList([]string{"", "read-write", "rw", "write-only", "write", "wo"}, perms)
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	if reset {
		return doAccessReset(c, manager, username, topic)
	}
	return doAccessAllow(c, manager, username, topic, read, write)
}

func doAccessAllow(c *cli.Context, manager auth.Manager, username string, topic string, read bool, write bool) error {
	if err := manager.AllowAccess(username, topic, read, write); err != nil {
		return err
	}
	if username == "" {
		if read && write {
			fmt.Fprintf(c.App.Writer, "Anonymous users granted full access to topic %s\n", topic)
		} else if read {
			fmt.Fprintf(c.App.Writer, "Anonymous users granted read-only access to topic %s\n", topic)
		} else if write {
			fmt.Fprintf(c.App.Writer, "Anonymous users granted write-only access to topic %s\n", topic)
		} else {
			fmt.Fprintf(c.App.Writer, "Revoked all access to topic %s for all anonymous users\n", topic)
		}
	} else {
		if read && write {
			fmt.Fprintf(c.App.Writer, "User %s now has read-write access to topic %s\n", username, topic)
		} else if read {
			fmt.Fprintf(c.App.Writer, "User %s now has read-only access to topic %s\n", username, topic)
		} else if write {
			fmt.Fprintf(c.App.Writer, "User %s now has write-only access to topic %s\n", username, topic)
		} else {
			fmt.Fprintf(c.App.Writer, "Revoked all access to topic %s for user %s\n", topic, username)
		}
	}
	user, err := manager.User(username)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.App.Writer)
	return showUsers(c, []*auth.User{user})
}

func doAccessReset(c *cli.Context, manager auth.Manager, username, topic string) error {
	if err := manager.ResetAccess(username, topic); err != nil {
		return err
	}
	if username == "" {
		if topic == "" {
			fmt.Fprintln(c.App.Writer, "Reset access for all anonymous users and all topics")
		} else {
			fmt.Fprintf(c.App.Writer, "Reset access to topic %s for all anonymous users\n", topic)
		}
	} else {
		if topic == "" {
			fmt.Fprintf(c.App.Writer, "Reset access for user %s to all topics\n", username)
		} else {
			fmt.Fprintf(c.App.Writer, "Reset access for user %s and topic %s\n", username, topic)
		}
	}
	return nil
}
