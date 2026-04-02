//go:build nowebpush

package server

import (
	"net/http"

	"heckel.io/ntfy/v2/model"
)

const (
	// WebPushAvailable is a constant used to indicate that WebPush support is available.
	// It can be disabled with the 'nowebpush' build tag.
	WebPushAvailable = false
)

func (s *Server) handleWebPushUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	return errHTTPNotFound
}

func (s *Server) handleWebPushDelete(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	return errHTTPNotFound
}

func (s *Server) publishToWebPushEndpoints(v *visitor, m *model.Message) {
	// Nothing to see here
}

func (s *Server) pruneAndNotifyWebPushSubscriptions() {
	// Nothing to see here
}
