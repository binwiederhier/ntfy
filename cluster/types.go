package cluster

import (
	"time"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// apiMessage is one line of a message request body (NDJSON: one message per line; a single
// message is just a one-line body). It carries the two fields that model.Message does not
// serialize to JSON (Sender and User), which are needed to reconstruct the visitor on the
// receiving node. The origin node travels in a request header, not in the body.
type apiMessage struct {
	Sender  string         `json:"sender,omitempty"`
	User    string         `json:"user,omitempty"`
	Message *model.Message `json:"message"`
}

// apiState is the peer state-exchange envelope. Each concern is an optional section; future
// concerns (rate limit counters, stats) become siblings of Topics.
type apiState struct {
	Topics *apiStateTopics `json:"topics,omitempty"`
}

// apiStateTopics carries a peer's subscription knowledge: either a full snapshot (Filter, a
// marshaled Bloom filter over the topics with live subscribers) replacing all prior knowledge,
// or an incremental update (Added) merged into it.
type apiStateTopics struct {
	Filter []byte   `json:"filter,omitempty"`
	Added  []string `json:"added,omitempty"`
}

// peerState is what a peer last told us about itself; Relay routes around peers whose
// fresh state provably excludes a topic.
type peerState struct {
	topics    *util.BloomFilter
	updatedAt time.Time
}

// peerQueue is the bounded, batching send queue for a single peer, pinned to the advertise URL
// the peer was created with: a peer re-registering under a different advertise URL is treated
// as a replacement (reconcile retires the old queue; Relay creates a fresh one on demand).
type peerQueue struct {
	advertiseURL string
	queue        *util.LingerQueue[[]byte] // pre-marshaled apiMessage fragments
}
