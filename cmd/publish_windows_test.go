package cmd

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestProcessExists_ExitedWithOpenHandle(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	require.NoError(t, cmd.Start())
	defer cmd.Wait()

	// Hold our own handle, like any other process may, and wait for the process to exit
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	require.NoError(t, err)
	defer windows.CloseHandle(h)
	_, err = windows.WaitForSingleObject(h, windows.INFINITE)
	require.NoError(t, err)

	require.False(t, processExists(cmd.Process.Pid))
}

func TestWaitForProcess_ReturnsAfterExit(t *testing.T) {
	sleep := exec.Command("ping", "-n", "2", "127.0.0.1") // Takes about 1 second
	require.NoError(t, sleep.Start())
	go sleep.Wait() // Must be called to release resources

	done := make(chan error, 1)
	go func() {
		_, err := waitForProcess(sleep.Process.Pid)
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("waitForProcess did not return after the process exited")
	}
}
