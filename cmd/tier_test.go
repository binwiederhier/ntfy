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
	require.Nil(t, runTierCommand(app, conf, "change",
		"--message-limit=999",
		"--message-expiry-duration=99h",
		"--email-limit=91",
		"--reservation-limit=98",
		"--attachment-file-size-limit=100m",
		"--attachment-expiry-duration=7h",
		"--attachment-total-size-limit=10G",
		"--attachment-bandwidth-limit=100G",
		"--stripe-monthly-price-id=price_991",
		"--stripe-yearly-price-id=price_992",
		"pro",
	))
	require.Contains(t, stderr.String(), "- Message limit: 999")
	require.Contains(t, stderr.String(), "- Message expiry duration: 99h")
	require.Contains(t, stderr.String(), "- Email limit: 91")
	require.Contains(t, stderr.String(), "- Reservation limit: 98")
	require.Contains(t, stderr.String(), "- Attachment file size limit: 100.0 MB")
	require.Contains(t, stderr.String(), "- Attachment expiry duration: 7h")
	require.Contains(t, stderr.String(), "- Attachment total size limit: 10.0 GB")
	require.Contains(t, stderr.String(), "- Stripe prices (monthly/yearly): price_991 / price_992")

	app, _, _, stderr = newTestApp()
	require.Nil(t, runTierCommand(app, conf, "remove", "pro"))
	require.Contains(t, stderr.String(), "tier pro removed")
}

func runTierCommand(app *cli.App, conf *server.Config, args ...string) error {
	userArgs := []string{
		"ntfy",
		"--log-level=ERROR",
		"tier",
		"--config=" + conf.File, // Dummy config file to avoid lookups of real file
		"--auth-file=" + conf.AuthFile,
		"--auth-default-access=" + conf.AuthDefault.String(),
	}
	return app.Run(append(userArgs, args...))
}
