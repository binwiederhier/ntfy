package cluster

import (
	"time"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// Config configures the cluster. It is assembled by the server from its own config, which keeps
// this package free of server types.
type Config struct {
	Enabled             bool          // Master switch; when false, New returns the nop cluster
	NodeID              NodeID        // Stable per-node identifier; required
	AdvertiseURL        string        // Base URL peers use to reach this node's fan-out endpoint
	Secret              string        // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval   time.Duration // How often the node registry heartbeat is refreshed
	NodeTTL             time.Duration // Registry rows older than this do not count as live peers
	BatchLinger         time.Duration // How long messages wait in a peer queue to form a batch; 0 = send immediately
	StateInterval       time.Duration // How often the full subscription state is pushed to peers
	MaxMessageBytes     int64         // Upper bound for a single message on the wire (batch limits derive from this)
	LeaderRenewInterval time.Duration // Overrides the leader lease renewal cadence; tests only, 0 = default
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// State is a subscription-state delta for Cluster.BroadcastState.
type State struct {
	AddedTopics []string // Topics that just gained their first local subscriber on this node
}

// TopicsFunc returns the topics that currently have at least one live subscriber, computed
// fresh on every call: membership is never tracked as a list, so topics "leave" simply by not
// appearing in the next snapshot. The server supplies it (same inversion as DeliverFunc).
type TopicsFunc func() []string

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

// peerState is what a peer last told us about itself; ForwardMessage routes around peers whose
// fresh state provably excludes a topic.
type peerState struct {
	topics    *util.BloomFilter
	updatedAt time.Time
}

// peerQueue is the bounded, batching send queue for a single peer, pinned to the advertise URL
// the peer was created with: a peer re-registering under a different advertise URL is treated
// as a replacement (reconcile retires the old queue; ForwardMessage creates a fresh one on demand).
type peerQueue struct {
	advertiseURL string
	queue        *util.LingerQueue[[]byte] // pre-marshaled apiMessage fragments
}
