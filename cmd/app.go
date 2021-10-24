// Package cmd provides the ntfy CLI application
package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/config"
	"heckel.io/ntfy/server"
	"log"
	"os"
)

// New creates a new CLI application
func New() *cli.App {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: "/etc/ntfy/config.yml", DefaultText: "/etc/ntfy/config.yml", Usage: "config file"},
		altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-http", Aliases: []string{"l"}, EnvVars: []string{"NTFY_LISTEN_HTTP"}, Value: config.DefaultListenHTTP, Usage: "ip:port used to as listen address"}),
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

	// Run main bot, can be killed by signal
	conf := config.New(listenHTTP)
	s := server.New(conf)
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
		if context.IsSet(configFlag) && !fileExists(configFile) {
			return fmt.Errorf("config file %s does not exist", configFile)
		} else if !context.IsSet(configFlag) && !fileExists(configFile) {
			return nil
		}
		inputSource, err := altsrc.NewYamlSourceFromFile(configFile)
		if err != nil {
			return err
		}
		return altsrc.ApplyInputSourceValues(context, inputSource, flags)
	}
}

func fileExists(filename string) bool {
	stat, _ := os.Stat(filename)
	return stat != nil
}
