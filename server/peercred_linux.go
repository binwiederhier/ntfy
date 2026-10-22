package server

import (
	"net"

	"golang.org/x/sys/unix"
)

// readPeerCredentials returns the uid, gid and pid of the process connected to the unix socket.
// Errors are swallowed: the credentials are for the audit log, the file mode is the authorization.
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
		ucred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err == nil {
			creds = peerCredentials{
				UID: int(ucred.Uid),
				GID: int(ucred.Gid),
				PID: int(ucred.Pid),
			}
		}
	})
	return creds
}
