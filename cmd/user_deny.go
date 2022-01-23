package cmd

import (
	"errors"
	"github.com/urfave/cli/v2"
)

var flagsDeny = userCommandFlags()
var cmdDeny = &cli.Command{
	Name:      "deny",
	Usage:     "Revoke user access from a topic",
	UsageText: "ntfy deny USERNAME TOPIC",
	Flags:     flagsDeny,
	Before:    initConfigFileInputSource("config", flagsDeny),
	Action:    execUserDeny,
	Category:  categoryServer,
}

func execUserDeny(c *cli.Context) error {
	username := c.Args().Get(0)
	topic := c.Args().Get(1)
	if username == "" {
		return errors.New("username expected, type 'ntfy allow --help' for help")
	} else if topic == "" {
		return errors.New("topic expected, type 'ntfy allow --help' for help")
	}
	if username == "everyone" {
		username = ""
	}
	manager, err := createAuthManager(c)
	if err != nil {
		return err
	}
	return doAccessAllow(c, manager, username, topic, false, false)
}
