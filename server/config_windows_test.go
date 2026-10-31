//go:build windows

package server_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/server"
)

func TestConfig_DefaultListenAdminUnix(t *testing.T) {
	t.Setenv("ProgramData", `D:\Data`)
	require.Equal(t, `D:\Data\ntfy\admin.sock`, server.DefaultListenAdminUnix())
	t.Setenv("ProgramData", "")
	require.Equal(t, `C:\ProgramData\ntfy\admin.sock`, server.DefaultListenAdminUnix())
}
