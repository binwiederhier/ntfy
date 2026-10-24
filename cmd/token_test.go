package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/test"
	"regexp"
	"testing"
)

func TestCLI_Token_AddListRemove(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "add", "phil"))
		require.Regexp(t, `token tk_.+ created for user phil, never expires`, stdout.String())

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "list", "phil"))
		require.Regexp(t, `user phil\n- tk_.+, never expires, accessed from 0.0.0.0 at .+`, stdout.String())
		re := regexp.MustCompile(`tk_\w+`)
		token := re.FindString(stdout.String())

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "remove", "phil", token))
		require.Regexp(t, fmt.Sprintf("token %s for user phil removed", token), stdout.String())

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "list"))
		require.Equal(t, "no users with tokens\n", stdout.String())
	})
}

func TestCLI_Token_Admin(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "--role=admin", "phil"))
		app, _, stdout, _ := newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "add", "--label=mine", "--expires=2d", "phil"))
		require.Regexp(t, `token tk_.+ created for user phil, expires `, stdout.String())

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runTokenCommand(app, conf, "list"))
		require.Regexp(t, `user phil\n- tk_.+ \(mine\), expires .+, accessed from 0.0.0.0 at .+`, stdout.String())

		app, _, _, _ = newTestApp()
		err := runTokenCommand(app, conf, "remove", "phil", "tk_doesnotexistdoesnotexist12345")
		require.Error(t, err)
		require.Equal(t, "token tk_doesnotexistdoesnotexist12345 for user phil does not exist", err.Error())
	})
}

func runTokenCommand(app *cli.App, conf *server.Config, args ...string) error {
	userArgs := []string{
		"ntfy",
		"--log-level=ERROR",
		"token",
		"--config=" + conf.File, // Dummy config file to avoid lookups of real file
		"--auth-file=" + conf.AuthFile,
		"--admin-socket=" + conf.AdminSocket,
	}
	return app.Run(append(userArgs, args...))
}
