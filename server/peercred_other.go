//go:build !linux && !darwin && !freebsd

package server

import (
	"net"
)

// readPeerCredentials is not implemented on this OS; the audit log shows zero credentials
func readPeerCredentials(conn net.Conn) peerCredentials {
	return peerCredentials{}
}
