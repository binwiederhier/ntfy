package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

const (
	adminSocketDirMode     = fs.FileMode(0750)
	adminSocketDialTimeout = time.Second // Used to detect a stale socket file (nobody listening)
)

var (
	// adminSocketUser is the synthetic admin every socket request runs as. Its name is one that
	// user.AllowedUsername rejects, so it can never collide with a real account.
	adminSocketUser = &user.User{
		ID:      "u_admin_socket",
		Name:    "(admin socket)",
		Role:    user.RoleAdmin,
		Stats:   &user.Stats{},   // Read by newVisitor
		Billing: &user.Billing{}, // Read by the visitor's log context
	}
)

// peerCredentials identifies the process on the other end of a unix socket connection, for the
// audit log. Zero values mean "unknown" (other OS, or the lookup failed).
type peerCredentials struct {
	UID int
	GID int
	PID int
}

// DefaultAdminSocket returns the default admin socket path. On Windows there is none: unix socket
// file permissions do not protect it there, so the CLI uses the database or --server instead.
func DefaultAdminSocket() string {
	return defaultAdminSocketFor(runtime.GOOS)
}

func defaultAdminSocketFor(goos string) string {
	switch goos {
	case "windows":
		return ""
	case "linux":
		return "/run/ntfy/admin.sock"
	}
	return "/var/run/ntfy/admin.sock" // macOS and the BSDs have no /run
}

// startAdminSocket listens on the admin socket, if configured, and serves the admin API on it.
// Failing to listen is not fatal: the CLI falls back to the database, so it only logs a warning.
func (s *Server) startAdminSocket() {
	if s.config.AdminSocket == "" || s.userManager == nil {
		return // Without auth, there is nothing to administer
	}
	listener, err := s.listenAdminSocket()
	if err != nil {
		log.Tag(tagStartup).Err(err).Warn("Admin socket %s not available, 'ntfy user/access/token/tier' will write to the database directly", s.config.AdminSocket)
		return
	}
	log.Tag(tagStartup).Info("Admin socket listening on %s", s.config.AdminSocket)
	s.adminSocketListener = listener
	s.adminSocketServer = &http.Server{
		Handler:     http.HandlerFunc(s.handleAdminSocket),
		ConnContext: adminSocketConnContext,
	}
	go func(server *http.Server) {
		// Not fed into Run's error channel: losing the socket must not take the server down
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Tag(tagStartup).Err(err).Warn("Admin socket %s stopped", s.config.AdminSocket)
		}
	}(s.adminSocketServer)
}

// listenAdminSocket creates the socket's directory, removes a stale socket file (one with no
// server behind it) and listens. Anything at the path that is not a socket is left alone, in case
// admin-socket points somewhere it should not (e.g. at the database). A socket that answers belongs to another running server; in
// that case this one does not take it over.
func (s *Server) listenAdminSocket() (net.Listener, error) {
	path := s.config.AdminSocket
	if err := os.MkdirAll(filepath.Dir(path), adminSocketDirMode); err != nil {
		return nil, err
	}
	if conn, err := net.DialTimeout("unix", path, adminSocketDialTimeout); err == nil {
		conn.Close()
		return nil, fmt.Errorf("another server is listening on %s", path)
	} else if errors.Is(err, fs.ErrPermission) {
		return nil, fmt.Errorf("%s belongs to a server of another user, set admin-socket to a path of your own", path)
	}
	if stat, err := os.Lstat(path); err == nil {
		if stat.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket, refusing to replace it", path)
		} else if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, DefaultAdminSocketMode); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// handleAdminSocket serves one admin API request that arrived over the local socket. There is
// no authentication: whoever can open the socket (file mode 0660, owner ntfy) is an admin.
// The request runs as adminSocketUser, bypasses rate limiting and the ban feed, and carries
// contextAdminSocket so handlers can allow what the network API refuses.
func (s *Server) handleAdminSocket(w http.ResponseWriter, r *http.Request) {
	r = withContext(r, map[contextKey]any{contextAdminSocket: true})
	v := newVisitor(s.config, s.messageCache, s.userManager, netip.AddrFrom4([4]byte{127, 0, 0, 1}), adminSocketUser)
	adminLog(v, r).Debug("Admin socket request")
	h := s.adminHandler(r, true)
	if h == nil || s.userManager == nil {
		s.handleError(w, r, v, errHTTPNotFound) // No auth configured means nothing to administer, same as ensureUserManager
		return
	}
	if err := h(w, r, v); err != nil {
		s.handleError(w, r, v, err)
	}
}

// adminSocketConnContext is the http.Server ConnContext for the admin socket: it reads the
// peer's credentials once per connection and stores them for the handlers' audit log.
func adminSocketConnContext(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, contextAdminSocketPeer, readPeerCredentials(conn))
}
