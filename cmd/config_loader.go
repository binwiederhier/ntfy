package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"gopkg.in/yaml.v2"
	"heckel.io/ntfy/v2/util"
	"os"
)

// initConfigFileInputSourceFunc is like altsrc.InitInputSourceWithContext and altsrc.NewYamlSourceFromFlagFunc, but checks
// if the config flag is exists and only loads it if it does. If the flag is set and the file exists, it fails.
func initConfigFileInputSourceFunc(configFlag string, flags []cli.Flag, next cli.BeforeFunc) cli.BeforeFunc {
	return func(context *cli.Context) error {
		configFile := context.String(configFlag)
		if context.IsSet(configFlag) && !util.FileExists(configFile) {
			return fmt.Errorf("config file %s does not exist", configFile)
		} else if !context.IsSet(configFlag) && !util.FileExists(configFile) {
			return nil
		}
		inputSource, err := newYamlSourceFromFile(configFile, flags)
		if err != nil {
			return err
		}
		if err := altsrc.ApplyInputSourceValues(context, inputSource, flags); err != nil {
			return err
		}
		if next != nil {
			if err := next(context); err != nil {
				return err
			}
		}
		return nil
	}
}

// newYamlSourceFromFile creates a new Yaml InputSourceContext from a filepath.
//
// This function also maps aliases, so a .yml file can contain short options, or options with underscores
// instead of dashes. See https://github.com/binwiederhier/ntfy/issues/255.
func newYamlSourceFromFile(file string, flags []cli.Flag) (altsrc.InputSourceContext, error) {
	var rawConfig map[any]any
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &rawConfig); err != nil {
		return nil, err
	}
	for _, f := range flags {
		flagName := f.Names()[0]
		for _, flagAlias := range f.Names()[1:] {
			if _, ok := rawConfig[flagAlias]; ok {
				rawConfig[flagName] = rawConfig[flagAlias]
			}
		}
	}
	return altsrc.NewMapInputSource(file, rawConfig), nil
}
