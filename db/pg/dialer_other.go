//go:build !linux

package pg

import (
	"net"
)

// newDialer returns the dialer for database connections. TCP_USER_TIMEOUT is Linux only, so
// elsewhere a connection to a vanished node fails only after the kernel's retransmissions.
func newDialer() *net.Dialer {
	return &net.Dialer{}
}
