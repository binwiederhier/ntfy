//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"net/netip"
	"time"
)

func init() {
	commands = append(commands, cmdToken)
}

var flagsToken = flagsUser

var cmdToken = &cli.Command{
	Name:      "token",
	Usage:     "Create, list or delete user tokens",
	UsageText: "ntfy token [list|add|remove] ...",
	Flags:     flagsToken,
	Before:    initConfigFileInputSourceFunc("config", flagsToken, initLogFunc),
	Category:  categoryServer,
	Subcommands: []*cli.Command{
		{
			Name:      "add",
			Aliases:   []string{"a"},
			Usage:     "Create a new token",
			UsageText: "ntfy token add [--expires=<duration>] [--label=..] USERNAME",
			Action:    execTokenAdd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "expires", Aliases: []string{"e"}, Value: "", Usage: "token expires after"},
				&cli.StringFlag{Name: "label", Aliases: []string{"l"}, Value: "", Usage: "token label"},
			},
			Description: `Create a new user access token.

User access tokens can be used to publish, subscribe, or perform any other user-specific tasks.
Tokens have full access, and can perform any task a user can do. They are meant to be used to 
avoid spreading the password to various places.

Examples:
  ntfy token add phil                   # Create token for user phil which never expires
  ntfy token add --expires=2d phil      # Create token for user phil which expires in 2 days
  ntfy token add -e "tuesday, 8pm" phil # Create token for user phil which expires next Tuesday
  ntfy token add -l backups phil        # Create token for user phil with label "backups"`,
		},
		{
			Name:      "remove",
			Aliases:   []string{"del", "rm"},
			Usage:     "Removes a token",
			UsageText: "ntfy token remove USERNAME TOKEN",
			Action:    execTokenDel,
			Description: `Remove a token from the ntfy user database.

Example:
  ntfy token del phil tk_th2srHVlxrANQHAso5t0HuQ1J1TjN`,
		},
		{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "Shows a list of tokens",
			Action:  execTokenList,
			Description: `Shows a list of all tokens.

This is a server-only command. It directly reads from the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.`,
		},
	},
	Description: `Manage access tokens for individual users.

User access tokens can be used to publish, subscribe, or perform any other user-specific tasks.
Tokens have full access, and can perform any task a user can do. They are meant to be used to 
avoid spreading the password to various places.

This is a server-only command. It directly manages the user.db as defined in the server config
file server.yml. The command only works if 'auth-file' is properly defined.

Examples:
  ntfy token list                               # Shows list of tokens for all users
  ntfy token list phil                          # Shows list of tokens for user phil
  ntfy token add phil                           # Create token for user phil which never expires
  ntfy token add --expires=2d phil              # Create token for user phil which expires in 2 days
  ntfy token remove phil tk_th2srHVlxr...       # Delete token`,
}

func execTokenAdd(c *cli.Context) error {
	username := c.Args().Get(0)
	expiresStr := c.String("expires")
	label := c.String("label")
	if username == "" {
		return errors.New("username expected, type 'ntfy token add --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	expires := time.Unix(0, 0)
	if expiresStr != "" {
		var err error
		expires, err = util.ParseFutureTime(expiresStr, time.Now())
		if err != nil {
			return err
		}
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	u, err := manager.User(username)
	if err == user.ErrUserNotFound {
		return fmt.Errorf("user %s does not exist", username)
	} else if err != nil {
		return err
	}
	token, err := manager.CreateToken(u.ID, label, expires, netip.IPv4Unspecified())
	if err != nil {
		return err
	}
	if expires.Unix() == 0 {
		fmt.Fprintf(c.App.ErrWriter, "token %s created for user %s, never expires\n", token.Value, u.Name)
	} else {
		fmt.Fprintf(c.App.ErrWriter, "token %s created for user %s, expires %v\n", token.Value, u.Name, expires.Format(time.UnixDate))
	}
	return nil
}

func execTokenDel(c *cli.Context) error {
	username, token := c.Args().Get(0), c.Args().Get(1)
	if username == "" || token == "" {
		return errors.New("username and token expected, type 'ntfy token remove --help' for help")
	} else if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	u, err := manager.User(username)
	if err == user.ErrUserNotFound {
		return fmt.Errorf("user %s does not exist", username)
	} else if err != nil {
		return err
	}
	if err := manager.RemoveToken(u.ID, token); err != nil {
		return err
	}
	fmt.Fprintf(c.App.ErrWriter, "token %s for user %s removed\n", token, username)
	return nil
}

func execTokenList(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	manager, err := createUserManager(c)
	if err != nil {
		return err
	}
	var users []*user.User
	if username != "" {
		u, err := manager.User(username)
		if err == user.ErrUserNotFound {
			return fmt.Errorf("user %s does not exist", username)
		} else if err != nil {
			return err
		}
		users = append(users, u)
	} else {
		users, err = manager.Users()
		if err != nil {
			return err
		}
	}
	usersWithTokens := 0
	for _, u := range users {
		tokens, err := manager.Tokens(u.ID)
		if err != nil {
			return err
		} else if len(tokens) == 0 && username != "" {
			fmt.Fprintf(c.App.ErrWriter, "user %s has no access tokens\n", username)
			return nil
		} else if len(tokens) == 0 {
			continue
		}
		usersWithTokens++
		fmt.Fprintf(c.App.ErrWriter, "user %s\n", u.Name)
		for _, t := range tokens {
			var label, expires string
			if t.Label != "" {
				label = fmt.Sprintf(" (%s)", t.Label)
			}
			if t.Expires.Unix() == 0 {
				expires = "never expires"
			} else {
				expires = fmt.Sprintf("expires %s", t.Expires.Format(time.RFC822))
			}
			fmt.Fprintf(c.App.ErrWriter, "- %s%s, %s, accessed from %s at %s\n", t.Value, label, expires, t.LastOrigin.String(), t.LastAccess.Format(time.RFC822))
		}
	}
	if usersWithTokens == 0 {
		fmt.Fprintf(c.App.ErrWriter, "no users with tokens\n")
	}
	return nil
}
