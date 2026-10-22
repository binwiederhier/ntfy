//go:build !noserver

package cmd

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/test"
	"heckel.io/ntfy/v2/user"
)

func TestCLI_AdminAPI_SocketDoesNotTouchDatabase(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, true)
	defer test.StopServer(t, s, port)

	// The CLI only needs to know that auth is configured; with a socket, it never opens the file
	app, stdin, stdout, stderr := newTestApp()
	stdin.WriteString("mypass\nmypass")
	confWithBadFile := *conf
	confWithBadFile.AuthFile = "/does/not/exist/user.db"
	require.Nil(t, runUserCommand(app, &confWithBadFile, "add", "phil"))
	require.Contains(t, stdout.String(), "user phil added with role user")
	require.NotContains(t, stderr.String(), "warning")
}

func TestCLI_AdminAPI_FallbackWarning(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, false)
	defer test.StopServer(t, s, port)

	// Missing socket file
	app, _, stdout, stderr := newTestApp()
	socket := filepath.Join(shortTempDir(t), "admin.sock")
	require.Nil(t, app.Run([]string{"ntfy", "user", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--admin-socket=" + socket, "list"}))
	require.Contains(t, stdout.String(), "user * (role: anonymous, tier: none)")
	require.Contains(t, stderr.String(), "warning: no ntfy server answered on the admin socket "+socket)

	// Stale socket file, nobody listening
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	app, _, _, stderr = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "user", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--admin-socket=" + socket, "list"}))
	require.Contains(t, stderr.String(), "warning: no ntfy server answered on the admin socket "+socket)
}

func TestCLI_AdminAPI_SocketErrorNoFallback(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, false)
	defer test.StopServer(t, s, port)

	// Something answers on the socket, but with an error that is not "not found": no fallback
	socket := filepath.Join(shortTempDir(t), "admin.sock")
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":40101,"http":401,"error":"unauthorized"}`))
	})}
	go httpServer.Serve(listener)
	defer httpServer.Close()

	app, _, stdout, _ := newTestApp()
	err = app.Run([]string{"ntfy", "user", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--admin-socket=" + socket, "list"})
	require.Error(t, err)
	require.Empty(t, stdout.String())
}

func TestCLI_AdminAPI_Server(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, false)
	defer test.StopServer(t, s, port)

	// Admin with a token, and a regular user, created directly
	manager, err := user.NewSQLiteManager(conf.AuthFile, "", &user.Config{BcryptCost: user.DefaultUserPasswordBcryptCost})
	require.Nil(t, err)
	require.Nil(t, manager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, manager.AddUser("ben", "ben", user.RoleUser, false))
	phil, err := manager.User("phil")
	require.Nil(t, err)
	token, err := manager.CreateToken(phil.ID, "", time.Unix(0, 0), netip.IPv4Unspecified(), false)
	require.Nil(t, err)
	require.Nil(t, manager.Close())
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	remote := func(command string, args ...string) (string, error) {
		app, _, stdout, _ := newTestApp()
		err := app.Run(append([]string{"ntfy", command, "--config=" + conf.File, "--server=" + serverURL, "--token=" + token.Value}, args...))
		return stdout.String(), err
	}

	// List, grant and show access
	out, err := remote("user", "list")
	require.Nil(t, err)
	require.Contains(t, out, "user ben (role: user, tier: none)")
	_, err = remote("access", "ben", "mytopic", "ro")
	require.Nil(t, err)
	out, err = remote("access", "ben")
	require.Nil(t, err)
	require.Contains(t, out, "- read-only access to topic mytopic")

	// Admins cannot be created or changed over the network
	app, stdin, _, _ := newTestApp()
	stdin.WriteString("mypass\nmypass")
	err = app.Run([]string{"ntfy", "user", "--config=" + conf.File, "--server=" + serverURL, "--token=" + token.Value, "add", "--role=admin", "emma"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "only possible over the admin socket")
	_, err = remote("user", "del", "phil")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only possible over the admin socket")

	// --token is required, and must be an admin's
	app, _, _, _ = newTestApp()
	err = app.Run([]string{"ntfy", "user", "--config=" + conf.File, "--server=" + serverURL, "list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--token is required")
	_, err = remote("user", "--token=tk_doesnotexistdoesnotexist12345", "list")
	require.Error(t, err)
}

func TestCLI_AdminAPI_SocketPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can open any socket")
	}
	s, conf, port := newTestServerWithAuth(t, true)
	defer test.StopServer(t, s, port)
	require.Nil(t, os.Chmod(conf.AdminSocket, 0))

	// Database not accessible either: the user is not allowed to manage this server, say how to fix it
	dir := filepath.Dir(conf.AuthFile)
	require.Nil(t, os.Chmod(dir, 0))
	app, _, _, stderr := newTestApp()
	err := runUserCommand(app, conf, "list")
	require.Nil(t, os.Chmod(dir, 0700))
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied on the admin socket")
	require.NotContains(t, stderr.String(), "warning")

	// Database accessible: the socket belongs to some other server (e.g. the system's), use the database
	app, _, stdout, stderr := newTestApp()
	require.Nil(t, runUserCommand(app, conf, "list"))
	require.Contains(t, stdout.String(), "user * (role: anonymous")
	require.Contains(t, stderr.String(), "(permission denied), writing to the database directly")
}
