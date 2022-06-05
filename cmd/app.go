// Package cmd provides the ntfy CLI application
package cmd

import (
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
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
	&cli.BoolFlag{Name: "trace", EnvVars: []string{"NTFY_TRACE"}, Usage: "enable tracing (very verbose, be careful)"},
	&cli.BoolFlag{Name: "no-log-dates", Aliases: []string{"no_log_dates"}, EnvVars: []string{"NTFY_NO_LOG_DATES"}, Usage: "disable the date/time prefix"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "log-level", Aliases: []string{"log_level"}, Value: log.InfoLevel.String(), EnvVars: []string{"NTFY_LOG_LEVEL"}, Usage: "set log level"}),
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
		Before:                 initLogFunc,
	}
}

func initLogFunc(c *cli.Context) error {
	if c.Bool("trace") {
		log.SetLevel(log.TraceLevel)
	} else if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.ToLevel(c.String("log-level")))
	}
	if c.Bool("no-log-dates") {
		log.DisableDates()
	}
	return nil
}
