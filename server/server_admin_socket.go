package server

import (
	"net/http"
	"net/netip"
	"slices"

	"heckel.io/ntfy/v2/user"
)

var (
	// adminSocketPaths are the paths served over the admin socket. All of them are admin-only
	// routes in handleInternal (ensureAdmin), which the socket's synthetic admin user passes.
	adminSocketPaths = []string{
		apiVersionPath,
		apiUsersPath,
		apiUsersAccessPath,
		apiUsersTokensPath,
		apiUsersPasswordResetPath,
		apiTiersPath,
	}

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

// handleAdminSocket serves one admin API request that arrived over the local socket. There is
// no authentication: whoever can open the socket (file mode 0660, owner ntfy) is an admin.
// The request runs as adminSocketUser, bypasses rate limiting and the ban feed, and carries
// contextAdminSocket so handlers can allow what the network API refuses.
func (s *Server) handleAdminSocket(w http.ResponseWriter, r *http.Request) {
	r = withContext(r, map[contextKey]any{contextAdminSocket: true})
	v := newVisitor(s.config, s.messageCache, s.userManager, netip.AddrFrom4([4]byte{127, 0, 0, 1}), adminSocketUser)
	if !slices.Contains(adminSocketPaths, r.URL.Path) {
		s.handleError(w, r, v, errHTTPNotFound) // Publishing, subscribing, the web app, etc. are not served here
		return
	}
	if err := s.handleInternal(w, r, v); err != nil {
		s.handleError(w, r, v, err)
	}
}
