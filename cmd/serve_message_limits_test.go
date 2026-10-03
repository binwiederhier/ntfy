package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/util"
)

func TestCLI_Serve_MessageFieldLimitsInvalid(t *testing.T) {
	for _, flag := range []string{"message-title-size-limit", "message-tags-size-limit"} {
		for _, value := range []string{"0", "-1", "bad", "999999999999999999999999999999"} {
			t.Run(flag+"/"+value, func(t *testing.T) {
				app, _, _, _ := newTestApp()
				err := app.Run([]string{"ntfy", "serve", "--config=" + newEmptyFile(t), "--base-url=https://example.com", "--" + flag + "=" + value})
				require.ErrorContains(t, err, "invalid message")
			})
		}
	}
}

func TestCLI_Serve_MessageFieldLimitsSources(t *testing.T) {
	for _, source := range []string{"default", "yaml", "env", "cli"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("NTFY_MESSAGE_TITLE_SIZE_LIMIT", "")
			t.Setenv("NTFY_MESSAGE_TAGS_SIZE_LIMIT", "")
			require.NoError(t, os.Unsetenv("NTFY_MESSAGE_TITLE_SIZE_LIMIT"))
			require.NoError(t, os.Unsetenv("NTFY_MESSAGE_TAGS_SIZE_LIMIT"))
			filename := filepath.Join(t.TempDir(), "server.yml")
			config := ""
			expectedTitle, expectedTags := int64(1024), int64(512)
			if source != "default" {
				config = "message-title-size-limit: 2K\nmessage-tags-size-limit: 1K\n"
				expectedTitle, expectedTags = 2048, 1024
			}
			if source == "env" || source == "cli" {
				t.Setenv("NTFY_MESSAGE_TITLE_SIZE_LIMIT", "3K")
				t.Setenv("NTFY_MESSAGE_TAGS_SIZE_LIMIT", "2K")
				expectedTitle, expectedTags = 3072, 2048
			}
			require.NoError(t, os.WriteFile(filename, []byte(config), 0600))
			args := []string{"ntfy", "serve", "--config=" + filename}
			if source == "cli" {
				args = append(args, "--message-title-size-limit=4K", "--message-tags-size-limit=3K")
				expectedTitle, expectedTags = 4096, 3072
			}
			command := *cmdServe
			called := false
			command.Action = func(c *cli.Context) error {
				title, err := util.ParseSize(c.String("message-title-size-limit"))
				require.NoError(t, err)
				tags, err := util.ParseSize(c.String("message-tags-size-limit"))
				require.NoError(t, err)
				require.Equal(t, expectedTitle, title)
				require.Equal(t, expectedTags, tags)
				called = true
				return nil
			}
			app, _, _, _ := newTestApp()
			app.Commands = []*cli.Command{&command}
			require.NoError(t, app.Run(args))
			require.True(t, called)
		})
	}
}
