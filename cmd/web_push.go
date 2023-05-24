//go:build !noserver

package cmd

import (
	"fmt"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/urfave/cli/v2"
)

func init() {
	commands = append(commands, cmdWebPush)
}

var cmdWebPush = &cli.Command{
	Name:      "web-push-keys",
	Usage:     "Generate web push VAPID keys",
	UsageText: "ntfy web-push-keys",
	Category:  categoryServer,
	Action:    generateWebPushKeys,
}

func generateWebPushKeys(c *cli.Context) error {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}

	fmt.Fprintf(c.App.ErrWriter, `Add the following lines to your config file:
web-push-enabled: true
web-push-public-key: %s
web-push-private-key: %s
web-push-subscriptions-file: <filename>
web-push-email-address: <email address>
`, publicKey, privateKey)

	return nil
}
