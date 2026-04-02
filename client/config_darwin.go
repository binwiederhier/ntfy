//go:build darwin

package client

import (
	"os"
	"os/user"
	"path/filepath"
)

func init() {
	u, err := user.Current()
	if err == nil && u.Uid == "0" {
		DefaultConfigFile = "/etc/ntfy/client.yml"
	} else if configDir, err := os.UserConfigDir(); err == nil {
		DefaultConfigFile = filepath.Join(configDir, "ntfy", "client.yml")
	}
}
