package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"gopkg.in/yaml.v2"
	"heckel.io/ntfy/v2/util"
	"os"
	"path/filepath"
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
	rawConfig, err := readYamlConfig(file, flags, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	return altsrc.NewMapInputSource(file, rawConfig), nil
}

func readYamlConfig(file string, flags []cli.Flag, active map[string]bool) (map[any]any, error) {
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	if active[resolved] {
		return nil, fmt.Errorf("config include cycle at %s", file)
	}
	active[resolved] = true
	defer delete(active, resolved)
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
	var includes []string
	switch value := rawConfig["include"].(type) {
	case nil:
	case string:
		includes = []string{value}
	case []any:
		for _, item := range value {
			path, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("config include in %s must contain file paths", file)
			}
			includes = append(includes, path)
		}
	default:
		return nil, fmt.Errorf("config include in %s must be a path or list of paths", file)
	}
	delete(rawConfig, "include")
	if rawConfig == nil {
		rawConfig = make(map[any]any)
	}
	for _, include := range includes {
		if include == "" {
			return nil, fmt.Errorf("config include in %s must not be empty", file)
		}
		if !filepath.IsAbs(include) {
			include = filepath.Join(filepath.Dir(file), include)
		}
		values, err := readYamlConfig(include, flags, active)
		if err != nil {
			return nil, fmt.Errorf("config include %s: %w", include, err)
		}
		for key, value := range values {
			rawConfig[key] = value
		}
	}
	return rawConfig, nil
}
