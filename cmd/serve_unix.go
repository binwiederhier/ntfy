//go:build linux || dragonfly || freebsd || netbsd || openbsd

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/server"
)

func sigHandlerConfigReload(config string) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP)
	for range sigs {
		log.Info("Partially hot reloading configuration ...")
		inputSource, err := newYamlSourceFromFile(config, flagsServe)
		if err != nil {
			log.Warn("Hot reload failed: %s", err.Error())
			continue
		}
		if err := reloadLogLevel(inputSource); err != nil {
			log.Warn("Reloading log level failed: %s", err.Error())
		}
	}
}

func reloadLogLevel(inputSource altsrc.InputSourceContext) error {
	newLevelStr, err := inputSource.String("log-level")
	if err != nil {
		return err
	}
	overrides, err := inputSource.StringSlice("log-level-overrides")
	if err != nil {
		return err
	}
	log.ResetLevelOverrides()
	if err := applyLogLevelOverrides(overrides); err != nil {
		return err
	}
	log.SetLevel(log.ToLevel(newLevelStr))
	if len(overrides) > 0 {
		log.Info("Log level is %v, %d override(s) in place", newLevelStr, len(overrides))
	} else {
		log.Info("Log level is %v", newLevelStr)
	}
	return nil
}

func maybeRunAsService(conf *server.Config) (bool, error) {
	return false, nil
}
