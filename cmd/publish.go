package cmd

import (
	"errors"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"strings"
)

var cmdPublish = &cli.Command{
	Name:      "publish",
	Aliases:   []string{"pub", "send", "push", "trigger"},
	Usage:     "Send message via a ntfy server",
	UsageText: "ntfy send [OPTIONS..] TOPIC MESSAGE",
	Action:    execPublish,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "title", Aliases: []string{"t"}, Usage: "message title"},
		&cli.StringFlag{Name: "priority", Aliases: []string{"p"}, Usage: "priority of the message (1=min, 2=low, 3=default, 4=high, 5=max)"},
		&cli.StringFlag{Name: "tags", Aliases: []string{"ta"}, Usage: "comma separated list of tags and emojis"},
		&cli.StringFlag{Name: "delay", Aliases: []string{"at", "in"}, Usage: "delay/schedule message"},
		&cli.BoolFlag{Name: "no-cache", Aliases: []string{"C"}, Usage: "do not cache message server-side"},
		&cli.BoolFlag{Name: "no-firebase", Aliases: []string{"F"}, Usage: "do not forward message to Firebase"},
	},
	Description: `Publish a message to a ntfy server.

Examples:
  ntfy publish mytopic This is my message                 # Send simple message
  ntfy send myserver.com/mytopic "This is my message"     # Send message to different default host
  ntfy pub -p high backups "Backups failed"               # Send high priority message
  ntfy pub --tags=warning,skull backups "Backups failed"  # Add tags/emojis to message
  ntfy pub --delay=10s delayed_topic Laterzz              # Delay message by 10s
  ntfy pub --at=8:30am delayed_topic Laterzz              # Send message at 8:30am
  ntfy trigger mywebhook                                  # Sending without message, useful for webhooks

Please also check out the docs on publishing messages. Especially for the --tags and --delay options, 
it has incredibly useful information: https://ntfy.sh/docs/publish/.`,
}

func execPublish(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("topic missing")
	}
	title := c.String("title")
	priority := c.String("priority")
	tags := c.String("tags")
	delay := c.String("delay")
	noCache := c.Bool("no-cache")
	noFirebase := c.Bool("no-firebase")
	topic := c.Args().Get(0)
	message := ""
	if c.NArg() > 1 {
		message = strings.Join(c.Args().Slice()[1:], " ")
	}
	var options []client.PublishOption
	if title != "" {
		options = append(options, client.WithTitle(title))
	}
	if priority != "" {
		options = append(options, client.WithPriority(priority))
	}
	if tags != "" {
		options = append(options, client.WithTags(tags))
	}
	if delay != "" {
		options = append(options, client.WithDelay(delay))
	}
	if noCache {
		options = append(options, client.WithNoCache())
	}
	if noFirebase {
		options = append(options, client.WithNoFirebase())
	}
	conf, err := loadConfig(c)
	if err != nil {
		return err
	}
	cl := client.New(conf)
	return cl.Publish(topic, message, options...)
}
