//go:build (darwin || linux || dragonfly || freebsd || netbsd || openbsd) && !noserver

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

// sigHandlerShutdown stops the server gracefully on SIGTERM/SIGINT (systemd stop, Ctrl-C), so
// pending writes such as the message cache batch are persisted. It closes stopping first, so
// the caller can tell the resulting Run error apart from a real failure.
func sigHandlerShutdown(s *server.Server, stopping chan<- struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigs
	log.Info("Received %s, shutting down ...", sig)
	close(stopping)
	s.Stop()
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
