package cluster

import (
	"sync"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// apiFanoutMessage is one line of a fan-out request body (NDJSON: one message per line; a single
// message is just a one-line body). It carries the two fields that model.Message does not
// serialize to JSON (Sender and User), which are needed to reconstruct the visitor on the
// receiving node. The origin node travels in a request header, not in the body.
type apiFanoutMessage struct {
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
