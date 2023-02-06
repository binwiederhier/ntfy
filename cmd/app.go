// Package cmd provides the ntfy CLI application
package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/log"
	"os"
	"regexp"
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
	altsrc.NewStringSliceFlag(&cli.StringSliceFlag{Name: "log-level-overrides", Aliases: []string{"log_level_overrides"}, EnvVars: []string{"NTFY_LOG_LEVEL_OVERRIDES"}, Usage: "set log level overrides"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "log-format", Aliases: []string{"log_format"}, Value: log.TextFormat.String(), EnvVars: []string{"NTFY_LOG_FORMAT"}, Usage: "set log format"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "log-file", Aliases: []string{"log_file"}, EnvVars: []string{"NTFY_LOG_FILE"}, Usage: "set log file, default is STDOUT"}),
}

var (
	logLevelOverrideRegex = regexp.MustCompile(`(?i)^([^=]+)\s*=\s*(\S+)\s*->\s*(TRACE|DEBUG|INFO|WARN|ERROR)$`)
)

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
	log.SetLevel(log.ToLevel(c.String("log-level")))
	log.SetFormat(log.ToFormat(c.String("log-format")))
	if c.Bool("trace") {
		log.SetLevel(log.TraceLevel)
	} else if c.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	if c.Bool("no-log-dates") {
		log.DisableDates()
	}
	if err := applyLogLevelOverrides(c.StringSlice("log-level-overrides")); err != nil {
		return err
	}
	logFile := c.String("log-file")
	if logFile != "" {
		w, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		log.SetOutput(w)
	}
	return nil
}

func applyLogLevelOverrides(rawOverrides []string) error {
	for _, override := range rawOverrides {
		m := logLevelOverrideRegex.FindStringSubmatch(override)
		if len(m) != 4 {
			return fmt.Errorf(`invalid log level override "%s", must be "field=value -> loglevel", e.g. "user_id=u_123 -> DEBUG"`, override)
		}
		field, value, level := m[1], m[2], m[3]
		log.SetLevelOverride(field, value, log.ToLevel(level))
	}
	return nil
}
