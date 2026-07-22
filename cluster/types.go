package cluster

import (
	"sync"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// envelopeKindBatch identifies a fan-out request carrying a batch of published messages (a batch
// of one is the degenerate case; there is no single-message format). Receivers ignore requests
// with unknown kinds (with a 200 response), so future envelope kinds can be introduced without
// breaking mixed-version clusters during rolling deploys.
const envelopeKindBatch = "batch"

// apiBatch is the wire format for node-to-node fan-out: a batch of messages from one origin node
// (so a node can skip its own broadcasts).
type apiBatch struct {
	Kind     string             `json:"kind"`
	Origin   string             `json:"origin"`
	Messages []*apiBatchMessage `json:"messages,omitempty"`
}

// apiBatchMessage is one message in a fan-out batch. It carries the two fields that model.Message
// does not serialize to JSON (Sender and User), which are needed to reconstruct the visitor on
// the receiving node.
type apiBatchMessage struct {
	Sender  string         `json:"sender,omitempty"`
	User    string         `json:"user,omitempty"`
	Message *model.Message `json:"message"`
}

// peer is a live remote node as read from the registry.
type peer struct {
	nodeID       string
	advertiseURL string
}

// peerQueue is the bounded, batching send queue and target URL for a single peer. The URL may be
// updated when a peer re-registers under a different advertise URL.
type peerQueue struct {
	mu    sync.Mutex
	url   string
	queue *util.LingerQueue[[]byte] // pre-marshaled apiBatchMessage fragments
}

func (q *peerQueue) FanoutURL() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.url
}
