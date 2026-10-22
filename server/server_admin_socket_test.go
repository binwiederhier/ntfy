package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/ban"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

func TestAdminSocket_DefaultPath(t *testing.T) {
	require.Equal(t, "/run/ntfy/admin.sock", defaultAdminSocketFor("linux"))
	require.Equal(t, "/var/run/ntfy/admin.sock", defaultAdminSocketFor("darwin"))
	require.Equal(t, "/var/run/ntfy/admin.sock", defaultAdminSocketFor("freebsd"))
	require.Equal(t, "/var/run/ntfy/admin.sock", defaultAdminSocketFor("openbsd"))
	require.Equal(t, "", defaultAdminSocketFor("windows"))
}

func TestAdminSocket_ListenAndRemove(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s, socket := newTestServerWithAdminSocket(t, newTestConfigWithAuthFile(t, databaseURL))

		// Socket exists with the right mode
		stat, err := os.Stat(socket)
		require.Nil(t, err)
		require.Equal(t, os.ModeSocket, stat.Mode().Type())
		require.Equal(t, DefaultAdminSocketMode, stat.Mode().Perm())

		// Removed on stop
		s.Stop()
		_, err = os.Stat(socket)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestAdminSocket_UsersWithoutCredentials(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s, socket := newTestServerWithAdminSocket(t, newTestConfigWithAuthFile(t, databaseURL))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))
		client := adminSocketClient(socket)

		// List users, no Authorization header
		resp, err := client.Get("http://ntfy/v1/users")
		require.Nil(t, err)
		require.Equal(t, 200, resp.StatusCode)
		var users []*apiUserResponse
		require.Nil(t, json.NewDecoder(resp.Body).Decode(&users))
		resp.Body.Close()
		require.Len(t, users, 2) // ben and everyone

		// Create an admin: role changes work end to end
		resp, err = client.Post("http://ntfy/v1/users", "application/json", strings.NewReader(`{"username":"phil","password":"phil","role":"admin"}`))
		require.Nil(t, err)
		require.Equal(t, 200, resp.StatusCode)
		resp.Body.Close()
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		require.Equal(t, user.RoleAdmin, u.Role)
	})
}

func TestAdminSocket_NonAdminRoutes(t *testing.T) {
	_, socket := newTestServerWithAdminSocket(t, newTestConfigWithAuthFile(t, ""))
	client := adminSocketClient(socket)
	resp, err := client.Get("http://ntfy/mytopic/json?poll=1")
	require.Nil(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	resp, err = client.Post("http://ntfy/mytopic", "text/plain", strings.NewReader("hi"))
	require.Nil(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
}

func TestAdminSocket_StaleSocketFile(t *testing.T) {
	conf := newTestConfigWithAuthFile(t, "")
	conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	stale, err := net.Listen("unix", conf.AdminSocket)
	require.Nil(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	stale.Close() // Socket file stays behind, nobody listening (e.g. after a crash)
	s := newTestServer(t, conf)
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Contains(t, output, "Admin socket listening on "+conf.AdminSocket)
	resp, err := adminSocketClient(conf.AdminSocket).Get("http://ntfy/v1/users")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
}

func TestAdminSocket_RegularFileNotRemoved(t *testing.T) {
	// A misconfigured path (e.g. pointing at the database) must never be deleted
	conf := newTestConfigWithAuthFile(t, "")
	conf.AdminSocket = filepath.Join(shortTempDir(t), "user.db")
	require.Nil(t, os.WriteFile(conf.AdminSocket, []byte("precious"), 0600))
	s := newTestServer(t, conf)
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Contains(t, output, "not a socket")
	require.Nil(t, s.adminSocketServer)
	data, err := os.ReadFile(conf.AdminSocket)
	require.Nil(t, err)
	require.Equal(t, "precious", string(data))
}

func TestAdminSocket_AnotherServerListening(t *testing.T) {
	conf := newTestConfigWithAuthFile(t, "")
	conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	other, err := net.Listen("unix", conf.AdminSocket)
	require.Nil(t, err)
	defer other.Close()
	s := newTestServer(t, conf)
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Contains(t, output, "another server is listening")
	require.Nil(t, s.adminSocketServer)

	// The other listener is untouched
	_, err = os.Stat(conf.AdminSocket)
	require.Nil(t, err)
}

func TestAdminSocket_OtherUsersSocket(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can open any socket")
	}
	conf := newTestConfigWithAuthFile(t, "")
	conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	other, err := net.Listen("unix", conf.AdminSocket)
	require.Nil(t, err)
	defer other.Close()
	require.Nil(t, os.Chmod(conf.AdminSocket, 0)) // As seen by a user who may not open it
	s := newTestServer(t, conf)
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Contains(t, output, "belongs to a server of another user")
	require.Nil(t, s.adminSocketServer)
}

func TestAdminSocket_UnwritableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := shortTempDir(t)
	require.Nil(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	conf := newTestConfigWithAuthFile(t, "")
	conf.AdminSocket = filepath.Join(dir, "sub", "admin.sock")
	s := newTestServer(t, conf)
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Contains(t, output, "Admin socket")
	require.Contains(t, output, "not available")
	require.NotContains(t, output, "listening")
	require.Nil(t, s.adminSocketServer)
}

func TestAdminSocket_DisabledWithoutPathOrAuth(t *testing.T) {
	// No path: no listener, no warning
	s := newTestServer(t, newTestConfigWithAuthFile(t, ""))
	output := captureLog(t, func() {
		s.startAdminSocket()
	})
	require.Nil(t, s.adminSocketServer)
	require.NotContains(t, output, "Admin socket")

	// No auth configured: nothing to administer, so no listener either
	conf := newTestConfig(t, "")
	conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	s = newTestServer(t, conf)
	s.startAdminSocket()
	require.Nil(t, s.adminSocketServer)
	_, err := os.Stat(conf.AdminSocket)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestAdminSocket_AuditLogPeerCredentials(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("peer credentials are only read on Linux")
	}
	_, socket := newTestServerWithAdminSocket(t, newTestConfigWithAuthFile(t, ""))
	output := captureLog(t, func() {
		resp, err := adminSocketClient(socket).Post("http://ntfy/v1/users", "application/json", strings.NewReader(`{"username":"ben","password":"ben"}`))
		require.Nil(t, err)
		require.Equal(t, 200, resp.StatusCode)
		resp.Body.Close()
	})
	require.Contains(t, output, "Adding user ben")
	require.Contains(t, output, "admin_via=socket")
	require.Contains(t, output, fmt.Sprintf("peer_uid=%d", os.Getuid()))
	require.Contains(t, output, fmt.Sprintf("peer_pid=%d", os.Getpid()))
}

func TestAdminSocket_NoBanFeed(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	conf := newTestConfigWithAuthFile(t, "")
	conf.BanFile = banFile
	conf.BanWindow = time.Minute
	conf.BanThreshold = 1
	conf.BanWeights = ban.Weights{"*": 1}
	s, socket := newTestServerWithAdminSocket(t, conf)
	require.NotNil(t, s.ban)
	client := adminSocketClient(socket)
	for i := 0; i < 3; i++ {
		resp, err := client.Get("http://ntfy/nope")
		require.Nil(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}

	// Writes are async; Close flushes. Nothing may have been recorded.
	s.ban.Close()
	data, err := os.ReadFile(banFile)
	if !errors.Is(err, os.ErrNotExist) {
		require.Nil(t, err)
		require.Empty(t, strings.TrimSpace(string(data)))
	}
}

// newTestServerWithAdminSocket starts a test server with the admin socket listening in a short
// temp dir (unix socket paths are limited to ~104 bytes, t.TempDir() can be longer on macOS)
func newTestServerWithAdminSocket(t *testing.T, conf *Config) (*Server, string) {
	conf.AdminSocket = filepath.Join(shortTempDir(t), "admin.sock")
	s := newTestServer(t, conf)
	s.startAdminSocket()
	require.NotNil(t, s.adminSocketServer)
	return s, conf.AdminSocket
}

// adminSocketClient returns an HTTP client that sends every request to the given unix socket
func adminSocketClient(socket string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
}

func shortTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "ntfy-admin")
	require.Nil(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// captureLog runs f and returns what was logged meanwhile (at info level or above)
func captureLog(t *testing.T, f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)
	defer func() {
		log.SetLevel(log.ErrorLevel)                  // As in TestMain
		log.SetOutput(struct{ io.Writer }{os.Stderr}) // Wrapped, so the log package does not treat stderr as a log file
	}()
	f()
	return buf.String()
}
