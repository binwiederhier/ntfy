//go:build !windows

package server

import (
	"runtime"
)

func init() {
	DefaultConfigFile = "/etc/ntfy/server.yml"
	DefaultTemplateDir = "/etc/ntfy/templates"
}

// DefaultListenAdminUnix returns the default admin socket path
func DefaultListenAdminUnix() string {
	if runtime.GOOS == "darwin" {
		return "/var/run/ntfy/admin.sock"
	}
	return "/run/ntfy/admin.sock"
}
