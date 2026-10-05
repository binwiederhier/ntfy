package cluster

import (
	"time"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// Config configures the cluster. It is assembled by the server from its own config, which keeps
// this package free of server types.
type Config struct {
	Enabled           bool            // Master switch; when false, New returns the nop cluster
	NodeID            NodeID          // Stable per-node identifier; required
	AdvertiseURL      string          // Base URL peers use to reach this node's fan-out endpoint
	Secret            string          // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration   // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration   // Registry rows older than this do not count as live peers
	BatchLinger       time.Duration   // How long messages wait in a peer queue to form a batch; 0 = send immediately
	MaxMessageBytes   int64           // Upper bound for a single message on the wire (batch limits derive from this)
	CancelFunc        CancelFunc      // Applies a peer's subscriber-cancel request to local connections; may be nil
	TopicsAddedFunc   TopicsAddedFunc // Told about topics that just gained their first subscriber on a peer; may be nil
	GapFunc           GapFunc         // Told that a peer could not deliver messages for these topics; may be nil
	IsolatedFunc      func()          // Called while this node lost its registration but a peer is healthy; may be nil
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// State is a state delta for Cluster.BroadcastState.
type State struct {
	AddedTopics       []string            // Topics that just gained their first local subscriber on this node
	SubscriberCancels []*SubscriberCancel // Requests to cancel matching live subscriber connections on peer nodes
}

// SubscriberCancel asks peer nodes to cancel matching live subscriber connections, mirroring
// the two local operations that reach into open connections: reservation takeover (Topic +
// ExceptUserID: cancel everyone else on that one topic) and access revocation (Topic pattern +
// UserID: cancel that user wherever the pattern matches). Without relaying these, they only
// affect the node they ran on and revoked subscribers keep receiving on other nodes.
type SubscriberCancel struct {
	Topic        string `json:"topic"`                    // Topic ID, or a "*" pattern when UserID is set
	UserID       string `json:"user_id,omitempty"`        // Cancel this user's subscribers (access revocation)
	ExceptUserID string `json:"except_user_id,omitempty"` // Cancel everyone BUT this user (reservation takeover)
}

// CancelFunc applies a peer's subscriber-cancel request to this node's local connections. The
// server supplies it (same inversion as DeliverFunc); it must only cancel locally, never
// re-broadcast (loop prevention).
type CancelFunc func(cancel *SubscriberCancel)

// GapFunc is called with topics for which a peer could not deliver one or more messages to this
// node (its queue overflowed, or the request failed). The server closes those topics' local
// subscribers so their clients reconnect and replay the gap from the message cache with
// since=. GapAllTopics means "every topic": too many topics to enumerate.
type GapFunc func(topics []string)

// GapAllTopics is the GapFunc marker for "gaps in so many topics that they are not worth
// enumerating; treat every local subscriber as having missed something".
const GapAllTopics = "*"

// TopicsAddedFunc is called with topics a peer announced as having just gained their first
// subscriber there. The server supplies it to drop cached negative rate-visitor lookups for
// those topics; it must not re-broadcast (loop prevention). It is a cache hint only: a lost
// announcement costs a stale lookup until its TTL, never a lost message.
type TopicsAddedFunc func(topics []string)

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
	Topics  *apiStateTopics     `json:"topics,omitempty"`
	Cancels []*SubscriberCancel `json:"cancels,omitempty"`
	Gaps    []string            `json:"gaps,omitempty"`
}

// apiStateTopics carries topics that just gained their first subscriber on the sending node.
type apiStateTopics struct {
	Added []string `json:"added,omitempty"`
}

// peerQueue is the bounded, batching send queue for a single peer, pinned to the advertise URL
// the peer was created with: a peer re-registering under a different advertise URL is treated
// as a replacement (reconcile retires the old queue; ForwardMessage creates a fresh one on demand).
type peerQueue struct {
	advertiseURL string
	queue        *util.LingerQueue[*fragment]
}

// fragment is one pre-marshaled apiMessage line plus the topic it belongs to: a batch that is
// dropped or rejected turns into a delivery gap, which is reported per topic (see GapFunc).
type fragment struct {
	topic string
	data  []byte
}
