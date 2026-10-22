package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/test"
	"testing"
)

func TestCLI_Access_Show(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, _, stdout, _ := newTestApp()
		require.Nil(t, runAccessCommand(app, conf))
		require.Contains(t, stdout.String(), "user * (role: anonymous, tier: none)\n- no topic-specific permissions\n- no access to any (other) topics (server config)")
	})
}

func TestCLI_Access_Grant_And_Publish(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("philpass\nphilpass\nbenpass\nbenpass")
		require.Nil(t, runUserCommand(app, conf, "add", "--role=admin", "phil"))
		require.Nil(t, runUserCommand(app, conf, "add", "ben"))
		require.Nil(t, runAccessCommand(app, conf, "ben", "announcements", "rw"))
		require.Nil(t, runAccessCommand(app, conf, "ben", "sometopic", "read"))
		require.Nil(t, runAccessCommand(app, conf, "everyone", "announcements", "read"))

		app, _, stdout, _ := newTestApp()
		require.Nil(t, runAccessCommand(app, conf))
		expected := `user phil (role: admin, tier: none)
- read-write access to all topics (admin role)
user ben (role: user, tier: none)
- read-write access to topic announcements
- read-only access to topic sometopic
user * (role: anonymous, tier: none)
- read-only access to topic announcements
- no access to any (other) topics (server config)
`
		require.Equal(t, expected, stdout.String())

		// See if access permissions match
		app, _, _, _ = newTestApp()
		require.Error(t, app.Run([]string{
			"ntfy",
			"publish",
			fmt.Sprintf("http://127.0.0.1:%d/announcements", port),
		}))
		require.Nil(t, app.Run([]string{
			"ntfy",
			"publish",
			"-u", "ben:benpass",
			fmt.Sprintf("http://127.0.0.1:%d/announcements", port),
		}))
		require.Nil(t, app.Run([]string{
			"ntfy",
			"publish",
			"-u", "phil:philpass",
			fmt.Sprintf("http://127.0.0.1:%d/announcements", port),
		}))
		require.Nil(t, app.Run([]string{
			"ntfy",
			"subscribe",
			"--poll",
			fmt.Sprintf("http://127.0.0.1:%d/announcements", port),
		}))
		require.Error(t, app.Run([]string{
			"ntfy",
			"subscribe",
			"--poll",
			fmt.Sprintf("http://127.0.0.1:%d/something-else", port),
		}))
	})
}

func TestCLI_Access_ShowUsers_OldServer(t *testing.T) {
	// Servers before v2.33.0 do not send the default access; skip that line instead of failing
	app, _, stdout, _ := newTestApp()
	c := cli.NewContext(app, nil, nil)
	require.Nil(t, showUsers(c, []*client.AdminUser{{Username: "*", Role: "anonymous"}}))
	require.Equal(t, "user * (role: anonymous, tier: none)\n- no topic-specific permissions\n", stdout.String())
}

func runAccessCommand(app *cli.App, conf *server.Config, args ...string) error {
	userArgs := []string{
		"ntfy",
		"--log-level=ERROR",
		"access",
		"--config=" + conf.File, // Dummy config file to avoid lookups of real file
		"--auth-file=" + conf.AuthFile,
		"--admin-socket=" + conf.AdminSocket,
		"--auth-default-access=" + conf.AuthDefault.String(),
	}
	return app.Run(append(userArgs, args...))
}
