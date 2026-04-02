//go:build windows

package client

import (
	"os"
	"path/filepath"
)

func init() {
	if configDir, err := os.UserConfigDir(); err == nil {
		DefaultConfigFile = filepath.Join(configDir, "ntfy", "client.yml")
	}
}
