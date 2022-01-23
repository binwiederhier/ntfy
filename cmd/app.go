// Package cmd provides the ntfy CLI application
package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/util"
	"os"
)

var (
	defaultClientRootConfigFile = "/etc/ntfy/client.yml"
	defaultClientUserConfigFile = "~/.config/ntfy/client.yml"
)

const (
	categoryClient = "Client commands"
	categoryServer = "Server commands"
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
		Action:                 execMainApp,
		Before:                 initConfigFileInputSource("config", flagsServe), // DEPRECATED, see deprecation notice
		Flags:                  flagsServe,                                      // DEPRECATED, see deprecation notice
		Commands: []*cli.Command{
			// Server commands
			cmdServe,
			cmdUser,
			cmdAllow,
			cmdDeny,

			// Client commands
			cmdPublish,
			cmdSubscribe,
		},
	}
}

func execMainApp(c *cli.Context) error {
	fmt.Fprintln(c.App.ErrWriter, "\x1b[1;33mDeprecation notice: Please run the server using 'ntfy serve'; see 'ntfy -h' for help.\x1b[0m")
	fmt.Fprintln(c.App.ErrWriter, "\x1b[1;33mThis way of running the server will be removed March 2022. See https://ntfy.sh/docs/deprecations/ for details.\x1b[0m")
	return execServe(c)
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
