package cmd

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/util"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	defaultClientUserConfigFileRelative      = "ntfy\\client.yml"
	defaultClientConfigFileDescriptionSuffix = `The default config file for all client commands is %AppData%\ntfy\client.yml.`
)

func runCommandInternal(c *cli.Context, command string, m *client.Message) error {
	scriptFile, err := createTmpScript(command)
	if err != nil {
		return err
	}
	defer os.Remove(scriptFile)
	verbose := c.Bool("verbose")
	if verbose {
		log.Printf("[%s] Executing: %s (for message: %s)", util.ShortTopicURL(m.TopicURL), command, m.Raw)
	}
	cmd := exec.Command("cmd.exe", "/Q", "/C", scriptFile)
	cmd.Stdin = c.App.Reader
	cmd.Stdout = c.App.Writer
	cmd.Stderr = c.App.ErrWriter
	cmd.Env = envVars(m)
	return cmd.Run()
}

func createTmpScript(command string) (string, error) {
	scriptFile := fmt.Sprintf("%s/ntfy-subscribe-%s.bat", os.TempDir(), util.RandomString(10))
	if err := os.WriteFile(scriptFile, []byte(command), 0700); err != nil {
		return "", err
	}
	return scriptFile, nil
}

func defaultConfigFile() string {
	homeDir, _ := os.UserConfigDir()
	return filepath.Join(homeDir, defaultClientUserConfigFileRelative)
}
