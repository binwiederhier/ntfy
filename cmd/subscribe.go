package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
)

func init() {
	commands = append(commands, cmdSubscribe)
}

const (
	clientRootConfigFileUnixAbsolute    = "/etc/ntfy/client.yml"
	clientUserConfigFileUnixRelative    = "ntfy/client.yml"
	clientUserConfigFileWindowsRelative = "ntfy\\client.yml"
)

var flagsSubscribe = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "client config file"},
	&cli.StringFlag{Name: "since", Aliases: []string{"s"}, Usage: "return events since `SINCE` (Unix timestamp, or all)"},
	&cli.StringFlag{Name: "user", Aliases: []string{"u"}, EnvVars: []string{"NTFY_USER"}, Usage: "username[:password] used to auth against the server"},
	&cli.BoolFlag{Name: "from-config", Aliases: []string{"from_config", "C"}, Usage: "read subscriptions from config file (service mode)"},
	&cli.BoolFlag{Name: "poll", Aliases: []string{"p"}, Usage: "return events and exit, do not listen for new events"},
	&cli.BoolFlag{Name: "scheduled", Aliases: []string{"sched", "S"}, Usage: "also return scheduled/delayed events"},
)

var cmdSubscribe = &cli.Command{
	Name:      "subscribe",
	Aliases:   []string{"sub"},
	Usage:     "Subscribe to one or more topics on a ntfy server",
	UsageText: "ntfy subscribe [OPTIONS..] [TOPIC]",
	Action:    execSubscribe,
	Category:  categoryClient,
	Flags:     flagsSubscribe,
	Before:    initLogFunc,
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

` + clientCommandDescriptionSuffix,
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
		var user string
		var password *string
		if s.User != "" {
			user = s.User
		} else if conf.DefaultUser != "" {
			user = conf.DefaultUser
		}
		if s.Password != nil {
			password = s.Password
		} else if conf.DefaultPassword != nil {
			password = conf.DefaultPassword
		}
		if user != "" && password != nil {
			topicOptions = append(topicOptions, client.WithBasicAuth(user, *password))
		}
		subscriptionID := cl.Subscribe(s.Topic, topicOptions...)
		if s.Command != "" {
			cmds[subscriptionID] = s.Command
		} else if conf.DefaultCommand != "" {
			cmds[subscriptionID] = conf.DefaultCommand
		} else {
			cmds[subscriptionID] = ""
		}
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
		log.Debug("%s Dispatching received message: %s", logMessagePrefix(m), m.Raw)
		printMessageOrRunCommand(c, m, cmd)
	}
	return nil
}

func printMessageOrRunCommand(c *cli.Context, m *client.Message, command string) {
	if command != "" {
		runCommand(c, command, m)
	} else {
		log.Debug("%s Printing raw message", logMessagePrefix(m))
		fmt.Fprintln(c.App.Writer, m.Raw)
	}
}

func runCommand(c *cli.Context, command string, m *client.Message) {
	if err := runCommandInternal(c, command, m); err != nil {
		log.Warn("%s Command failed: %s", logMessagePrefix(m), err.Error())
	}
}

func runCommandInternal(c *cli.Context, script string, m *client.Message) error {
	scriptFile := fmt.Sprintf("%s/ntfy-subscribe-%s.%s", os.TempDir(), util.RandomString(10), scriptExt)
	log.Debug("%s Running command '%s' via temporary script %s", logMessagePrefix(m), script, scriptFile)
	script = scriptHeader + script
	if err := os.WriteFile(scriptFile, []byte(script), 0700); err != nil {
		return err
	}
	defer os.Remove(scriptFile)
	log.Debug("%s Executing script %s", logMessagePrefix(m), scriptFile)
	cmd := exec.Command(scriptLauncher[0], append(scriptLauncher[1:], scriptFile)...)
	cmd.Stdin = c.App.Reader
	cmd.Stdout = c.App.Writer
	cmd.Stderr = c.App.ErrWriter
	cmd.Env = envVars(m)
	return cmd.Run()
}

func envVars(m *client.Message) []string {
	env := make([]string, 0)
	env = append(env, envVar(m.ID, "NTFY_ID", "id")...)
	env = append(env, envVar(m.Topic, "NTFY_TOPIC", "topic")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Time), "NTFY_TIME", "time")...)
	env = append(env, envVar(m.Message, "NTFY_MESSAGE", "message", "m")...)
	env = append(env, envVar(m.Title, "NTFY_TITLE", "title", "t")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Priority), "NTFY_PRIORITY", "priority", "prio", "p")...)
	env = append(env, envVar(strings.Join(m.Tags, ","), "NTFY_TAGS", "tags", "tag", "ta")...)
	env = append(env, envVar(m.Raw, "NTFY_RAW", "raw")...)
	sort.Strings(env)
	if log.IsTrace() {
		log.Trace("%s With environment:\n%s", logMessagePrefix(m), strings.Join(env, "\n"))
	}
	return append(os.Environ(), env...)
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
	configFile := defaultClientConfigFile()
	if s, _ := os.Stat(configFile); s != nil {
		return client.LoadConfig(configFile)
	}
	return client.NewConfig(), nil
}

//lint:ignore U1000 Conditionally used in different builds
func defaultClientConfigFileUnix() string {
	u, _ := user.Current()
	configFile := clientRootConfigFileUnixAbsolute
	if u.Uid != "0" {
		homeDir, _ := os.UserConfigDir()
		return filepath.Join(homeDir, clientUserConfigFileUnixRelative)
	}
	return configFile
}

//lint:ignore U1000 Conditionally used in different builds
func defaultClientConfigFileWindows() string {
	homeDir, _ := os.UserConfigDir()
	return filepath.Join(homeDir, clientUserConfigFileWindowsRelative)
}

func logMessagePrefix(m *client.Message) string {
	return fmt.Sprintf("%s/%s", util.ShortTopicURL(m.TopicURL), m.ID)
}
