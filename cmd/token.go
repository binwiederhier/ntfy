//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"time"
)

func init() {
	commands = append(commands, cmdToken)
}

var flagsToken = append([]cli.Flag{}, flagsUser...)

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

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead.

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

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead.`,
		},
		{
			Name:   "generate",
			Usage:  "Generates a random token",
			Action: execTokenGenerate,
			Description: `Randomly generate a token to be used in provisioned tokens.

This command only generates the token value, but does not persist it anywhere.
The output can be used in the 'auth-tokens' config option.`,
		},
	},
	Description: `Manage access tokens for individual users.

User access tokens can be used to publish, subscribe, or perform any other user-specific tasks.
Tokens have full access, and can perform any task a user can do. They are meant to be used to 
avoid spreading the password to various places.

This is a server-only command. It talks to the running ntfy server over its local admin socket
(admin-socket in server.yml, default /run/ntfy/admin.sock), so changes apply immediately. If no
server is running, it reads/writes the database defined in server.yml (auth-file or database-url)
directly. With --server and --token, it manages a remote server over HTTPS instead.

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
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	token, err := api.AddToken(username, label, expires)
	if err != nil {
		return adminError(err, username, "", "")
	}
	if token.Expires == 0 {
		fmt.Fprintf(c.App.Writer, "token %s created for user %s, never expires\n", token.Token, token.Username)
	} else {
		fmt.Fprintf(c.App.Writer, "token %s created for user %s, expires %v\n", token.Token, token.Username, time.Unix(token.Expires, 0).Format(time.UnixDate))
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
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	if err := api.RemoveToken(username, token); err != nil {
		return adminError(err, username, "", token)
	}
	fmt.Fprintf(c.App.Writer, "token %s for user %s removed\n", token, username)
	return nil
}

func execTokenList(c *cli.Context) error {
	username := c.Args().Get(0)
	if username == userEveryone || username == user.Everyone {
		return errors.New("username not allowed")
	}
	api, err := newAdminAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()
	tokens, err := api.Tokens(username)
	if err != nil {
		return adminError(err, username, "", "")
	} else if len(tokens) == 0 && username != "" {
		fmt.Fprintf(c.App.Writer, "user %s has no access tokens\n", username)
		return nil
	} else if len(tokens) == 0 {
		fmt.Fprintf(c.App.Writer, "no users with tokens\n")
		return nil
	}
	lastUsername := ""
	for _, t := range tokens {
		if t.Username != lastUsername {
			fmt.Fprintf(c.App.Writer, "user %s\n", t.Username)
			lastUsername = t.Username
		}
		var label, expires, provisioned string
		if t.Label != "" {
			label = fmt.Sprintf(" (%s)", t.Label)
		}
		if t.Expires == 0 {
			expires = "never expires"
		} else {
			expires = fmt.Sprintf("expires %s", time.Unix(t.Expires, 0).Format(time.RFC822))
		}
		if t.Provisioned {
			provisioned = " (server config)"
		}
		fmt.Fprintf(c.App.Writer, "- %s%s, %s, accessed from %s at %s%s\n", t.Token, label, expires, t.LastOrigin, time.Unix(t.LastAccess, 0).Format(time.RFC822), provisioned)
	}
	return nil
}

func execTokenGenerate(c *cli.Context) error {
	fmt.Fprintln(c.App.Writer, user.GenerateToken())
	return nil
}
