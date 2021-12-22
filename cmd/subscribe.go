package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/util"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

var cmdSubscribe = &cli.Command{
	Name:      "subscribe",
	Aliases:   []string{"sub"},
	Usage:     "Subscribe to one or more topics on a ntfy server",
	UsageText: "ntfy subscribe [OPTIONS..] [TOPIC]",
	Action:    execSubscribe,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "client config file"},
		&cli.StringFlag{Name: "since", Aliases: []string{"s"}, Usage: "return events since `SINCE` (Unix timestamp, or all)"},
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
  
ntfy subscribe TOPIC COMMAND
  This executes COMMAND for every incoming messages. The message fields are passed to the
  command as environment variables:

    Variable        Aliases         Description
    --------------- --------------- -----------------------------------
    $NTFY_ID        $id             Unique message ID
    $NTFY_TIME      $time           Unix timestamp of the message delivery
    $NTFY_TOPIC     $topic          Topic name
    $NTFY_MESSAGE   $message, $m    Message body
    $NTFY_TITLE     $title, $t      Message title
    $NTFY_PRIORITY  $priority, $p   Message priority (1=min, 5=max)
    $NTFY_TAGS      $tags, $ta      Message tags (comma separated list)

  Examples:
    ntfy sub mytopic 'notify-send "$m"'    # Execute command for incoming messages
    ntfy sub topic1 /my/script.sh          # Execute script for incoming messages

ntfy subscribe --from-config
  Service mode (used in ntfy-client.service). This reads the config file (/etc/ntfy/client.yml 
  or ~/.config/ntfy/client.yml) and sets up subscriptions for every topic in the "subscribe:" 
  block (see config file).

  Examples: 
    ntfy sub --from-config                           # Read topics from config file
    ntfy sub --config=/my/client.yml --from-config   # Read topics from alternate config file

The default config file for all client commands is /etc/ntfy/client.yml (if root user),
or ~/.config/ntfy/client.yml for all other users.`,
}

func execSubscribe(c *cli.Context) error {
	// Read config and options
	conf, err := loadConfig(c)
	if err != nil {
		return err
	}
	cl := client.New(conf)
	since := c.String("since")
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
	commands := make(map[string]string) // Subscription ID -> command
	for _, s := range conf.Subscribe {  // May be nil
		topicOptions := append(make([]client.SubscribeOption, 0), options...)
		for filter, value := range s.If {
			topicOptions = append(topicOptions, client.WithFilter(filter, value))
		}
		subscriptionID := cl.Subscribe(s.Topic, topicOptions...)
		commands[subscriptionID] = s.Command
	}
	if topic != "" {
		subscriptionID := cl.Subscribe(topic, options...)
		commands[subscriptionID] = command
	}
	for m := range cl.Messages {
		command, ok := commands[m.SubscriptionID]
		if !ok {
			continue
		}
		printMessageOrRunCommand(c, m, command)
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

func runCommandInternal(c *cli.Context, command string, m *client.Message) error {
	scriptFile, err := createTmpScript(command)
	if err != nil {
		return err
	}
	defer os.Remove(scriptFile)
	verbose := c.Bool("verbose")
	if verbose {
		log.Printf("[%s] Executing: %s (for message: %s)", collapseTopicURL(m.TopicURL), command, m.Raw)
	}
	cmd := exec.Command("sh", "-c", scriptFile)
	cmd.Stdin = c.App.Reader
	cmd.Stdout = c.App.Writer
	cmd.Stderr = c.App.ErrWriter
	cmd.Env = envVars(m)
	return cmd.Run()
}

func createTmpScript(command string) (string, error) {
	scriptFile := fmt.Sprintf("%s/ntfy-subscribe-%s.sh.tmp", os.TempDir(), util.RandomString(10))
	script := fmt.Sprintf("#!/bin/sh\n%s", command)
	if err := os.WriteFile(scriptFile, []byte(script), 0700); err != nil {
		return "", err
	}
	return scriptFile, nil
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
	u, _ := user.Current()
	configFile := defaultClientRootConfigFile
	if u.Uid != "0" {
		configFile = util.ExpandHome(defaultClientUserConfigFile)
	}
	if s, _ := os.Stat(configFile); s != nil {
		return client.LoadConfig(configFile)
	}
	return client.NewConfig(), nil
}
