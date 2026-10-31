//go:build windows

package server

import (
	"os"
	"path/filepath"
)

func init() {
	DefaultConfigFile = filepath.Join(programDataDir(), "ntfy", "server.yml")
	DefaultTemplateDir = filepath.Join(programDataDir(), "ntfy", "templates")
}

// DefaultListenAdminUnix returns the default admin socket path, %ProgramData%\ntfy\admin.sock.
// Access to it is restricted by a DACL, see prepareAdminSocketDir.
func DefaultListenAdminUnix() string {
	return filepath.Join(programDataDir(), "ntfy", "admin.sock")
}

func programDataDir() string {
	if programData := os.Getenv("ProgramData"); programData != "" {
		return programData
	}
	return `C:\ProgramData`
}
