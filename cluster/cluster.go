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

const (
	// secretHeader carries the shared secret authenticating node-to-node fan-out requests.
	secretHeader = "X-Cluster-Secret"

	// originHeader carries the sending node's ID on fan-out requests, so a node can skip
	// requests that carry its own broadcasts (loop prevention).
	originHeader = "X-Cluster-Origin"
)

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

// Cluster fans published messages out to peer cluster nodes and receives their fan-out requests.
// Local delivery to a node's own subscribers still happens inline in the server; the cluster
// only covers the cross-node hop.
type Cluster interface {
	http.Handler
	// ForwardMessage sends a locally published message on to the peer nodes that may have subscribers
	// for its topic (all of them, when subscription knowledge is missing or stale). It is
	// fire-and-forget and must not block the caller's request path.
	ForwardMessage(m *model.Message) error
	// BroadcastState pushes a subscription-state delta to ALL peers (unlike ForwardMessage,
	// which routes), closing the routing-knowledge window to ~one round trip. Nop single-node.
	BroadcastState(state *State)
	// IsLeader reports whether this node holds the cluster leader lock. Singleton background
	// jobs (e.g. the Firebase keepaliver) are gated on the leader.
	IsLeader() bool
	// Healthy reports whether this node is fit to serve: its registry heartbeat is fresh
	// enough (within NodeTTL) that peers still forward messages to it. Health checkers must
	// fail open (never pull ALL nodes): during a full database outage every node reports
	// unhealthy while the mesh keeps delivering on stale peer caches.
	Healthy() bool
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
