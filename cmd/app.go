// Package cmd provides the ntfy CLI application
package cmd

import (
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/log"
	"os"
)

const (
	categoryClient = "Client commands"
	categoryServer = "Server commands"
)

var commands = make([]*cli.Command, 0)

var flagsDefault = []cli.Flag{
	&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}, EnvVars: []string{"NTFY_DEBUG"}, Usage: "enable debug logging"},
	&cli.StringFlag{Name: "log-level", Aliases: []string{"log_level"}, Value: log.InfoLevel.String(), EnvVars: []string{"NTFY_LOG_LEVEL"}, Usage: "set log level"},
}

// New creates a new CLI application
func New() *cli.App {
	return &cli.App{
		Name:                   "ntfy",
		Usage:                  "Simple pub-sub notification service",
		UsageText:              "ntfy [OPTION..]",
		HideVersion:            true,
		UseShortOptionHandling: true,
		Reader:                 os.Stdin,
		Writer:                 os.Stdout,
		ErrWriter:              os.Stderr,
		Commands:               commands,
		Flags:                  flagsDefault,
		Before:                 initLogFunc(nil),
	}
}

func initLogFunc(next cli.BeforeFunc) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.ToLevel(c.String("log-level")))
		}
		if next != nil {
			if err := next(c); err != nil {
				return err
			}
		}
		return nil
	}
}
