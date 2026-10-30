//go:build !windows

package server

import (
	"io/fs"
	"os"
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
