//go:build !windows

package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminSocket_FileModes(t *testing.T) {
	conf := newTestConfigWithAuthFile(t, "")
	conf.ListenAdminUnix = filepath.Join(shortTempDir(t), "sub", "admin.sock")
	s := newTestServer(t, conf)
	runTestServer(t, s)
	waitFor(t, func() bool { return adminSocketListening(s) })

	// A new directory gets 0750, the socket 0660
	stat, err := os.Stat(filepath.Dir(conf.ListenAdminUnix))
	require.Nil(t, err)
	require.Equal(t, adminSocketDirMode, stat.Mode().Perm())
	stat, err = os.Stat(conf.ListenAdminUnix)
	require.Nil(t, err)
	require.Equal(t, DefaultListenAdminUnixMode, stat.Mode().Perm())
}

func TestAdminSocket_UnwritableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := shortTempDir(t)
	require.Nil(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	conf := newTestConfigWithAuthFile(t, "")
	conf.ListenAdminUnix = filepath.Join(dir, "sub", "admin.sock")
	s := newTestServer(t, conf)
	output := captureLogUntil(t, "not available", func() {
		runTestServer(t, s)
	})
	require.Contains(t, output, "Admin socket")
	require.Contains(t, output, "directly, which is deprecated, see https://ntfy.sh/docs/deprecations/")
	require.False(t, adminSocketListening(s))
}

func TestAdminSocket_InsecureDirectoryWarning(t *testing.T) {
	for _, tt := range []struct {
		mode fs.FileMode
		warn bool
	}{
		{0750, false},
		{0777, true},                  // Anyone can replace the socket
		{0777 | fs.ModeSticky, false}, // Like /tmp: only the owner can remove or replace it
	} {
		dir := shortTempDir(t)
		require.Nil(t, os.Chmod(dir, tt.mode))
		conf := newTestConfigWithAuthFile(t, "")
		conf.ListenAdminUnix = filepath.Join(dir, "admin.sock")
		s := newTestServer(t, conf)
		output := captureLog(t, func() {
			runTestServer(t, s)
			waitFor(t, func() bool { return adminSocketListening(s) }) // Warned before it listens
		})
		if tt.warn {
			require.Contains(t, output, "Admin socket directory "+dir+" is writable by all users", tt.mode)
		} else {
			require.NotContains(t, output, "Admin socket directory", tt.mode)
		}
	}
}
