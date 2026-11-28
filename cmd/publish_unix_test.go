//go:build darwin || linux || dragonfly || freebsd || netbsd || openbsd

package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessExists_OtherUser(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can signal every process")
	}
	require.True(t, processExists(1)) // PID 1 is owned by root, so kill(1, 0) returns EPERM
}

func TestProcessExists_Exited(t *testing.T) {
	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())
	require.False(t, processExists(cmd.Process.Pid))
}
