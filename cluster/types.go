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
	Role              Role            // Active (default) or passive: a passive node never leads and never fences
	Secret            string          // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration   // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration   // Registry rows older than this do not count as live peers
	BatchLinger       time.Duration   // How long messages wait in a peer queue to form a batch; 0 = send immediately
	MaxMessageBytes   int64           // Upper bound for a single message on the wire (batch limits derive from this)
	CancelFunc        CancelFunc      // Applies a peer's subscriber-cancel request to local connections; may be nil
	TopicsAddedFunc   TopicsAddedFunc // Told about topics that just gained their first subscriber on a peer; may be nil
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

// TopicsAddedFunc is called with topics a peer announced as having just gained their first
// subscriber there. The server supplies it to drop cached negative rate-visitor lookups for
// those topics; it must not re-broadcast (loop prevention). It is a cache hint only: a lost
// announcement costs a stale lookup until its TTL, never a lost message.
type TopicsAddedFunc func(topics []string)

// Status is a node's view of the cluster, served on StatusPath. Times are unix seconds, 0 for never.
type Status struct {
	NodeID  NodeID        `json:"node_id"`
	Role    Role          `json:"role"`
	Leader  bool          `json:"leader"`
	Healthy bool          `json:"healthy"`
	Time    int64         `json:"time"` // When this view was taken, to judge the other timestamps by
	Peers   []*PeerStatus `json:"peers"`
}

// PeerStatus is one peer as seen from this node: every live peer, plus any peer this node has
// exchanged batches with since it started (Live false once it left the registry)
type PeerStatus struct {
	NodeID       NodeID       `json:"node_id"`
	AdvertiseURL string       `json:"advertise_url,omitempty"`
	Role         Role         `json:"role,omitempty"`
	Live         bool         `json:"live"`
	HeartbeatAge int64        `json:"heartbeat_age,omitempty"` // Seconds since the peer's last registry heartbeat, as of the last refresh
	Out          PeerOutbound `json:"out"`
	In           PeerInbound  `json:"in"`
}

// PeerOutbound counts the message batches this node sent to a peer since it started
type PeerOutbound struct {
	BatchesOK     int64  `json:"batches_ok"`
	BatchesFailed int64  `json:"batches_failed"`
	Messages      int64  `json:"messages"` // Messages in batches the peer accepted
	Dropped       int64  `json:"dropped"`  // Messages dropped because the peer's queue was full
	Queued        int    `json:"queued"`   // Messages waiting in the peer's queue right now
	LastOK        int64  `json:"last_ok,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastErrorAt   int64  `json:"last_error_at,omitempty"`
}

// PeerInbound counts the message batches this node received from a peer since it started
type PeerInbound struct {
	Batches  int64 `json:"batches"`
	Messages int64 `json:"messages"`
	LastAt   int64 `json:"last_at,omitempty"`
}

// peerStats counts the message batches exchanged with one peer, for Status. It outlives the peer's
// queue, so a peer that left still shows what happened before it did.
type peerStats struct {
	batchesOK     int64
	batchesFailed int64
	messages      int64
	dropped       int64
	lastOK        time.Time
	lastError     string
	lastErrorAt   time.Time
	batchesIn     int64
	messagesIn    int64
	lastIn        time.Time
}

// apiHealth is the answer on HealthPath: written by handleHealth, read by peerHealthy, so this
// package owns both ends of it. The status code carries the same answer for anything that only
// looks at that, but a peer reads the field, since a 200 from something that is not an ntfy
// cluster node (a proxy on a misconfigured advertise URL) says nothing.
type apiHealth struct {
	Healthy bool `json:"healthy"`
}

// apiState is the peer state-exchange envelope. Each concern is an optional section; future
// concerns (rate limit counters, stats) become siblings of Topics.
type apiState struct {
	Topics  *apiStateTopics     `json:"topics,omitempty"`
	Cancels []*SubscriberCancel `json:"cancels,omitempty"`
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
	queue        *util.LingerQueue[[]byte]
}

// apiMessage is one line of a message request body (NDJSON: one message per line; a single
// message is just a one-line body). It carries the two fields that model.Message does not
// serialize to JSON (Sender and User), which are needed to reconstruct the visitor on the
// receiving node. The origin node travels in a request header, not in the body.
type apiMessage struct {
	Sender  string         `json:"sender,omitempty"`
	User    string         `json:"user,omitempty"`
	Message *model.Message `json:"message"`
}
