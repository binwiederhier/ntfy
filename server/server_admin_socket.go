package server

import (
	"net/http"
	"net/netip"

	"heckel.io/ntfy/v2/user"
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

// handleAdminSocket serves one admin API request that arrived over the local socket. There is
// no authentication: whoever can open the socket (file mode 0660, owner ntfy) is an admin.
// The request runs as adminSocketUser, bypasses rate limiting and the ban feed, and carries
// contextAdminSocket so handlers can allow what the network API refuses.
func (s *Server) handleAdminSocket(w http.ResponseWriter, r *http.Request) {
	r = withContext(r, map[contextKey]any{contextAdminSocket: true})
	v := newVisitor(s.config, s.messageCache, s.userManager, netip.AddrFrom4([4]byte{127, 0, 0, 1}), adminSocketUser)
	h := s.adminHandler(r, true)
	if h == nil || s.userManager == nil {
		s.handleError(w, r, v, errHTTPNotFound) // No auth configured means nothing to administer, same as ensureUserManager
		return
	}
	if err := h(w, r, v); err != nil {
		s.handleError(w, r, v, err)
	}
}
