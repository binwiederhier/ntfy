package cmd

import (
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/server"
	"heckel.io/ntfy/test"
	"testing"
)

func TestCLI_Tier_AddListChangeDelete(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
	defer test.StopServer(t, s, port)

	app, _, _, stderr := newTestApp()
	require.Nil(t, runTierCommand(app, conf, "add", "--name", "Pro", "--message-limit", "1234", "pro"))
	require.Contains(t, stderr.String(), "tier added\n\ntier pro (id: ti_")

	err := runTierCommand(app, conf, "add", "pro")
	require.NotNil(t, err)
	require.Equal(t, "tier pro already exists", err.Error())

	app, _, _, stderr = newTestApp()
	require.Nil(t, runTierCommand(app, conf, "list"))
	require.Contains(t, stderr.String(), "tier pro (id: ti_")
	require.Contains(t, stderr.String(), "- Name: Pro")
	require.Contains(t, stderr.String(), "- Message limit: 1234")

	app, _, _, stderr = newTestApp()
	require.Nil(t, runTierCommand(app, conf, "change", "--message-limit", "999", "pro"))
	require.Contains(t, stderr.String(), "- Message limit: 999")

	app, _, _, stderr = newTestApp()
	require.Nil(t, runTierCommand(app, conf, "remove", "pro"))
	require.Contains(t, stderr.String(), "tier pro removed")
}

func runTierCommand(app *cli.App, conf *server.Config, args ...string) error {
	userArgs := []string{
		"ntfy",
		"tier",
		"--auth-file=" + conf.AuthFile,
		"--auth-default-access=" + conf.AuthDefault.String(),
	}
	return app.Run(append(userArgs, args...))
}
