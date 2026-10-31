//go:build !noserver

package cmd

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/test"
	"heckel.io/ntfy/v2/user"
)

func TestCLI_Backend_SocketDoesNotTouchDatabase(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, true)
	defer test.StopServer(t, s, port)

	// The CLI only needs to know that auth is configured; with a socket, it never opens the file
	app, _, stdout, stderr := newTestApp()
	confWithBadFile := *conf
	confWithBadFile.AuthFile = "/does/not/exist/user.db"
	require.Nil(t, runTierCommand(app, &confWithBadFile, "add", "--name=Pro", "pro"))
	require.Contains(t, stdout.String(), "tier added")
	require.NotContains(t, stderr.String(), "warning")
}

func TestCLI_Backend_FallbackWarning(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, false)
	defer test.StopServer(t, s, port)

	// Missing socket file
	app, _, stdout, stderr := newTestApp()
	socket := filepath.Join(shortTempDir(t), "admin.sock")
	require.Nil(t, app.Run([]string{"ntfy", "tier", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--listen-admin-unix=" + socket, "add", "--name=Pro", "pro"}))
	require.Contains(t, stdout.String(), "tier added")
	require.Contains(t, stderr.String(), "DEPRECATION WARNING: No ntfy server answered on the admin socket\n"+socket)
	require.NotContains(t, stderr.String(), "\033[") // Not a terminal, no colors

	// Stale socket file, nobody listening
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	app, _, _, stderr = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "tier", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--listen-admin-unix=" + socket, "list"}))
	require.Contains(t, stderr.String(), "DEPRECATION WARNING: No ntfy server answered on the admin socket\n"+socket)
	require.NotContains(t, stderr.String(), "\033[") // Not a terminal, no colors
}

func TestCLI_Backend_SocketDisabled(t *testing.T) {
	s, conf, port := newTestServerWithAuth(t, false)
	defer test.StopServer(t, s, port)

	// "-" and "" disable the socket, like in the server: no socket named "-", straight to the
	// database, which is deprecated just the same
	for _, socket := range []string{"-", ""} {
		app, _, _, stderr := newTestApp()
		require.Nil(t, app.Run([]string{"ntfy", "tier", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--listen-admin-unix=" + socket, "list"}))
		require.Contains(t, stderr.String(), "DEPRECATION WARNING: The admin socket is disabled\n(listen-admin-unix is \"-\" or empty), writing to the database")
	}
}

func TestCLI_Backend_FallbackWarningBanner(t *testing.T) {
	plain := adminFallbackWarning("No ntfy server answered on the admin socket\n/run/ntfy/admin.sock (no such file or directory)", false)
	require.Equal(t, `**********************************************************************
DEPRECATION WARNING: No ntfy server answered on the admin socket
/run/ntfy/admin.sock (no such file or directory), writing to the database
directly. A running server may not see this change right away.

Direct database access will be removed after 2026-12-30. After that, the
ntfy user/access/token/tier commands need a running server.
See https://docs.ntfy.sh/deprecations/ for details.
**********************************************************************
`, plain)
	yellow := adminFallbackWarning("No ntfy server answered on the admin socket\n/run/ntfy/admin.sock (no such file or directory)", true)
	require.Equal(t, "\033[1;33m"+strings.TrimSuffix(plain, "\n")+"\033[0m\n", yellow)
}

func TestCLI_Backend_SocketErrorNoFallback(t *testing.T) {
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
	err = app.Run([]string{"ntfy", "tier", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--listen-admin-unix=" + socket, "list"})
	require.Error(t, err)
	require.Empty(t, stdout.String())
}

func TestCLI_Backend_Server(t *testing.T) {
	s, port, remote := newTestServerWithAdminToken(t)
	defer test.StopServer(t, s, port)

	// Add and list over the network
	out, err := remote.run("tier", "add", "--name=Pro", "pro")
	require.Nil(t, err)
	require.Contains(t, out, "tier added")
	out, err = remote.run("tier", "list")
	require.Nil(t, err)
	require.Contains(t, out, "tier pro (id: ti_")

	// Flags after the arguments would be ignored, i.e. the command would silently go to the local
	// server (here: the database) instead of --server
	app, _, _, _ := newTestApp()
	err = app.Run([]string{"ntfy", "tier", "--config=" + remote.conf.File, "--auth-file=" + remote.conf.AuthFile, "change", "--name=Changed", "pro", "--server=" + remote.url, "--token=" + remote.token})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must come before")
	out, err = remote.run("tier", "list")
	require.Nil(t, err)
	require.Contains(t, out, "- Name: Pro")

	// The token goes over plain HTTP: warn, unless it stays on this machine
	app, _, _, stderr := newTestApp()
	_ = app.Run([]string{"ntfy", "tier", "--config=" + remote.conf.File, "--server=http://ntfy.invalid:1", "--token=" + remote.token, "list"})
	require.Contains(t, stderr.String(), "warning: --server http://ntfy.invalid:1 is not HTTPS, the token is sent unencrypted")
	app, _, _, stderr = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "tier", "--config=" + remote.conf.File, "--server=" + remote.url, "--token=" + remote.token, "list"}))
	require.NotContains(t, stderr.String(), "not HTTPS")

	// --token is required, and must be an admin's
	app, _, _, _ = newTestApp()
	err = app.Run([]string{"ntfy", "tier", "--config=" + remote.conf.File, "--server=" + remote.url, "list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--token is required")
	_, err = remote.run("tier", "--token=tk_doesnotexistdoesnotexist12345", "list")
	require.Error(t, err)
}

func TestCLI_Backend_SocketPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can open any socket")
	}
	s, conf, port := newTestServerWithAuth(t, true)
	defer test.StopServer(t, s, port)
	require.Nil(t, os.Chmod(conf.ListenAdminUnix, 0))

	// Database not accessible either: the user is not allowed to manage this server, say how to fix it
	dir := filepath.Dir(conf.AuthFile)
	require.Nil(t, os.Chmod(dir, 0))
	app, _, _, stderr := newTestApp()
	err := runTierCommand(app, conf, "list")
	require.Nil(t, os.Chmod(dir, 0700))
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied on the admin socket")
	require.NotContains(t, stderr.String(), "warning")

	// Database accessible: the socket belongs to some other server (e.g. the system's), use the database
	app, _, stdout, stderr := newTestApp()
	require.Nil(t, runTierCommand(app, conf, "add", "--name=Pro", "pro"))
	require.Contains(t, stdout.String(), "tier added")
	require.Contains(t, stderr.String(), "(permission denied), writing to the database\ndirectly")
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

func newTestServerWithAuth(t *testing.T, socket bool, options ...func(conf *server.Config)) (s *server.Server, conf *server.Config, port int) {
	configFile := filepath.Join(t.TempDir(), "server-dummy.yml")
	require.Nil(t, os.WriteFile(configFile, []byte(""), 0600)) // Dummy config file to avoid lookup of real server.yml
	conf = server.NewConfig()
	conf.File = configFile
	conf.AuthFile = filepath.Join(t.TempDir(), "user.db")
	conf.AuthDefault = user.PermissionDenyAll
	conf.AuthAccessCacheEnabled = false
	conf.ListenAdminUnix = "" // Never the real default path, a server on this machine may be listening there
	if socket {
		conf.ListenAdminUnix = filepath.Join(shortTempDir(t), "admin.sock")
	}
	for _, option := range options {
		option(conf)
	}
	s, port = test.StartServerWithConfig(t, conf)
	if socket {
		waitForAdminSocket(t, conf.ListenAdminUnix)
	}
	return
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
		if newServerSocketClient(socket).Ping() == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("admin socket %s did not come up", socket)
}

// testRemote runs CLI commands against a test server over --server, with an admin's token
type testRemote struct {
	conf  *server.Config
	url   string
	token string
}

func (r *testRemote) run(command string, args ...string) (string, error) {
	app, _, stdout, _ := newTestApp()
	err := app.Run(append([]string{"ntfy", command, "--config=" + r.conf.File, "--server=" + r.url, "--token=" + r.token}, args...))
	return stdout.String(), err
}

// newTestServerWithAdminToken starts a test server without admin socket, with an admin user "phil"
// (whose token the remote uses) and a regular user "ben", both created directly in the database
func newTestServerWithAdminToken(t *testing.T) (s *server.Server, port int, remote *testRemote) {
	s, conf, port := newTestServerWithAuth(t, false)
	manager, err := user.NewSQLiteManager(conf.AuthFile, "", &user.Config{BcryptCost: user.DefaultUserPasswordBcryptCost})
	require.Nil(t, err)
	require.Nil(t, manager.AddUser("phil", "phil", user.RoleAdmin, false))
	require.Nil(t, manager.AddUser("ben", "ben", user.RoleUser, false))
	phil, err := manager.User("phil")
	require.Nil(t, err)
	token, err := manager.CreateToken(phil.ID, "", time.Unix(0, 0), netip.IPv4Unspecified(), false)
	require.Nil(t, err)
	require.Nil(t, manager.Close())
	remote = &testRemote{
		conf:  conf,
		url:   fmt.Sprintf("http://127.0.0.1:%d", port),
		token: token.Value,
	}
	return s, port, remote
}
