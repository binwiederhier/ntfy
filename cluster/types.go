package cluster

import (
	"sync"

	"heckel.io/ntfy/v2/model"
)

// envelopeKindMessage identifies a fan-out envelope carrying a published message. Receivers
// ignore envelopes with unknown kinds (with a 200 response), so future envelope kinds can be
// introduced without breaking mixed-version clusters during rolling deploys.
const envelopeKindMessage = "message"

// envelope is the wire format for node-to-node fan-out. It carries the origin node ID (so a node
// can skip its own broadcasts) plus the two fields that model.Message does not serialize to JSON
// (Sender and User), which are needed to reconstruct the visitor on the receiving node.
type envelope struct {
	Kind    string         `json:"kind"`
	Origin  string         `json:"origin"`
	Sender  string         `json:"sender,omitempty"`
	User    string         `json:"user,omitempty"`
	Message *model.Message `json:"message,omitempty"`
}

// peer is a live remote node as read from the registry.
type peer struct {
	nodeID       string
	advertiseURL string
}

// peerQueue is the bounded send queue and target URL for a single peer. The URL may be updated
// when a peer re-registers under a different advertise URL.
type peerQueue struct {
	mu  sync.Mutex
	url string
	ch  chan []byte
}

func (q *peerQueue) FanoutURL() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.url
}
