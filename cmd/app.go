// Package cmd provides the ntfy CLI application
package cmd

import (
	"github.com/urfave/cli/v2"
	"os"
)

const (
	categoryClient = "Client commands"
	categoryServer = "Server commands"
)

var commands = make([]*cli.Command, 0)

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
	}
}
