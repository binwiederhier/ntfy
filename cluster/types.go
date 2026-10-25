package cluster

import (
	"time"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// Config configures the cluster. It is assembled by the server from its own config, which keeps
// this package free of server types.
type Config struct {
	Enabled           bool              // Master switch; when false, New returns the nop cluster
	NodeID            NodeID            // Stable per-node identifier; required
	AdvertiseURL      string            // Base URL peers use to reach this node's fan-out endpoint
	Role              Role              // Active (default) or passive: a passive node never leads and never fences
	Secret            string            // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration     // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration     // Registry rows older than this do not count as live peers
	BatchLinger       time.Duration     // How long messages wait in a peer queue to form a batch; 0 = send immediately
	MaxMessageBytes   int64             // Upper bound for a single message on the wire (batch limits derive from this)
	UsersDeletedFunc  UsersDeletedFunc  // Told that a peer deleted users; may be nil
	AccessChangedFunc AccessChangedFunc // Told that a peer changed access entries; may be nil
	TopicsAddedFunc   TopicsAddedFunc   // Told about topics that just gained their first subscriber on a peer; may be nil
	IsolatedFunc      IsolatedFunc      // Called while this node is cut off from the cluster; may be nil
	UsageFunc         UsageFunc         // Told what a peer's keys used of the mesh kinds (see BroadcastUsage); may be nil
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// State is a state delta for Cluster.BroadcastState.
type State struct {
	AddedTopics    []string // Topics that just gained their first local subscriber on this node
	DeletedUserIDs []string // Users this node just deleted: peers end their streams
	AccessChanged  bool     // Access entries changed (written by this node): peers reload their access cache
}

// UsersDeletedFunc is called with the IDs of users a peer just deleted (State.DeletedUserIDs). The
// server supplies it to end those users' streams on this node; it must not re-broadcast (loop
// prevention). Access changes do not need this, since peers reload their access cache and end
// the streams that lost access themselves (see AccessChangedFunc), but a deleted user without
// access entries (e.g. an admin) leaves nothing in the cache to compare.
type UsersDeletedFunc func(userIDs []string)

// TopicsAddedFunc is called with topics a peer announced as having just gained their first
// subscriber there. The server supplies it to drop cached negative rate-visitor lookups for
// those topics; it must not re-broadcast (loop prevention). It is a cache hint only: a lost
// announcement costs a stale lookup until its TTL, never a lost message.
type TopicsAddedFunc func(topics []string)

// AccessChangedFunc is called when a peer changed access entries (State.AccessChanged). The server
// supplies it to reload its access cache, which ends the local streams that lost access; it must
// not re-broadcast (loop prevention). A lost signal is caught by the periodic access cache reload.
type AccessChangedFunc func()

// UsageFunc is called with the usage deltas a peer sent (see Cluster.BroadcastUsage). The
// server supplies it to charge the deltas into its own buckets; it must not re-broadcast (loop
// prevention). A lost round costs one round of convergence, never a lost message.
type UsageFunc func(deltas []limit.Delta)

// IsolatedFunc is called while this node lost its registration but a peer is healthy: peers no
// longer forward messages to it, so the server closes its subscribers, which reconnect to a
// healthy node (see meshCluster.maybeIsolated).
type IsolatedFunc func()

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
	LastSeen     int64        `json:"last_seen,omitempty"`     // When this node last saw the peer in the registry (or first heard from it)
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
	LastFailed    bool   `json:"last_failed,omitempty"` // The most recent batch failed; timestamps cannot order outcomes within one second
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
	out      PeerOutbound // Queued is filled in when Status is built, from the live queue
	in       PeerInbound
	lastSeen int64 // Unix seconds; a peer gone for peerStatsRetention is forgotten
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
	Topics         *apiStateTopics `json:"topics,omitempty"`
	DeletedUserIDs []string        `json:"deleted_user_ids,omitempty"`
	AccessChanged  bool            `json:"access_changed,omitempty"`
}

// apiStateTopics carries topics that just gained their first subscriber on the sending node.
type apiStateTopics struct {
	Added []string `json:"added,omitempty"`
}

// apiUsage is a round of usage deltas (see BroadcastUsage); short field names, since a round can
// carry thousands of entries
type apiUsage struct {
	Deltas []*apiUsageDelta `json:"d"`
}

// apiUsageDelta is one entry of a round: key k used n units of kind since the sender's last round
type apiUsageDelta struct {
	Key  string `json:"k"`
	Kind string `json:"kind"`
	N    int64  `json:"n"`
}

// usageQueue is the single-slot send queue of usage rounds for one peer (see BroadcastUsage)
type usageQueue struct {
	advertiseURL string
	slot         chan []byte
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
