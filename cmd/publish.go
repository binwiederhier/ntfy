package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	commands = append(commands, cmdPublish)
}

var flagsPublish = append(
	flagsDefault,
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG"}, Usage: "client config file"},
	&cli.StringFlag{Name: "title", Aliases: []string{"t"}, EnvVars: []string{"NTFY_TITLE"}, Usage: "message title"},
	&cli.StringFlag{Name: "priority", Aliases: []string{"p"}, EnvVars: []string{"NTFY_PRIORITY"}, Usage: "priority of the message (1=min, 2=low, 3=default, 4=high, 5=max)"},
	&cli.StringFlag{Name: "tags", Aliases: []string{"tag", "T"}, EnvVars: []string{"NTFY_TAGS"}, Usage: "comma separated list of tags and emojis"},
	&cli.StringFlag{Name: "delay", Aliases: []string{"at", "in", "D"}, EnvVars: []string{"NTFY_DELAY"}, Usage: "delay/schedule message"},
	&cli.StringFlag{Name: "click", Aliases: []string{"U"}, EnvVars: []string{"NTFY_CLICK"}, Usage: "URL to open when notification is clicked"},
	&cli.StringFlag{Name: "actions", Aliases: []string{"A"}, EnvVars: []string{"NTFY_ACTIONS"}, Usage: "actions JSON array or simple definition"},
	&cli.StringFlag{Name: "attach", Aliases: []string{"a"}, EnvVars: []string{"NTFY_ATTACH"}, Usage: "URL to send as an external attachment"},
	&cli.StringFlag{Name: "filename", Aliases: []string{"name", "n"}, EnvVars: []string{"NTFY_FILENAME"}, Usage: "filename for the attachment"},
	&cli.StringFlag{Name: "file", Aliases: []string{"f"}, EnvVars: []string{"NTFY_FILE"}, Usage: "file to upload as an attachment"},
	&cli.StringFlag{Name: "email", Aliases: []string{"mail", "e"}, EnvVars: []string{"NTFY_EMAIL"}, Usage: "also send to e-mail address"},
	&cli.StringFlag{Name: "user", Aliases: []string{"u"}, EnvVars: []string{"NTFY_USER"}, Usage: "username[:password] used to auth against the server"},
	&cli.IntFlag{Name: "pid", Aliases: []string{"done", "w"}, EnvVars: []string{"NTFY_PID"}, Usage: "monitor process with given PID and publish when it exists"},
	&cli.BoolFlag{Name: "no-cache", Aliases: []string{"C"}, EnvVars: []string{"NTFY_NO_CACHE"}, Usage: "do not cache message server-side"},
	&cli.BoolFlag{Name: "no-firebase", Aliases: []string{"F"}, EnvVars: []string{"NTFY_NO_FIREBASE"}, Usage: "do not forward message to Firebase"},
	&cli.BoolFlag{Name: "env-topic", Aliases: []string{"P"}, EnvVars: []string{"NTFY_ENV_TOPIC"}, Usage: "use topic from NTFY_TOPIC env variable"},
	&cli.BoolFlag{Name: "quiet", Aliases: []string{"q"}, EnvVars: []string{"NTFY_QUIET"}, Usage: "do not print message"},
)

var cmdPublish = &cli.Command{
	Name:      "publish",
	Aliases:   []string{"pub", "send", "trigger"},
	Usage:     "Send message via a ntfy server",
	UsageText: "ntfy publish [OPTIONS..] TOPIC [MESSAGE]\nNTFY_TOPIC=.. ntfy publish [OPTIONS..] -P [MESSAGE]",
	Action:    execPublish,
	Category:  categoryClient,
	Flags:     flagsPublish,
	Before:    initLogFunc,
	Description: `Publish a message to a ntfy server.

Examples:
  ntfy publish mytopic This is my message                 # Send simple message
  ntfy send myserver.com/mytopic "This is my message"     # Send message to different default host
  ntfy pub -p high backups "Backups failed"               # Send high priority message
  ntfy pub --tags=warning,skull backups "Backups failed"  # Add tags/emojis to message
  ntfy pub --delay=10s delayed_topic Laterzz              # Delay message by 10s
  ntfy pub --at=8:30am delayed_topic Laterzz              # Send message at 8:30am
  ntfy pub -e phil@example.com alerts 'App is down!'      # Also send email to phil@example.com
  ntfy pub --click="https://reddit.com" redd 'New msg'    # Opens Reddit when notification is clicked
  ntfy pub --attach="http://some.tld/file.zip" files      # Send ZIP archive from URL as attachment
  ntfy pub --file=flower.jpg flowers 'Nice!'              # Send image.jpg as attachment
  ntfy pub -u phil:mypass secret Psst                     # Publish with username/password
  NTFY_USER=phil:mypass ntfy pub secret Psst              # Use env variables to set username/password
  NTFY_TOPIC=mytopic ntfy pub -P "some message""          # Use NTFY_TOPIC variable as topic 
  cat flower.jpg | ntfy pub --file=- flowers 'Nice!'      # Same as above, send image.jpg as attachment
  ntfy trigger mywebhook                                  # Sending without message, useful for webhooks
 
Please also check out the docs on publishing messages. Especially for the --tags and --delay options, 
it has incredibly useful information: https://ntfy.sh/docs/publish/.

` + clientCommandDescriptionSuffix,
}

func execPublish(c *cli.Context) error {
	conf, err := loadConfig(c)
	if err != nil {
		return err
	}
	title := c.String("title")
	priority := c.String("priority")
	tags := c.String("tags")
	delay := c.String("delay")
	click := c.String("click")
	actions := c.String("actions")
	attach := c.String("attach")
	filename := c.String("filename")
	file := c.String("file")
	email := c.String("email")
	user := c.String("user")
	pid := c.Int("pid")
	noCache := c.Bool("no-cache")
	noFirebase := c.Bool("no-firebase")
	envTopic := c.Bool("env-topic")
	quiet := c.Bool("quiet")
	var topic, message string
	if envTopic {
		topic = os.Getenv("NTFY_TOPIC")
		if c.NArg() > 0 {
			message = strings.Join(c.Args().Slice(), " ")
		}
	} else {
		if c.NArg() < 1 {
			return errors.New("must specify topic, type 'ntfy publish --help' for help")
		}
		topic = c.Args().Get(0)
		if c.NArg() > 1 {
			message = strings.Join(c.Args().Slice()[1:], " ")
		}
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
	if click != "" {
		options = append(options, client.WithClick(click))
	}
	if actions != "" {
		options = append(options, client.WithActions(strings.ReplaceAll(actions, "\n", " ")))
	}
	if attach != "" {
		options = append(options, client.WithAttach(attach))
	}
	if filename != "" {
		options = append(options, client.WithFilename(filename))
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
	var body io.Reader
	if file == "" {
		body = strings.NewReader(message)
	} else {
		if message != "" {
			options = append(options, client.WithMessage(message))
		}
		if file == "-" {
			if filename == "" {
				options = append(options, client.WithFilename("stdin"))
			}
			body = c.App.Reader
		} else {
			if filename == "" {
				options = append(options, client.WithFilename(filepath.Base(file)))
			}
			body, err = os.Open(file)
			if err != nil {
				return err
			}
		}
	}
	if pid > 0 {
		if err := waitForProcess(pid); err != nil {
			return err
		}
	}
	cl := client.New(conf)
	m, err := cl.PublishReader(topic, body, options...)
	if err != nil {
		return err
	}
	if !quiet {
		fmt.Fprintln(c.App.Writer, strings.TrimSpace(m.Raw))
	}
	return nil
}

func waitForProcess(pid int) error {
	if !processExists(pid) {
		return fmt.Errorf("process with PID %d not running", pid)
	}
	log.Debug("Waiting for process with PID %d to exit", pid)
	for processExists(pid) {
		time.Sleep(500 * time.Millisecond)
	}
	log.Debug("Process with PID %d exited", pid)
	return nil
}
