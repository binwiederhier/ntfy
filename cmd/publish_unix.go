//go:build darwin || linux || dragonfly || freebsd || netbsd || openbsd
// +build darwin linux dragonfly freebsd netbsd openbsd

package cmd

import "syscall"

func processExists(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil
}
