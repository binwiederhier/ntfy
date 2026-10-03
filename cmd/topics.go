//go:build !noserver

package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func init() {
	commands = append(commands, cmdTopics)
}

var flagsTopics = append([]cli.Flag{}, flagsUser...)

var cmdTopics = &cli.Command{
	Name:      "topics",
	Usage:     "Manage topics",
	UsageText: "ntfy topics [fix-shared-read] ...",
	Flags:     flagsTopics,
	Before:    initConfigFileInputSourceFunc("config", flagsUser, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "fix-shared-read",
			Usage:     "Repair shared topics whose Everyone ACL is deny-all",
			UsageText: "ntfy topics fix-shared-read",
			Action:    execTopicsFixSharedRead,
			Description: `Repair shared topics that subscribers cannot actually read.

A topic whose visibility is "shared" but whose "everyone" ACL is still "deny-all" can be discovered
and subscribed to, but the subscriber receives no messages. New shares no longer have this problem
(sharing upgrades "everyone" from deny-all to read-only automatically), so this command exists only
to repair pre-existing data. It upgrades "everyone" from deny-all to read-only for every affected
topic, leaves broader grants (read-only/read-write) untouched, and prints each topic it changed.

This is a one-off, idempotent data repair, not a schema migration: running it again after it has
fixed everything is a no-op. It is a server-only command that directly manages the user.db as
defined in the server config file server.yml; the command only works if 'auth-file' (or
'database-url') is properly defined.

Examples:
  ntfy topics fix-shared-read   # Upgrade deny-all "everyone" grants on shared topics to read-only
`,
		},
	},
}

func execTopicsFixSharedRead(c *cli.Context) error {
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	changes, err := manager.BackfillSharedTopicReadAccess()
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Fprintln(c.App.Writer, "No shared topics need fixing; every shared topic's everyone ACL is already read-only or broader.")
		return nil
	}
	for _, change := range changes {
		fmt.Fprintf(
			c.App.Writer,
			"topic %s (owner %s): everyone %s -> %s\n",
			change.Topic,
			change.Owner,
			change.Before.String(),
			change.After.String(),
		)
	}
	fmt.Fprintf(c.App.Writer, "\nFixed %d shared topic(s).\n", len(changes))
	return nil
}
