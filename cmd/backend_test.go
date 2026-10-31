//go:build !noserver

package cmd

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cmd/client"
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
	require.Contains(t, stderr.String(), "DEPRECATION WARNING: No ntfy server answered on the admin socket\n"+socket+" (no such file or directory)")

	// Stale socket file, nobody listening
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	app, _, _, stderr = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "tier", "--config=" + conf.File, "--auth-file=" + conf.AuthFile, "--listen-admin-unix=" + socket, "list"}))
	require.Contains(t, stderr.String(), "DEPRECATION WARNING: No ntfy server answered on the admin socket\n"+socket+" (connection refused)")
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
	var buf bytes.Buffer
	printAdminFallbackWarning(&buf, "No ntfy server answered on the admin socket\n/run/ntfy/admin.sock (no such file or directory)")
	require.Equal(t, `**********************************************************************
DEPRECATION WARNING: No ntfy server answered on the admin socket
/run/ntfy/admin.sock (no such file or directory), writing to the database
directly. A running server may not see this change right away.

Direct database access will be removed after 2026-12-30. After that, the
ntfy user/access/token/tier commands need a running server.
See https://docs.ntfy.sh/deprecations/ for details.
**********************************************************************
`, buf.String())
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
		if client.New(socket).Ping() == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("admin socket %s did not come up", socket)
}
