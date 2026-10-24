//go:build darwin || freebsd

package server

import (
	"net"

	"golang.org/x/sys/unix"
)

// readPeerCredentials returns the uid and primary gid of the process connected to the unix
// socket (no pid: FreeBSD has no portable way to get it). Errors are swallowed, see peercred_linux.go.
func readPeerCredentials(conn net.Conn) peerCredentials {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return peerCredentials{}
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return peerCredentials{}
	}
	var creds peerCredentials
	raw.Control(func(fd uintptr) {
		xucred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			return
		}
		creds.UID = int(xucred.Uid)
		if xucred.Ngroups > 0 {
			creds.GID = int(xucred.Groups[0])
		}
	})
	return creds
}
