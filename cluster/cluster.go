// Package cluster implements cross-node message delivery for a multi-node ntfy cluster. Nodes
// register themselves in a PostgreSQL node registry (control plane) and fan published messages
// out to each other directly over HTTP (data plane); PostgreSQL is never on the message path.
// The single-node default is the nop cluster, which does nothing.
package cluster

import (
	"errors"
	"net/http"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/model"
)

// The internal peer API: every kind of node-to-node communication is a path under
// /v1/internal/, served only on the dedicated cluster listener. Future concerns (rate limit
// counters, stats) become new paths or new sections of the state envelope.
const (
	// MessagePath receives batches of published messages (NDJSON, one apiMessage per line).
	MessagePath = "/v1/internal/message"
	// StatePath receives peer state (JSON apiState): full subscription snapshots and
	// incremental updates.
	StatePath = "/v1/internal/state"
)

// NodeID identifies a cluster node; it keys the registry, the per-peer queues, and the peer
// state table.
//
// Naming convention: a "node" is any cluster member in the absolute sense (identity, registry,
// config); a "peer" is another node as seen from this one (Peers, peerQueue, peerState). A peer
// IS a node, which is why peer values carry a NodeID.
type NodeID string

// secretHeader carries the shared secret authenticating node-to-node fan-out requests.
const secretHeader = "X-Cluster-Secret"

// originHeader carries the sending node's ID on fan-out requests, so a node can skip requests
// that carry its own broadcasts (loop prevention).
const originHeader = "X-Cluster-Origin"

// Content types of the peer API: message bodies are NDJSON (one JSON message per line, matching
// the framing of ntfy's own /topic/json subscribe stream), state bodies are plain JSON. Future
// node-to-node request types get their own paths on the cluster listener; an old node answering
// 404 on an unknown path keeps mixed-version clusters working during rolling deploys.
const (
	contentTypeNDJSON = "application/x-ndjson"
	contentTypeJSON   = "application/json"
)

const (
	defaultHeartbeatInterval = 3 * time.Second  // How often a node refreshes its registry heartbeat
	defaultNodeTTL           = 30 * time.Second // A node counts as live if its heartbeat is newer than this; generous to avoid false-dead flapping (see plans)
	defaultStateInterval     = 15 * time.Second // How often the full subscription state is pushed to peers

	// DefaultBatchLinger is how long a fan-out message may wait in a peer's queue for more
	// messages to arrive, so they are delivered as one batch. It trades up to this much
	// cross-node latency for a bounded request rate per peer.
	DefaultBatchLinger = 500 * time.Millisecond
)

// Config configures the cluster. It is assembled by the server from its own config, which keeps
// this package free of server types.
type Config struct {
	Enabled           bool          // Master switch; when false, New returns the nop cluster
	NodeID            NodeID        // Stable per-node identifier; required
	AdvertiseURL      string        // Base URL peers use to reach this node's fan-out endpoint
	Secret            string        // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration // Registry rows older than this do not count as live peers
	BatchLinger       time.Duration // How long messages wait in a peer queue to form a batch; 0 = send immediately
	StateInterval     time.Duration // How often the full subscription state is pushed to peers
	MaxMessageBytes   int64         // Upper bound for a single message on the wire (batch limits derive from this)
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// TopicsFunc returns the topics that currently have at least one live subscriber, computed
// fresh on every call: membership is never tracked as a list, so topics "leave" simply by not
// appearing in the next snapshot. The server supplies it (same inversion as DeliverFunc).
type TopicsFunc func() []string

// Cluster fans published messages out to peer cluster nodes and receives their fan-out requests.
// Local delivery to a node's own subscribers still happens inline in the server; the cluster
// only covers the cross-node hop.
type Cluster interface {
	// The internal peer API (/v1/internal/*), served on the dedicated cluster listener. The
	// nop cluster answers 404.
	http.Handler
	// Relay sends a locally published message on to the peer nodes that may have subscribers
	// for its topic (all of them, when subscription knowledge is missing or stale). It is
	// fire-and-forget and must not block the caller's request path.
	Relay(m *model.Message) error
	// AnnounceTopics tells peers that these topics just gained their first local subscriber,
	// closing the routing-knowledge window to ~one round trip. Nop in single-node mode.
	AnnounceTopics(topics []string)
	// IsLeader reports whether this node holds the cluster leader lock. Singleton background
	// jobs (e.g. the Firebase keepaliver) are gated on the leader.
	IsLeader() bool
	// TODO(T8): add Healthy() bool reflecting registration health (unhealthy when the last
	// successful Register is older than NodeTTL, i.e. when peers stop relaying to this node),
	// so /v1/health can pull a DB-partitioned node out of DNS rotation. Needs a fail-open
	// answer at the health checker first: during a full DB outage ALL nodes fail to register
	// while the mesh keeps delivering on stale peer caches, and naively pulling every node
	// would turn a control-plane blip into a total outage.
	// Close stops the cluster and releases its resources.
	Close() error
}

// New creates the cluster for the given config: the nop cluster when clustering is disabled (the
// single-node default), or the peer-mesh cluster otherwise.
func New(conf *Config, pool *db.DB, deliver DeliverFunc, topics TopicsFunc) (Cluster, error) {
	if !conf.Enabled {
		return &nopCluster{}, nil
	}
	if pool == nil {
		return nil, errors.New("cluster mode requires a PostgreSQL database (set database-url)")
	}
	if conf.AdvertiseURL == "" {
		return nil, errors.New("cluster mode requires an advertise URL (set cluster-advertise-url)")
	}
	if conf.NodeID == "" {
		return nil, errors.New("cluster mode requires a stable node ID (set cluster-node-id)")
	}
	if conf.HeartbeatInterval == 0 {
		conf.HeartbeatInterval = defaultHeartbeatInterval
	}
	if conf.NodeTTL == 0 {
		conf.NodeTTL = defaultNodeTTL
	}
	if conf.StateInterval == 0 {
		conf.StateInterval = defaultStateInterval
	}
	return newMeshCluster(conf, pool, deliver, topics)
}
