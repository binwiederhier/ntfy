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
	Name:      "web-push",
	Usage:     "Generate keys, in the future manage web push subscriptions",
	UsageText: "ntfy web-push [generate-keys]",
	Category:  categoryServer,

	Subcommands: []*cli.Command{
		{
			Action:    generateWebPushKeys,
			Name:      "generate-keys",
			Usage:     "Generate VAPID keys to enable browser background push notifications",
			UsageText: "ntfy web-push generate-keys",
			Category:  categoryServer,
		},
	},
}

func generateWebPushKeys(c *cli.Context) error {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}

	fmt.Fprintf(c.App.ErrWriter, `Keys generated.

VAPID Public Key:
%s

VAPID Private Key:
%s

---

Add the following lines to your config file:

web-push-enabled: true
web-push-public-key: %s
web-push-private-key: %s
web-push-subscriptions-file: <filename>
web-push-email-address: <email address>

Look at the docs for other methods (e.g. command line flags & environment variables).

You will also need to set a base-url.
`, publicKey, privateKey, publicKey, privateKey)

	return nil
}
