//go:build windows

package server

import (
	"os"
	"path/filepath"
)

func init() {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	DefaultConfigFile = filepath.Join(programData, "ntfy", "server.yml")
	DefaultTemplateDir = filepath.Join(programData, "ntfy", "templates")
}
