package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/util"
	"os"
	"strings"
)

func init() {
	commands = append(commands, cmdSubscribe)
}

var cmdSubscribe = &cli.Command{
	Name:      "subscribe",
	Aliases:   []string{"sub"},
	Usage:     "Subscribe to one or more topics on a ntfy server",
	UsageText: "ntfy subscribe [OPTIONS..] [TOPIC]",
	Action:    execSubscribe,
	Category:  categoryClient,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "client config file"},
		&cli.StringFlag{Name: "since", Aliases: []string{"s"}, Usage: "return events since `SINCE` (Unix timestamp, or all)"},
		&cli.StringFlag{Name: "user", Aliases: []string{"u"}, Usage: "username[:password] used to auth against the server"},
		&cli.BoolFlag{Name: "from-config", Aliases: []string{"C"}, Usage: "read subscriptions from config file (service mode)"},
		&cli.BoolFlag{Name: "poll", Aliases: []string{"p"}, Usage: "return events and exit, do not listen for new events"},
		&cli.BoolFlag{Name: "scheduled", Aliases: []string{"sched", "S"}, Usage: "also return scheduled/delayed events"},
		&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "print verbose output"},
	},
	Description: `Subscribe to a topic from a ntfy server, and either print or execute a command for 
every arriving message. There are 3 modes in which the command can be run:

ntfy subscribe TOPIC
  This prints the JSON representation of every incoming message. It is useful when you
  have a command that wants to stream-read incoming JSON messages. Unless --poll is passed,
  this command stays open forever. 

  Examples:
    ntfy subscribe mytopic            # Prints JSON for incoming messages for ntfy.sh/mytopic
    ntfy sub home.lan/backups         # Subscribe to topic on different server
    ntfy sub --poll home.lan/backups  # Just query for latest messages and exit
    ntfy sub -u phil:mypass secret    # Subscribe with username/password
  
ntfy subscribe TOPIC COMMAND
  This executes COMMAND for every incoming messages. The message fields are passed to the
  command as environment variables:

    Variable        Aliases               Description
    --------------- --------------------- -----------------------------------
    $NTFY_ID        $id                   Unique message ID
    $NTFY_TIME      $time                 Unix timestamp of the message delivery
    $NTFY_TOPIC     $topic                Topic name
    $NTFY_MESSAGE   $message, $m          Message body
    $NTFY_TITLE     $title, $t            Message title
    $NTFY_PRIORITY  $priority, $prio, $p  Message priority (1=min, 5=max)
    $NTFY_TAGS      $tags, $tag, $ta      Message tags (comma separated list)
	$NTFY_RAW       $raw                  Raw JSON message

  Examples:
    ntfy sub mytopic 'notify-send "$m"'    # Execute command for incoming messages
    ntfy sub topic1 myscript.sh            # Execute script for incoming messages

ntfy subscribe --from-config
  Service mode (used in ntfy-client.service). This reads the config file and sets up 
  subscriptions for every topic in the "subscribe:" block (see config file).

  Examples: 
    ntfy sub --from-config                           # Read topics from config file
    ntfy sub --config=myclient.yml --from-config     # Read topics from alternate config file

` + defaultClientConfigFileDescriptionSuffix,
}

func execSubscribe(c *cli.Context) error {
	// Read config and options
	conf, err := loadConfig(c)
	if err != nil {
		return err
	}
	cl := client.New(conf)
	since := c.String("since")
	user := c.String("user")
	poll := c.Bool("poll")
	scheduled := c.Bool("scheduled")
	fromConfig := c.Bool("from-config")
	topic := c.Args().Get(0)
	command := c.Args().Get(1)
	if !fromConfig {
		conf.Subscribe = nil // wipe if --from-config not passed
	}
	var options []client.SubscribeOption
	if since != "" {
		options = append(options, client.WithSince(since))
	}
	if user != "" {
		var pass string
		parts := strings.SplitN(user, ":", 2)
		if len(parts) == 2 {
			user = parts[0]
			pass = parts[1]
		} else {
			fmt.Fprint(c.App.ErrWriter, "Enter Password: ")
			p, err := util.ReadPassword(c.App.Reader)
			if err != nil {
				return err
			}
			pass = string(p)
			fmt.Fprintf(c.App.ErrWriter, "\r%s\r", strings.Repeat(" ", 20))
		}
		options = append(options, client.WithBasicAuth(user, pass))
	}
	if poll {
		options = append(options, client.WithPoll())
	}
	if scheduled {
		options = append(options, client.WithScheduled())
	}
	if topic == "" && len(conf.Subscribe) == 0 {
		return errors.New("must specify topic, type 'ntfy subscribe --help' for help")
	}

	// Execute poll or subscribe
	if poll {
		return doPoll(c, cl, conf, topic, command, options...)
	}
	return doSubscribe(c, cl, conf, topic, command, options...)
}

func doPoll(c *cli.Context, cl *client.Client, conf *client.Config, topic, command string, options ...client.SubscribeOption) error {
	for _, s := range conf.Subscribe { // may be nil
		if err := doPollSingle(c, cl, s.Topic, s.Command, options...); err != nil {
			return err
		}
	}
	if topic != "" {
		if err := doPollSingle(c, cl, topic, command, options...); err != nil {
			return err
		}
	}
	return nil
}

func doPollSingle(c *cli.Context, cl *client.Client, topic, command string, options ...client.SubscribeOption) error {
	messages, err := cl.Poll(topic, options...)
	if err != nil {
		return err
	}
	for _, m := range messages {
		printMessageOrRunCommand(c, m, command)
	}
	return nil
}

func doSubscribe(c *cli.Context, cl *client.Client, conf *client.Config, topic, command string, options ...client.SubscribeOption) error {
	cmds := make(map[string]string)    // Subscription ID -> command
	for _, s := range conf.Subscribe { // May be nil
		topicOptions := append(make([]client.SubscribeOption, 0), options...)
		for filter, value := range s.If {
			topicOptions = append(topicOptions, client.WithFilter(filter, value))
		}
		if s.User != "" && s.Password != "" {
			topicOptions = append(topicOptions, client.WithBasicAuth(s.User, s.Password))
		}
		subscriptionID := cl.Subscribe(s.Topic, topicOptions...)
		cmds[subscriptionID] = s.Command
	}
	if topic != "" {
		subscriptionID := cl.Subscribe(topic, options...)
		cmds[subscriptionID] = command
	}
	for m := range cl.Messages {
		cmd, ok := cmds[m.SubscriptionID]
		if !ok {
			continue
		}
		printMessageOrRunCommand(c, m, cmd)
	}
	return nil
}

func printMessageOrRunCommand(c *cli.Context, m *client.Message, command string) {
	if command != "" {
		runCommand(c, command, m)
	} else {
		fmt.Fprintln(c.App.Writer, m.Raw)
	}
}

func runCommand(c *cli.Context, command string, m *client.Message) {
	if err := runCommandInternal(c, command, m); err != nil {
		fmt.Fprintf(c.App.ErrWriter, "Command failed: %s\n", err.Error())
	}
}

func envVars(m *client.Message) []string {
	env := os.Environ()
	env = append(env, envVar(m.ID, "NTFY_ID", "id")...)
	env = append(env, envVar(m.Topic, "NTFY_TOPIC", "topic")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Time), "NTFY_TIME", "time")...)
	env = append(env, envVar(m.Message, "NTFY_MESSAGE", "message", "m")...)
	env = append(env, envVar(m.Title, "NTFY_TITLE", "title", "t")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Priority), "NTFY_PRIORITY", "priority", "prio", "p")...)
	env = append(env, envVar(strings.Join(m.Tags, ","), "NTFY_TAGS", "tags", "tag", "ta")...)
	env = append(env, envVar(m.Raw, "NTFY_RAW", "raw")...)
	return env
}

func envVar(value string, vars ...string) []string {
	env := make([]string, 0)
	for _, v := range vars {
		env = append(env, fmt.Sprintf("%s=%s", v, value))
	}
	return env
}

func loadConfig(c *cli.Context) (*client.Config, error) {
	filename := c.String("config")
	if filename != "" {
		return client.LoadConfig(filename)
	}
	configFile := defaultConfigFile()
	if s, _ := os.Stat(configFile); s != nil {
		return client.LoadConfig(configFile)
	}
	return client.NewConfig(), nil
}
