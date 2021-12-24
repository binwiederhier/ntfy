package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"strings"
)

var cmdPublish = &cli.Command{
	Name:      "publish",
	Aliases:   []string{"pub", "send", "trigger"},
	Usage:     "Send message via a ntfy server",
	UsageText: "ntfy send [OPTIONS..] TOPIC [MESSAGE]",
	Action:    execPublish,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "client config file"},
		&cli.StringFlag{Name: "title", Aliases: []string{"t"}, Usage: "message title"},
		&cli.StringFlag{Name: "priority", Aliases: []string{"p"}, Usage: "priority of the message (1=min, 2=low, 3=default, 4=high, 5=max)"},
		&cli.StringFlag{Name: "tags", Aliases: []string{"tag", "T"}, Usage: "comma separated list of tags and emojis"},
		&cli.StringFlag{Name: "delay", Aliases: []string{"at", "in", "D"}, Usage: "delay/schedule message"},
		&cli.StringFlag{Name: "email", Aliases: []string{"e-mail", "mail", "e"}, Usage: "also send to e-mail address"},
		&cli.BoolFlag{Name: "no-cache", Aliases: []string{"C"}, Usage: "do not cache message server-side"},
		&cli.BoolFlag{Name: "no-firebase", Aliases: []string{"F"}, Usage: "do not forward message to Firebase"},
		&cli.BoolFlag{Name: "quiet", Aliases: []string{"q"}, Usage: "do print message"},
	},
	Description: `Publish a message to a ntfy server.

Examples:
  ntfy publish mytopic This is my message                 # Send simple message
  ntfy send myserver.com/mytopic "This is my message"     # Send message to different default host
  ntfy pub -p high backups "Backups failed"               # Send high priority message
  ntfy pub --tags=warning,skull backups "Backups failed"  # Add tags/emojis to message
  ntfy pub --delay=10s delayed_topic Laterzz              # Delay message by 10s
  ntfy pub --at=8:30am delayed_topic Laterzz              # Send message at 8:30am
  ntfy pub -e phil@example.com alerts 'App is down!'      # Also send email to phil@example.com
  ntfy trigger mywebhook                                  # Sending without message, useful for webhooks

Please also check out the docs on publishing messages. Especially for the --tags and --delay options, 
it has incredibly useful information: https://ntfy.sh/docs/publish/.

The default config file for all client commands is /etc/ntfy/client.yml (if root user),
or ~/.config/ntfy/client.yml for all other users.`,
}

func execPublish(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("must specify topic, type 'ntfy publish --help' for help")
	}
	conf, err := loadConfig(c)
	if err != nil {
		return err
	}
	title := c.String("title")
	priority := c.String("priority")
	tags := c.String("tags")
	delay := c.String("delay")
	email := c.String("email")
	noCache := c.Bool("no-cache")
	noFirebase := c.Bool("no-firebase")
	quiet := c.Bool("quiet")
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
		options = append(options, client.WithTagsList(tags))
	}
	if delay != "" {
		options = append(options, client.WithDelay(delay))
	}
	if email != "" {
		options = append(options, client.WithEmail(email))
	}
	if noCache {
		options = append(options, client.WithNoCache())
	}
	if noFirebase {
		options = append(options, client.WithNoFirebase())
	}
	cl := client.New(conf)
	m, err := cl.Publish(topic, message, options...)
	if err != nil {
		return err
	}
	if !quiet {
		fmt.Fprintln(c.App.Writer, strings.TrimSpace(m.Raw))
	}
	return nil
}
