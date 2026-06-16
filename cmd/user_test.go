package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/test"
	"heckel.io/ntfy/v2/user"
)

func TestCLI_User_Add(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
	defer test.StopServer(t, s, port)

	app, stdin, stdout, _ := newTestApp()
	stdin.WriteString("mypass\nmypass")
	require.Nil(t, runUserCommand(app, conf, "add", "phil"))
	require.Contains(t, stdout.String(), "user phil added with role user")
}

func TestCLI_User_Add_Exists(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
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
}

func TestCLI_User_Add_Admin(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
	defer test.StopServer(t, s, port)

	app, stdin, stdout, _ := newTestApp()
	stdin.WriteString("mypass\nmypass")
	require.Nil(t, runUserCommand(app, conf, "add", "--role=admin", "phil"))
	require.Contains(t, stdout.String(), "user phil added with role admin")
}

func TestCLI_User_Add_Password_Mismatch(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
	defer test.StopServer(t, s, port)

	app, stdin, _, _ := newTestApp()
	stdin.WriteString("mypass\nNOTMATCH")
	err := runUserCommand(app, conf, "add", "phil")
	require.Error(t, err)
	require.Contains(t, err.Error(), "passwords do not match: try it again, but this time type slooowwwlly")
}

func TestCLI_User_ChangePass(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
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
}

func TestCLI_User_ChangeRole(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
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
}

func TestCLI_User_Delete(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t)
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
}

func newTestServerWithAuth(t *testing.T) (s *server.Server, conf *server.Config, port int) {
	configFile := filepath.Join(t.TempDir(), "server-dummy.yml")
	require.Nil(t, os.WriteFile(configFile, []byte(""), 0600)) // Dummy config file to avoid lookup of real server.yml
	conf = server.NewConfig()
	conf.File = configFile
	conf.AuthFile = filepath.Join(t.TempDir(), "user.db")
	conf.AuthDefault = user.PermissionDenyAll
	conf.AuthAccessCacheEnabled = false
	s, port = test.StartServerWithConfig(t, conf)
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
	}
	return app.Run(append(userArgs, args...))
}
