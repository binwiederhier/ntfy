package cluster

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
)

// ServeHTTP serves the internal peer API. Auth lives in the authenticated middleware, so every
// endpoint gets the same shared-secret and origin handling.
func (c *meshCluster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}

// authenticated wraps a peer API handler with the checks every endpoint needs: the shared
// secret (constant-time compare, rejected before any body is read), a present origin, and the
// origin self-skip (a request carrying this node's own traffic is acknowledged but ignored).
func (c *meshCluster) authenticated(h func(origin NodeID, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		origin := NodeID(r.Header.Get(originHeader))
		if origin == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if origin == c.conf.NodeID {
			w.WriteHeader(http.StatusOK) // Our own traffic; nothing to do
			return
		}
		h(origin, w, r)
	}
}

// secretAuthenticated wraps a handler with the shared-secret check only. Unlike authenticated
// it expects no origin node, because the callers are the load balancers' agents, not peers.
func (c *meshCluster) secretAuthenticated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// handleMessage receives a batch of peer messages (NDJSON) and streams them to local
// subscribers line by line, delivering each message as it is decoded.
func (c *meshCluster) handleMessage(origin NodeID, w http.ResponseWriter, r *http.Request) {
	// A batch can exceed its byte cap by one message, plus framing overhead
	maxBodyBytes := int64(batchMaxBytes) + c.conf.MaxMessageBytes + 1024
	received := 0
	deliver := func(m *model.Message) {
		received++
		if ev := log.Tag(tag); ev.IsTrace() {
			ev.Trace("Delivering message %s (topic %s) from peer %s", m.ID, m.Topic, origin)
		}
		c.deliver(m)
	}
	if err := decodeMessageBody(io.LimitReader(r.Body, maxBodyBytes), int(c.conf.MaxMessageBytes), deliver); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Tag(tag).Debug("Received batch of %d message(s) from peer %s", received, origin)
	w.WriteHeader(http.StatusOK)
}

// handleState receives a peer's state envelope and applies each section it carries.
func (c *meshCluster) handleState(origin NodeID, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, stateMaxBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var state apiState
	if err := json.Unmarshal(body, &state); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if state.Topics != nil && len(state.Topics.Added) > 0 && c.conf.TopicsAddedFunc != nil {
		log.Tag(tag).Debug("Received %d announced topic(s) from peer %s", len(state.Topics.Added), origin)
		c.conf.TopicsAddedFunc(state.Topics.Added)
	}
	if len(state.Cancels) > 0 && c.conf.CancelFunc != nil {
		log.Tag(tag).Debug("Received %d subscriber cancel(s) from peer %s", len(state.Cancels), origin)
		for _, cancel := range state.Cancels {
			c.conf.CancelFunc(cancel)
		}
	}
	w.WriteHeader(http.StatusOK)
}

// handleMembers lists the live cluster members for the load balancers' agents
func (c *meshCluster) handleMembers(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", contentTypeJSON)
	if err := json.NewEncoder(w).Encode(c.Members()); err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write member list")
	}
}

// handleHealth answers a peer's health probe: whether this node's registration is fresh enough
// that peers still forward to it (see maybeIsolated for what a peer does with the answer).
func (c *meshCluster) handleHealth(w http.ResponseWriter, _ *http.Request) {
	healthy := c.Healthy()
	w.Header().Set("Content-Type", contentTypeJSON)
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(&apiHealth{Healthy: healthy}); err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write health response")
	}
}
