// Package cmd provides the ntfy CLI application
package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/config"
	"heckel.io/ntfy/server"
	"heckel.io/ntfy/util"
	"log"
	"os"
	"time"
)

// New creates a new CLI application
func New() *cli.App {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: "/etc/ntfy/config.yml", DefaultText: "/etc/ntfy/config.yml", Usage: "config file"},
		altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-http", Aliases: []string{"l"}, EnvVars: []string{"NTFY_LISTEN_HTTP"}, Value: config.DefaultListenHTTP, Usage: "ip:port used to as listen address"}),
		altsrc.NewStringFlag(&cli.StringFlag{Name: "cache-file", Aliases: []string{"C"}, EnvVars: []string{"NTFY_CACHE_FILE"}, Usage: "cache file used for message caching"}),
		altsrc.NewStringFlag(&cli.StringFlag{Name: "firebase-key-file", Aliases: []string{"F"}, EnvVars: []string{"NTFY_FIREBASE_KEY_FILE"}, Usage: "Firebase credentials file; if set additionally publish to FCM topic"}),
		altsrc.NewDurationFlag(&cli.DurationFlag{Name: "message-buffer-duration", Aliases: []string{"b"}, EnvVars: []string{"NTFY_MESSAGE_BUFFER_DURATION"}, Value: config.DefaultMessageBufferDuration, Usage: "buffer messages in memory for this time to allow `since` requests"}),
		altsrc.NewDurationFlag(&cli.DurationFlag{Name: "keepalive-interval", Aliases: []string{"k"}, EnvVars: []string{"NTFY_KEEPALIVE_INTERVAL"}, Value: config.DefaultKeepaliveInterval, Usage: "default interval of keepalive messages"}),
		altsrc.NewDurationFlag(&cli.DurationFlag{Name: "manager-interval", Aliases: []string{"m"}, EnvVars: []string{"NTFY_MANAGER_INTERVAL"}, Value: config.DefaultManagerInterval, Usage: "default interval of for message pruning and stats printing"}),
	}
	return &cli.App{
		Name:                   "ntfy",
		Usage:                  "Simple pub-sub notification service",
		UsageText:              "ntfy [OPTION..]",
		HideHelp:               true,
		HideVersion:            true,
		EnableBashCompletion:   true,
		UseShortOptionHandling: true,
		Reader:                 os.Stdin,
		Writer:                 os.Stdout,
		ErrWriter:              os.Stderr,
		Action:                 execRun,
		Before:                 initConfigFileInputSource("config", flags),
		Flags:                  flags,
	}
}

func execRun(c *cli.Context) error {
	// Read all the options
	listenHTTP := c.String("listen-http")
	cacheFile := c.String("cache-file")
	firebaseKeyFile := c.String("firebase-key-file")
	messageBufferDuration := c.Duration("message-buffer-duration")
	keepaliveInterval := c.Duration("keepalive-interval")
	managerInterval := c.Duration("manager-interval")

	// Check values
	if firebaseKeyFile != "" && !util.FileExists(firebaseKeyFile) {
		return errors.New("if set, FCM key file must exist")
	} else if keepaliveInterval < 5*time.Second {
		return errors.New("keepalive interval cannot be lower than five seconds")
	} else if managerInterval < 5*time.Second {
		return errors.New("manager interval cannot be lower than five seconds")
	} else if messageBufferDuration < managerInterval {
		return errors.New("message buffer duration cannot be lower than manager interval")
	}

	// Run server
	conf := config.New(listenHTTP)
	conf.CacheFile = cacheFile
	conf.FirebaseKeyFile = firebaseKeyFile
	conf.MessageBufferDuration = messageBufferDuration
	conf.KeepaliveInterval = keepaliveInterval
	conf.ManagerInterval = managerInterval
	s, err := server.New(conf)
	if err != nil {
		log.Fatalln(err)
	}
	if err := s.Run(); err != nil {
		log.Fatalln(err)
	}
	log.Printf("Exiting.")
	return nil
}

// initConfigFileInputSource is like altsrc.InitInputSourceWithContext and altsrc.NewYamlSourceFromFlagFunc, but checks
// if the config flag is exists and only loads it if it does. If the flag is set and the file exists, it fails.
func initConfigFileInputSource(configFlag string, flags []cli.Flag) cli.BeforeFunc {
	return func(context *cli.Context) error {
		configFile := context.String(configFlag)
		if context.IsSet(configFlag) && !util.FileExists(configFile) {
			return fmt.Errorf("config file %s does not exist", configFile)
		} else if !context.IsSet(configFlag) && !util.FileExists(configFile) {
			return nil
		}
		inputSource, err := altsrc.NewYamlSourceFromFile(configFile)
		if err != nil {
			return err
		}
		return altsrc.ApplyInputSourceValues(context, inputSource, flags)
	}
}
