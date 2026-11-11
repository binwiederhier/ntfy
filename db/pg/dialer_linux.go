package pg

import (
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// newDialer returns the dialer for database connections. It sets TCP_USER_TIMEOUT, so a
// connection whose query is never acknowledged (the node went away in a resize or failover)
// fails after tcpUserTimeout instead of after the kernel's ~15 minutes of retransmissions.
// It only bounds unacknowledged data, not a slow query on a healthy connection.
func newDialer() *net.Dialer {
	return &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			if !strings.HasPrefix(network, "tcp") {
				return nil // Unix sockets (host=/var/run/postgresql) have no TCP_USER_TIMEOUT
			}
			var sockErr error
			err := c.Control(func(fd uintptr) {
				sockErr = unix.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, int(tcpUserTimeout.Milliseconds()))
			})
			if err != nil {
				return err
			}
			return sockErr
		},
	}
}
