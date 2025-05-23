//go:build !noserver

package cmd

import (
	"fmt"
	"os"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
)

var flagsWebPush = append(
	[]cli.Flag{},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "output-file", Aliases: []string{"f"}, Usage: "write VAPID keys to this file"}),
)

func init() {
	commands = append(commands, cmdWebPush)
}

var cmdWebPush = &cli.Command{
	Name:      "webpush",
	Usage:     "Generate keys, in the future manage web push subscriptions",
	UsageText: "ntfy webpush [keys]",
	Category:  categoryServer,

	Subcommands: []*cli.Command{
		{
			Action:    generateWebPushKeys,
			Name:      "keys",
			Usage:     "Generate VAPID keys to enable browser background push notifications",
			UsageText: "ntfy webpush keys",
			Category:  categoryServer,
			Flags:     flagsWebPush,
		},
	},
}

func generateWebPushKeys(c *cli.Context) error {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}

	if outputFile := c.String("output-file"); outputFile != "" {
		contents := fmt.Sprintf(`---
web-push-public-key: %s
web-push-private-key: %s
`, publicKey, privateKey)
		err = os.WriteFile(outputFile, []byte(contents), 0660)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(c.App.ErrWriter, "Web Push keys written to %s.\n", outputFile)
	} else {
		_, err = fmt.Fprintf(c.App.ErrWriter, `Web Push keys generated. Add the following lines to your config file:

web-push-public-key: %s
web-push-private-key: %s
web-push-file: /var/cache/ntfy/webpush.db # or similar
web-push-email-address: <email address>

See https://ntfy.sh/docs/config/#web-push for details.
`, publicKey, privateKey)
	}
	return err
}
