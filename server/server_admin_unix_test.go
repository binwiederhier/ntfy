//go:build !windows

package server

import (
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
	require.NotNil(t, s.adminSocketServer)

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
	output := captureLog(t, func() {
		runTestServer(t, s)
	})
	require.Contains(t, output, "Admin socket")
	require.Contains(t, output, "not available")
	require.Nil(t, s.adminSocketServer)
}
