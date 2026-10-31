//go:build !windows

package server

import (
	"io/fs"
	"os"
	"syscall"
)

const (
	adminSocketDirMode = fs.FileMode(0750)
)

// prepareAdminSocketDir creates the socket's directory. An existing directory is left alone,
// the socket's own mode (see restrictAdminSocket) is what protects it.
func prepareAdminSocketDir(dir string) error {
	return os.MkdirAll(dir, adminSocketDirMode)
}

// restrictAdminSocket limits the socket to its owner (the ntfy user) and group
func restrictAdminSocket(path string) error {
	return os.Chmod(path, DefaultListenAdminUnixMode)
}

// insecureAdminSocketDir returns why other users could use or replace an admin socket in dir, or
// "" if they cannot. A directory like /tmp (world-writable, but sticky) is fine: only the owner can
// remove or rename the socket there.
func insecureAdminSocketDir(dir string) string {
	info, err := os.Stat(dir)
	if err != nil {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	mode, sticky := info.Mode(), info.Mode()&fs.ModeSticky != 0
	ownGroup := int(st.Gid) == os.Getgid()
	switch {
	case int(st.Uid) != os.Getuid() && st.Uid != 0:
		return "is owned by another user"
	case mode&0002 != 0 && !sticky:
		return "is writable by all users"
	case mode&0020 != 0 && !ownGroup && !sticky:
		return "is writable by another group"
	case mode&fs.ModeSetgid != 0 && !ownGroup:
		return "has the setgid bit for another group, which the socket inherits"
	}
	return ""
}
