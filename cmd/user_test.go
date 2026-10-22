package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/test"
	"heckel.io/ntfy/v2/user"
)

func TestCLI_User_Add(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")
	})
}

func TestCLI_User_Add_Exists(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")

		app, stdin, _, _ = newTestApp()
		stdin.WriteString("mypass\nmypass")
		err := runUserCommand(app, conf, "add", "phil")
		require.Error(t, err)
		require.Contains(t, err.Error(), "user phil already exists")
	})
}

func TestCLI_User_Add_Admin(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "--role=admin", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role admin")
	})
}

func TestCLI_User_Add_Password_Mismatch(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nNOTMATCH")
		err := runUserCommand(app, conf, "add", "phil")
		require.Error(t, err)
		require.Contains(t, err.Error(), "passwords do not match: try it again, but this time type slooowwwlly")
	})
}

func TestCLI_User_ChangePass(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		conf.AuthUsers = []*user.User{
			{Name: "philuser", Hash: "$2a$10$U4WSIYY6evyGmZaraavM2e2JeVG6EMGUKN1uUwufUeeRd4Jpg6cGC", Role: user.RoleUser}, // philuser:philpass
		}
		defer test.StopServer(t, s, port)

		// Add user
		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")

		// Change pass
		app, stdin, stdout, _ = newTestApp()
		stdin.WriteString("newpass\nnewpass")
		require.Nil(t, runUserCommand(app, conf, "change-pass", "phil"))
		require.Contains(t, stdout.String(), "changed password for user phil")

		// Cannot change provisioned user's pass
		app, stdin, _, _ = newTestApp()
		stdin.WriteString("newpass\nnewpass")
		require.Error(t, runUserCommand(app, conf, "change-pass", "philuser"))
	})
}

func TestCLI_User_ChangeRole(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		// Add user
		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")

		// Change role
		app, _, stdout, _ = newTestApp()
		require.Nil(t, runUserCommand(app, conf, "change-role", "phil", "admin"))
		require.Contains(t, stdout.String(), "changed role for user phil to admin")
	})
}

func TestCLI_User_Delete(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		// Add user
		app, stdin, stdout, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Contains(t, stdout.String(), "user phil added with role user")

		// Delete user
		app, _, stdout, _ = newTestApp()
		require.Nil(t, runUserCommand(app, conf, "del", "phil"))
		require.Contains(t, stdout.String(), "user phil removed")

		// Delete user again (does not exist)
		app, _, _, _ = newTestApp()
		err := runUserCommand(app, conf, "del", "phil")
		require.Error(t, err)
		require.Contains(t, err.Error(), "user phil does not exist")
	})
}

func TestCLI_User_ResetPass(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket, withBaseURL)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))

		// Prints a working-looking reset link when base-url is set
		app, _, stdout, _ := newTestApp()
		require.Nil(t, runUserCommand(app, conf, "--base-url=https://ntfy.example.com", "reset-pass", "phil"))
		require.Contains(t, stdout.String(), "https://ntfy.example.com/account/password/reset/")
	})
}

func TestCLI_User_ResetPass_NoBaseURL(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))

		app, _, _, _ = newTestApp()
		err := runUserCommand(app, conf, "reset-pass", "phil")
		require.Error(t, err)
		require.Contains(t, err.Error(), "base-url")
	})
}

func TestCLI_User_ResetPass_SendEmailNoPrimary(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket, withBaseURL, withSMTP)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))

		// --send-email requires a primary email; phil has none
		app, _, _, _ = newTestApp()
		err := runUserCommand(app, conf, "--base-url=https://ntfy.example.com", "--smtp-sender-addr=localhost:25", "--smtp-sender-from=ntfy@example.com", "reset-pass", "--send-email", "phil")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no primary email")
	})
}

func TestCLI_User_ResetPass_ProvisionedRejected(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket, withBaseURL)
		defer test.StopServer(t, s, port)

		// Seed a provisioned user into the auth database via config provisioning
		m, err := user.NewSQLiteManager(conf.AuthFile, "", &user.Config{
			ProvisionEnabled: true,
			Users: []*user.User{
				{Name: "provuser", Hash: "$2a$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", Role: user.RoleUser},
			},
		})
		require.Nil(t, err)
		require.Nil(t, m.Close())

		app, _, _, _ := newTestApp()
		err = runUserCommand(app, conf, "--base-url=https://ntfy.example.com", "reset-pass", "provuser")
		require.Error(t, err)
		require.Contains(t, err.Error(), "provisioned")
	})
}

func TestCLI_User_AuthFileNotReadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	dir := t.TempDir()
	authFile := filepath.Join(dir, "user.db")
	require.Nil(t, os.WriteFile(authFile, []byte{}, 0600))
	require.Nil(t, os.Chmod(dir, 0))
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	configFile := filepath.Join(t.TempDir(), "server.yml")
	require.Nil(t, os.WriteFile(configFile, []byte{}, 0600))
	app, _, _, _ := newTestApp()
	err := app.Run([]string{"ntfy", "user", "--config=" + configFile, "--auth-file=" + authFile, "--admin-socket=", "list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
	require.NotContains(t, err.Error(), "does not exist")
}

// forEachAdminMode runs a CLI test twice: once with the commands talking to the test server over
// its admin socket, and once with the socket disabled, i.e. writing to the database directly.
// Both must produce the same output.
func forEachAdminMode(t *testing.T, f func(t *testing.T, socket bool)) {
	t.Run("socket", func(t *testing.T) {
		f(t, true)
	})
	t.Run("direct", func(t *testing.T) {
		f(t, false)
	})
}

func TestCLI_User_ChangeTier_Reset(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass")
		require.Nil(t, runUserCommand(app, conf, "add", "phil"))
		require.Nil(t, runTierCommand(app, conf, "add", "--name=Pro", "pro"))

		app, _, stdout, _ := newTestApp()
		require.Nil(t, runUserCommand(app, conf, "change-tier", "phil", "pro"))
		require.Contains(t, stdout.String(), "changed tier for user phil to pro")
		app, _, stdout, _ = newTestApp()
		require.Nil(t, runUserCommand(app, conf, "list"))
		require.Contains(t, stdout.String(), "user phil (role: user, tier: Pro)")

		app, _, stdout, _ = newTestApp()
		require.Nil(t, runUserCommand(app, conf, "change-tier", "phil", "-"))
		require.Contains(t, stdout.String(), "removed tier from user phil")

		app, _, _, _ = newTestApp()
		err := runUserCommand(app, conf, "change-tier", "phil", "nope")
		require.Error(t, err)
		require.Equal(t, "tier nope does not exist", err.Error())
	})
}

func TestCLI_User_Admin_ChangeAndDelete(t *testing.T) {
	forEachAdminMode(t, func(t *testing.T, socket bool) {
		s, conf, port := newTestServerWithAuth(t, socket)
		defer test.StopServer(t, s, port)

		app, stdin, _, _ := newTestApp()
		stdin.WriteString("mypass\nmypass\nnewpass\nnewpass")
		require.Nil(t, runUserCommand(app, conf, "add", "--role=admin", "phil"))
		require.Nil(t, runUserCommand(app, conf, "change-pass", "phil"))
		require.Nil(t, runUserCommand(app, conf, "change-role", "phil", "user"))
		require.Nil(t, runUserCommand(app, conf, "change-role", "phil", "admin"))

		app, _, stdout, _ := newTestApp()
		require.Nil(t, runUserCommand(app, conf, "del", "phil"))
		require.Contains(t, stdout.String(), "user phil removed")
	})
}

func withBaseURL(conf *server.Config) {
	conf.BaseURL = "https://ntfy.example.com"
}

func withSMTP(conf *server.Config) {
	conf.SMTPSenderAddr = "localhost:25"
	conf.SMTPSenderFrom = "ntfy@example.com"
}

func newTestServerWithAuth(t *testing.T, socket bool, options ...func(conf *server.Config)) (s *server.Server, conf *server.Config, port int) {
	configFile := filepath.Join(t.TempDir(), "server-dummy.yml")
	require.Nil(t, os.WriteFile(configFile, []byte(""), 0600)) // Dummy config file to avoid lookup of real server.yml
	conf = server.NewConfig()
	conf.File = configFile
	conf.AuthFile = filepath.Join(t.TempDir(), "user.db")
	conf.AuthDefault = user.PermissionDenyAll
	conf.AuthAccessCacheEnabled = false
	conf.AdminSocket = "" // Never the real default path, a server on this machine may be listening there
	if socket {
		conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	}
	for _, option := range options {
		option(conf)
	}
	s, port = test.StartServerWithConfig(t, conf)
	if socket {
		waitForAdminSocket(t, conf.AdminSocket)
	}
	return
}

func runUserCommand(app *cli.App, conf *server.Config, args ...string) error {
	userArgs := []string{
		"ntfy",
		"--log-level=ERROR",
		"user",
		"--config=" + conf.File, // Dummy config file to avoid lookups of real file
		"--auth-file=" + conf.AuthFile,
		"--auth-default-access=" + conf.AuthDefault.String(),
		"--admin-socket=" + conf.AdminSocket,
	}
	return app.Run(append(userArgs, args...))
}

// shortTempDir returns a temp dir with a short path, since unix socket paths are limited to ~104 bytes
func shortTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "ntfy-cli")
	require.Nil(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func waitForAdminSocket(t *testing.T, socket string) {
	for i := 0; i < 100; i++ {
		if client.NewAdminSocket(socket).Ping() == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("admin socket %s did not come up", socket)
}
