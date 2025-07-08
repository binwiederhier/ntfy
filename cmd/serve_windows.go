// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found at: https://github.com/kardianos/minwinsvc

//go:build windows && !noserver

package cmd

import (
	"os"
	"sync"

	"golang.org/x/sys/windows/svc"
)

var (
	onExit func()
	guard  sync.Mutex
)

func init() {
	isService, err := svc.IsWindowsService()
	if err != nil {
		panic(err)
	}
	if !isService {
		return
	}
	go func() {
		_ = svc.Run("", runner{})

		guard.Lock()
		f := onExit
		guard.Unlock()

		// Don't hold this lock in user code.
		if f != nil {
			f()
		}
		// Make sure we exit.
		os.Exit(0)
	}()
}

func setOnExit(f func()) {
	guard.Lock()
	onExit = f
	guard.Unlock()
}

type runner struct{}

func (runner) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		}
	}

	return false, 0
}
