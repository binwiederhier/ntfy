package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/server"
)

func TestCLI_WebPush_GenerateKeys(t *testing.T) {
	app, _, _, stderr := newTestApp()
	require.Nil(t, runWebPushCommand(app, server.NewConfig(), "keys"))
	require.Contains(t, stderr.String(), "Web Push keys generated.")
}

func runWebPushCommand(app *cli.App, conf *server.Config, args ...string) error {
	webPushArgs := []string{
		"ntfy",
		"--log-level=ERROR",
		"webpush",
	}
	return app.Run(append(webPushArgs, args...))
}
