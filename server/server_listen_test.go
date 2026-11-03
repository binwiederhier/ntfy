//go:build !windows

package server

import (
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListenTCP_NoKeepAliveProbes(t *testing.T) {
	// Go turns on TCP keep-alive probes every 15s on accepted connections by default; to an idle phone
	// that is a radio wake-up every 15s on top of ntfy's own keepalive event every 45s
	ln, err := listenTCP("127.0.0.1:0")
	require.Nil(t, err)
	defer ln.Close()
	client, err := net.Dial("tcp", ln.Addr().String())
	require.Nil(t, err)
	defer client.Close()
	conn, err := ln.Accept()
	require.Nil(t, err)
	defer conn.Close()
	raw, err := conn.(*net.TCPConn).SyscallConn()
	require.Nil(t, err)
	var keepAlive int
	var sockErr error
	require.Nil(t, raw.Control(func(fd uintptr) {
		keepAlive, sockErr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE)
	}))
	require.Nil(t, sockErr)
	require.Equal(t, 0, keepAlive, "accepted connections send TCP keep-alive probes")
}
