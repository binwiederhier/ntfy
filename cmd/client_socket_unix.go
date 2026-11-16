//go:build !noserver && !windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
)

// checkAdminSocketOwner refuses a socket that someone other than the server may have put there, e.g.
// another user in a world-writable directory like Docker's /run/ntfy, before the server started.
// Talking to it would hand that user the password hashes and tokens of the command.
func checkAdminSocketOwner(socket, authFile string) error {
	owner, ok := fileOwner(socket)
	if !ok {
		return nil // No socket (or no owner), the ping decides
	}
	trusted := []int{0, os.Getuid()}
	if dirOwner, ok := fileOwner(filepath.Dir(socket)); ok {
		trusted = append(trusted, dirOwner)
	}
	if authFile != "" {
		if authFileOwner, ok := fileOwner(authFile); ok {
			trusted = append(trusted, authFileOwner) // The server's user, which created it
		}
	}
	if !trustedAdminSocketOwner(owner, trusted) {
		return fmt.Errorf("refusing to use the admin socket %s: it is owned by uid %d, which is neither root, you, the owner of its directory, nor the owner of the auth-file", socket, owner)
	}
	return nil
}

func trustedAdminSocketOwner(owner int, trusted []int) bool {
	return slices.Contains(trusted, owner)
}

func fileOwner(path string) (int, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
