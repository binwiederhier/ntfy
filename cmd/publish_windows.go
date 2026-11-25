package cmd

import (
	"errors"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports while a process is running (STILL_ACTIVE)
const stillActive = 259

// processExists returns true if the process with the given PID is still running.
//
// On Windows, being able to open a process handle is not enough: an exited process stays
// around as long as anyone (including us) holds a handle to it, so we check its exit code.
func processExists(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// The process exists, but we may not query it (e.g. it belongs to another user)
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
