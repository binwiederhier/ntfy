// Package cmd provides the ntfy CLI application
package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/util"
	"log"
	"os"
	"strings"
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
			cmdServe,
			cmdPublish,
			cmdSubscribe,
		},
	}
}

func execMainApp(c *cli.Context) error {
	log.Printf("\x1b[1;33mDeprecation notice: Please run the server using 'ntfy serve'; see 'ntfy -h' for help.\x1b[0m")
	log.Printf("\x1b[1;33mThis way of running the server will be removed Feb 2022.\x1b[0m")
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

func expandTopicURL(s string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	} else if strings.Contains(s, "/") {
		return fmt.Sprintf("https://%s", s)
	}
	return fmt.Sprintf("%s/%s", client.DefaultBaseURL, s)
}

func collapseTopicURL(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
}
