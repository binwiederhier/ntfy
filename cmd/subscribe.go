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
	"strings"
)

var cmdSubscribe = &cli.Command{
	Name:      "subscribe",
	Aliases:   []string{"sub"},
	Usage:     "Subscribe to one or more topics on a ntfy server",
	UsageText: "ntfy subscribe [OPTIONS..] TOPIC",
	Action:    execSubscribe,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "exec", Aliases: []string{"e"}, Usage: "execute command for each message event"},
		&cli.StringFlag{Name: "since", Aliases: []string{"s"}, Usage: "return events since (Unix timestamp, or all)"},
		&cli.BoolFlag{Name: "poll", Aliases: []string{"p"}, Usage: "return events and exit, do not listen for new events"},
		&cli.BoolFlag{Name: "scheduled", Aliases: []string{"sched", "S"}, Usage: "also return scheduled/delayed events"},
	},
	Description: `(THIS COMMAND IS INCUBATING. IT MAY CHANGE WITHOUT NOTICE.)

Subscribe to one or more topics on a ntfy server, and either print 
or execute commands for every arriving message. 

By default, the subscribe command just prints the JSON representation of a message. 
When --exec is passed, each incoming message will execute a command. The message fields 
are passed to the command as environment variables:

    Variable        Aliases         Description
    --------------- --------------- -----------------------------------
    $NTFY_MESSAGE   $message, $m    Message body
    $NTFY_TITLE     $title, $t      Message title
    $NTFY_PRIORITY  $priority, $p   Message priority (1=min, 5=max)
    $NTFY_TAGS      $tags, $ta      Message tags (comma separated list)
    $NTFY_ID        $id             Unique message ID
    $NTFY_TIME      $time           Unix timestamp of the message delivery
    $NTFY_TOPIC     $topic          Topic name
    $NTFY_EVENT     $event, $ev     Event identifier (always "message")

Examples:
  ntfy subscribe mytopic                       # Prints JSON for incoming messages to stdout
  ntfy sub home.lan/backups alerts             # Subscribe to two different topics
  ntfy sub --exec='notify-send "$m"' mytopic   # Execute command for incoming messages
  ntfy sub --exec=/my/script topic1 topic2     # Subscribe to two topics and execute command for each message
`,
}

func execSubscribe(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("topic missing")
	}
	fmt.Fprintln(c.App.ErrWriter, "\x1b[1;33mThis command is incubating. The interface may change without notice.\x1b[0m")
	cl := client.DefaultClient
	command := c.String("exec")
	since := c.String("since")
	poll := c.Bool("poll")
	scheduled := c.Bool("scheduled")
	topics := c.Args().Slice()
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
	if poll {
		for _, topic := range topics {
			messages, err := cl.Poll(expandTopicURL(topic), options...)
			if err != nil {
				return err
			}
			for _, m := range messages {
				_ = dispatchMessage(c, command, m)
			}
		}
	} else {
		for _, topic := range topics {
			cl.Subscribe(expandTopicURL(topic), options...)
		}
		for m := range cl.Messages {
			_ = dispatchMessage(c, command, m)
		}
	}
	return nil
}

func dispatchMessage(c *cli.Context, command string, m *client.Message) error {
	if command != "" {
		return execCommand(c, command, m)
	}
	fmt.Println(m.Raw)
	return nil
}

func execCommand(c *cli.Context, command string, m *client.Message) error {
	if m.Event == client.OpenEvent {
		log.Printf("[%s] Connection opened, subscribed to topic", collapseTopicURL(m.TopicURL))
	} else if m.Event == client.MessageEvent {
		if err := runCommandInternal(c, command, m); err != nil {
			log.Printf("[%s] Command failed: %s", collapseTopicURL(m.TopicURL), err.Error())
		}
	}
	return nil
}

func runCommandInternal(c *cli.Context, command string, m *client.Message) error {
	scriptFile, err := createTmpScript(command)
	if err != nil {
		return err
	}
	defer os.Remove(scriptFile)
	log.Printf("[%s] Executing: %s (for message: %s)", collapseTopicURL(m.TopicURL), command, m.Raw)
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
	env = append(env, envVar(m.Event, "NTFY_EVENT", "event", "ev")...)
	env = append(env, envVar(m.Topic, "NTFY_TOPIC", "topic")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Time), "NTFY_TIME", "time")...)
	env = append(env, envVar(m.Message, "NTFY_MESSAGE", "message", "m")...)
	env = append(env, envVar(m.Title, "NTFY_TITLE", "title", "t")...)
	env = append(env, envVar(fmt.Sprintf("%d", m.Priority), "NTFY_PRIORITY", "priority", "prio", "p")...)
	env = append(env, envVar(strings.Join(m.Tags, ","), "NTFY_TAGS", "tags", "ta")...)
	return env
}

func envVar(value string, vars ...string) []string {
	env := make([]string, 0)
	for _, v := range vars {
		env = append(env, fmt.Sprintf("%s=%s", v, value))
	}
	return env
}
