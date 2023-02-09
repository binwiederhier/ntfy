//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"time"
)

func init() {
	commands = append(commands, cmdTier)
}

const (
	defaultMessageLimit             = 5000
	defaultMessageExpiryDuration    = 12 * time.Hour
	defaultEmailLimit               = 20
	defaultReservationLimit         = 3
	defaultAttachmentFileSizeLimit  = "15M"
	defaultAttachmentTotalSizeLimit = "100M"
	defaultAttachmentExpiryDuration = 6 * time.Hour
	defaultAttachmentBandwidthLimit = "1G"
)

var (
	flagsTier = append([]cli.Flag{}, flagsUser...)
)

var cmdTier = &cli.Command{
	Name:      "tier",
	Usage:     "Manage/show tiers",
	UsageText: "ntfy tier [list|add|change|remove] ...",
	Flags:     flagsTier,
	Before:    initConfigFileInputSourceFunc("config", flagsUser, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "add",
			Aliases:   []string{"a"},
			Usage:     "Adds a new tier",
			UsageText: "ntfy tier add [OPTIONS] CODE",
			Action:    execTierAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "tier name"},
				&cli.Int64Flag{Name: "message-limit", Value: defaultMessageLimit, Usage: "daily message limit"},
				&cli.DurationFlag{Name: "message-expiry-duration", Value: defaultMessageExpiryDuration, Usage: "duration after which messages are deleted"},
				&cli.Int64Flag{Name: "email-limit", Value: defaultEmailLimit, Usage: "daily email limit"},
				&cli.Int64Flag{Name: "reservation-limit", Value: defaultReservationLimit, Usage: "topic reservation limit"},
				&cli.StringFlag{Name: "attachment-file-size-limit", Value: defaultAttachmentFileSizeLimit, Usage: "per-attachment file size limit"},
				&cli.StringFlag{Name: "attachment-total-size-limit", Value: defaultAttachmentTotalSizeLimit, Usage: "total size limit of attachments for the user"},
				&cli.DurationFlag{Name: "attachment-expiry-duration", Value: defaultAttachmentExpiryDuration, Usage: "duration after which attachments are deleted"},
				&cli.StringFlag{Name: "attachment-bandwidth-limit", Value: defaultAttachmentBandwidthLimit, Usage: "daily bandwidth limit for attachment uploads/downloads"},
				&cli.StringFlag{Name: "stripe-price-id", Usage: "Stripe price ID for paid tiers (e.g. price_12345)"},
			},
			Description: `Add a new tier to the ntfy user database.

Tiers can be used to grant users higher limits, such as daily message limits, attachment size, or
make it possible for users to reserve topics.

This is a server-only command. It directly reads from user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.

Examples:
  ntfy tier add pro                     # Add tier with code "pro", using the defaults
  ntfy tier add \                       # Add a tier with custom limits
    --name="Pro" \
    --message-limit=10000 \
    --message-expiry-duration=24h \
    --email-limit=50 \
    --reservation-limit=10 \
    --attachment-file-size-limit=100M \
    --attachment-total-size-limit=1G \
    --attachment-expiry-duration=12h \
    --attachment-bandwidth-limit=5G \
    pro
`,
		},
		{
			Name:      "change",
			Aliases:   []string{"ch"},
			Usage:     "Change a tier",
			UsageText: "ntfy tier change [OPTIONS] CODE",
			Action:    execTierChange,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "tier name"},
				&cli.Int64Flag{Name: "message-limit", Usage: "daily message limit"},
				&cli.DurationFlag{Name: "message-expiry-duration", Usage: "duration after which messages are deleted"},
				&cli.Int64Flag{Name: "email-limit", Usage: "daily email limit"},
				&cli.Int64Flag{Name: "reservation-limit", Usage: "topic reservation limit"},
				&cli.StringFlag{Name: "attachment-file-size-limit", Usage: "per-attachment file size limit"},
				&cli.StringFlag{Name: "attachment-total-size-limit", Usage: "total size limit of attachments for the user"},
				&cli.DurationFlag{Name: "attachment-expiry-duration", Usage: "duration after which attachments are deleted"},
				&cli.StringFlag{Name: "attachment-bandwidth-limit", Usage: "daily bandwidth limit for attachment uploads/downloads"},
				&cli.StringFlag{Name: "stripe-price-id", Usage: "Stripe price ID for paid tiers (e.g. price_12345)"},
			},
			Description: `Updates a tier to change the limits.

After updating a tier, you may have to restart the ntfy server to apply them 
to all visitors. 

This is a server-only command. It directly reads from user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.

Examples:
  ntfy tier change --name="Pro" pro        # Update the name of an existing tier
  ntfy tier change \                       # Update multiple limits and fields
    --message-expiry-duration=24h \
    --stripe-price-id=price_1234 \
    pro
`,
		},
		{
			Name:      "remove",
			Aliases:   []string{"del", "rm"},
			Usage:     "Removes a tier",
			UsageText: "ntfy tier remove CODE",
			Action:    execTierDel,
			Description: `Remove a tier from the ntfy user database.

You cannot remove a tier if there are users associated with a tier. Use "ntfy user change-tier"
to remove or switch their tier first.

This is a server-only command. It directly reads from user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.

Example:
  ntfy tier del pro
`,
		},
		{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "Shows a list of tiers",
			Action:  execTierList,
			Description: `Shows a list of all configured tiers.

This is a server-only command. It directly reads from user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.
`,
		},
	},
	Description: `Manage tiers of the ntfy server.

The command allows you to add/remove/change tiers in the ntfy user database. Tiers are used
to grant users higher limits, such as daily message limits, attachment size, or make it 
possible for users to reserve topics.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.

Examples:
  ntfy tier add pro                     # Add tier with code "pro", using the defaults
  ntfy tier change --name="Pro" pro     # Update the name of an existing tier
  ntfy tier del pro                     # Delete an existing tier
`,
}

func execTierAdd(c *cli.Context) error {
	code := c.Args().Get(0)
	if code == "" {
		return errors.New("tier code expected, type 'ntfy tier add --help' for help")
	} else if !user.AllowedTier(code) {
		return errors.New("tier code must consist only of numbers and letters")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if tier, _ := manager.Tier(code); tier != nil {
		return fmt.Errorf("tier %s already exists", code)
	}
	name := c.String("name")
	if name == "" {
		name = code
	}
	attachmentFileSizeLimit, err := util.ParseSize(c.String("attachment-file-size-limit"))
	if err != nil {
		return err
	}
	attachmentTotalSizeLimit, err := util.ParseSize(c.String("attachment-total-size-limit"))
	if err != nil {
		return err
	}
	attachmentBandwidthLimit, err := util.ParseSize(c.String("attachment-bandwidth-limit"))
	if err != nil {
		return err
	}
	tier := &user.Tier{
		ID:                       "", // Generated
		Code:                     code,
		Name:                     name,
		MessageLimit:             c.Int64("message-limit"),
		MessageExpiryDuration:    c.Duration("message-expiry-duration"),
		EmailLimit:               c.Int64("email-limit"),
		ReservationLimit:         c.Int64("reservation-limit"),
		AttachmentFileSizeLimit:  attachmentFileSizeLimit,
		AttachmentTotalSizeLimit: attachmentTotalSizeLimit,
		AttachmentExpiryDuration: c.Duration("attachment-expiry-duration"),
		AttachmentBandwidthLimit: attachmentBandwidthLimit,
		StripePriceID:            c.String("stripe-price-id"),
	}
	if err := manager.AddTier(tier); err != nil {
		return err
	}
	tier, err = manager.Tier(code)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "tier added\n\n")
	printTier(c, tier)
	return nil
}

func execTierChange(c *cli.Context) error {
	code := c.Args().Get(0)
	if code == "" {
		return errors.New("tier code expected, type 'ntfy tier change --help' for help")
	} else if !user.AllowedTier(code) {
		return errors.New("tier code must consist only of numbers and letters")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	tier, err := manager.Tier(code)
	if err == user.ErrTierNotFound {
		return fmt.Errorf("tier %s does not exist", code)
	} else if err != nil {
		return err
	}
	if c.IsSet("name") {
		tier.Name = c.String("name")
	}
	if c.IsSet("message-limit") {
		tier.MessageLimit = c.Int64("message-limit")
	}
	if c.IsSet("message-expiry-duration") {
		tier.MessageExpiryDuration = c.Duration("message-expiry-duration")
	}
	if c.IsSet("email-limit") {
		tier.EmailLimit = c.Int64("email-limit")
	}
	if c.IsSet("reservation-limit") {
		tier.ReservationLimit = c.Int64("reservation-limit")
	}
	if c.IsSet("attachment-file-size-limit") {
		tier.AttachmentFileSizeLimit, err = util.ParseSize(c.String("attachment-file-size-limit"))
		if err != nil {
			return err
		}
	}
	if c.IsSet("attachment-total-size-limit") {
		tier.AttachmentTotalSizeLimit, err = util.ParseSize(c.String("attachment-total-size-limit"))
		if err != nil {
			return err
		}
	}
	if c.IsSet("attachment-expiry-duration") {
		tier.AttachmentExpiryDuration = c.Duration("attachment-expiry-duration")
	}
	if c.IsSet("attachment-bandwidth-limit") {
		tier.AttachmentBandwidthLimit, err = util.ParseSize(c.String("attachment-bandwidth-limit"))
		if err != nil {
			return err
		}
	}
	if c.IsSet("stripe-price-id") {
		tier.StripePriceID = c.String("stripe-price-id")
	}
	if err := manager.UpdateTier(tier); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "tier updated\n\n")
	printTier(c, tier)
	return nil
}

func execTierDel(c *cli.Context) error {
	code := c.Args().Get(0)
	if code == "" {
		return errors.New("tier code expected, type 'ntfy tier del --help' for help")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	if _, err := manager.Tier(code); err == user.ErrTierNotFound {
		return fmt.Errorf("tier %s does not exist", code)
	}
	if err := manager.RemoveTier(code); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "tier %s removed\n", code)
	return nil
}

func execTierList(c *cli.Context) error {
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	tiers, err := manager.Tiers()
	if err != nil {
		return err
	}
	for _, tier := range tiers {
		printTier(c, tier)
	}
	return nil
}

func printTier(c *cli.Context, tier *user.Tier) {
	stripePriceID := tier.StripePriceID
	if stripePriceID == "" {
		stripePriceID = "(none)"
	}
	fmt.Fprintf(c.App.ErrWriter, "tier %s (id: %s)\n", tier.Code, tier.ID)
	fmt.Fprintf(c.App.ErrWriter, "- Name: %s\n", tier.Name)
	fmt.Fprintf(c.App.ErrWriter, "- Message limit: %d\n", tier.MessageLimit)
	fmt.Fprintf(c.App.ErrWriter, "- Message expiry duration: %s (%d seconds)\n", tier.MessageExpiryDuration.String(), int64(tier.MessageExpiryDuration.Seconds()))
	fmt.Fprintf(c.App.ErrWriter, "- Email limit: %d\n", tier.EmailLimit)
	fmt.Fprintf(c.App.ErrWriter, "- Reservation limit: %d\n", tier.ReservationLimit)
	fmt.Fprintf(c.App.ErrWriter, "- Attachment file size limit: %s\n", util.FormatSize(tier.AttachmentFileSizeLimit))
	fmt.Fprintf(c.App.ErrWriter, "- Attachment total size limit: %s\n", util.FormatSize(tier.AttachmentTotalSizeLimit))
	fmt.Fprintf(c.App.ErrWriter, "- Attachment expiry duration: %s (%d seconds)\n", tier.AttachmentExpiryDuration.String(), int64(tier.AttachmentExpiryDuration.Seconds()))
	fmt.Fprintf(c.App.ErrWriter, "- Attachment daily bandwidth limit: %s\n", util.FormatSize(tier.AttachmentBandwidthLimit))
	fmt.Fprintf(c.App.ErrWriter, "- Stripe price: %s\n", stripePriceID)
}
