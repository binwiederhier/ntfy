package pg

import (
	"context"
	"net"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestNewDialer_SetsTCPUserTimeout(t *testing.T) {
	// A query sent to a database node that went away (a resize or failover moved it) is never
	// acknowledged; without TCP_USER_TIMEOUT, the kernel retransmits for ~15 minutes before the
	// connection fails, and the request holds its pool slot all that time
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	defer listener.Close()
	conn, err := newDialer().DialContext(context.Background(), "tcp", listener.Addr().String())
	require.Nil(t, err)
	defer conn.Close()
	raw, err := conn.(*net.TCPConn).SyscallConn()
	require.Nil(t, err)
	var timeout int
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		timeout, sockErr = unix.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, unix.TCP_USER_TIMEOUT)
	})
	require.Nil(t, err)
	require.Nil(t, sockErr)
	require.Equal(t, int(tcpUserTimeout.Milliseconds()), timeout)
}

func TestParseConnConfig_DialsWithTCPUserTimeout(t *testing.T) {
	// The pool's connections are opened with newDialer, so they get TCP_USER_TIMEOUT too
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.Nil(t, err)
	defer listener.Close()
	config, err := parseConnConfig("postgres://user:pass@" + listener.Addr().String() + "/db")
	require.Nil(t, err)
	conn, err := config.DialFunc(context.Background(), "tcp", listener.Addr().String())
	require.Nil(t, err)
	defer conn.Close()
	raw, err := conn.(*net.TCPConn).SyscallConn()
	require.Nil(t, err)
	var timeout int
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		timeout, sockErr = unix.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, unix.TCP_USER_TIMEOUT)
	})
	require.Nil(t, err)
	require.Nil(t, sockErr)
	require.Equal(t, int(tcpUserTimeout.Milliseconds()), timeout)
}

func TestParseConnConfig_DialsUnixSocket(t *testing.T) {
	// TCP_USER_TIMEOUT does not exist for Unix sockets (host=/var/run/postgresql), so the dialer
	// must not set it there, or every connection fails with "operation not supported"
	path := filepath.Join(t.TempDir(), ".s.PGSQL.5432")
	listener, err := net.Listen("unix", path)
	require.Nil(t, err)
	defer listener.Close()
	config, err := parseConnConfig("postgres://user:pass@/db?host=" + filepath.Dir(path))
	require.Nil(t, err)
	conn, err := config.DialFunc(context.Background(), "unix", path)
	require.Nil(t, err)
	require.Nil(t, conn.Close())
}
