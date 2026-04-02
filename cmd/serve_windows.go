//go:build windows && !noserver

package cmd

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows/svc"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/server"
)

const serviceName = "ntfy"

// sigHandlerConfigReload is a no-op on Windows since SIGHUP is not available.
// Windows users can restart the service to reload configuration.
func sigHandlerConfigReload(config string) {
	log.Debug("Config hot-reload via SIGHUP is not supported on Windows")
}

// runAsWindowsService runs the ntfy server as a Windows service
func runAsWindowsService(conf *server.Config) error {
	return svc.Run(serviceName, &windowsService{conf: conf})
}

// windowsService implements the svc.Handler interface
type windowsService struct {
	conf   *server.Config
	server *server.Server
	mu     sync.Mutex
}

// Execute is the main entry point for the Windows service
func (s *windowsService) Execute(args []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}

	// Create and start the server
	var err error
	s.mu.Lock()
	s.server, err = server.New(s.conf)
	s.mu.Unlock()
	if err != nil {
		log.Error("Failed to create server: %s", err.Error())
		return true, 1
	}

	// Start server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- s.server.Run()
	}()

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	log.Info("Windows service started")

	for {
		select {
		case err := <-serverErrChan:
			if err != nil {
				log.Error("Server error: %s", err.Error())
				return true, 1
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				log.Info("Windows service stopping...")
				status <- svc.Status{State: svc.StopPending}
				s.mu.Lock()
				if s.server != nil {
					s.server.Stop()
				}
				s.mu.Unlock()
				return false, 0
			default:
				log.Warn("Unexpected service control request: %d", req.Cmd)
			}
		}
	}
}

// maybeRunAsService checks if the process is running as a Windows service,
// and if so, runs the server as a service. Returns true if it ran as a service.
func maybeRunAsService(conf *server.Config) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("failed to detect Windows service mode: %w", err)
	} else if !isService {
		return false, nil
	}
	log.Info("Running as Windows service")
	if err := runAsWindowsService(conf); err != nil {
		return true, fmt.Errorf("failed to run as Windows service: %w", err)
	}
	return true, nil
}
