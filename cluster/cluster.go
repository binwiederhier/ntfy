// Package cluster implements cross-node message delivery for a multi-node ntfy cluster. Nodes
// register themselves in a PostgreSQL node registry (control plane) and fan published messages
// out to each other directly over HTTP (data plane); PostgreSQL is never on the message path.
// The single-node default is the nop cluster, which does nothing.
//
// This is the seam only: the interface the server talks to, the single-node default behind it,
// and the message wire format. The peer mesh that implements the interface for real is the next
// piece, so on a single node this package is the nop and nothing else.
package cluster

import (
	"errors"
	"net/http"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/model"
)

// The cluster API: every kind of node-to-node communication is a path under /v1/cluster/,
// served only on the dedicated cluster listener. Future concerns (rate limit counters, stats)
// become new paths or new sections of the state envelope.
const (
	// MessagePath receives batches of published messages (NDJSON, one apiMessage per line).
	MessagePath = "/v1/cluster/message"
	// StatePath receives peer state (JSON apiState): full subscription snapshots and
	// incremental updates.
	StatePath = "/v1/cluster/state"
	// MembersPath lists the live cluster members (this node plus its live peers), for the
	// load balancers' agents: each LB maintains its own upstream list from it.
	MembersPath = "/v1/cluster/members"
	// HealthPath reports a node's cluster health (200 healthy, 503 not); served on the cluster
	// listener too, where isolated nodes probe their peers.
	HealthPath = "/v1/health"
)

// NodeID identifies a cluster node; it keys the registry, the per-peer queues, and the peer
// state table.
//
// Naming convention: a "node" is any cluster member in the absolute sense (identity, registry,
// config); a "peer" is another node as seen from this one (Peers, peerQueue, peerState). A peer
// IS a node, which is why peer values carry a NodeID.
type NodeID string

// tag is the log tag for everything cluster-related, so "tag=cluster -> trace" turns on
// per-message decisions without raising the level anywhere else
const tag = "cluster"

const (
	defaultHeartbeatInterval = 3 * time.Second  // How often a node refreshes its registry heartbeat
	defaultNodeTTL           = 30 * time.Second // A node counts as live if its heartbeat is newer than this; generous to avoid false-dead flapping (see plans)
)

// Member is one live cluster node as reported by MembersPath
type Member struct {
	NodeID       NodeID `json:"node_id"`
	AdvertiseURL string `json:"advertise_url"`
	Healthy      bool   `json:"healthy"`
}

// Cluster fans published messages out to peer cluster nodes and receives their fan-out requests.
// Local delivery to a node's own subscribers still happens inline in the server; the cluster
// only covers the cross-node hop.
type Cluster interface {
	http.Handler
	// ForwardMessage sends a locally published message on to every live peer node; peers
	// without a subscriber for the topic drop it. It is fire-and-forget and must not block the
	// caller's request path.
	ForwardMessage(m *model.Message) error
	// BroadcastState pushes a state delta (first-subscriber hints, subscriber cancels) to all
	// peers. Nop single-node.
	BroadcastState(state *State)
	// IsLeader reports whether this node holds the cluster leader lock. Singleton background
	// jobs (e.g. the Firebase keepaliver) are gated on the leader.
	IsLeader() bool
	// Members lists the live cluster members (this node plus its live peers); served on
	// MembersPath for the load balancers' agents.
	Members() []Member
	// Healthy reports whether this node is fit to serve: its registry heartbeat is fresh
	// enough (within NodeTTL) that peers still forward messages to it. Health checkers must
	// fail open (never pull ALL nodes): during a full database outage every node reports
	// unhealthy while the mesh keeps delivering on stale peer caches.
	Healthy() bool
	// Close stops the cluster and releases its resources.
	Close() error
}

// New creates the cluster for the given config: the nop cluster when clustering is disabled (the
// single-node default), or the peer-mesh cluster otherwise. The mesh is not in this build yet, so
// an enabled config is validated and then refused; cmd refuses to start a server with
// experimental-cluster-listen set, so a running server never reaches that.
func New(conf *Config, pool *db.DB, deliver DeliverFunc) (Cluster, error) {
	if !conf.Enabled {
		return &nopCluster{}, nil
	}
	if pool == nil {
		return nil, errors.New("cluster mode requires a PostgreSQL database (set database-url)")
	}
	if conf.AdvertiseURL == "" {
		return nil, errors.New("cluster mode requires an advertise URL (set experimental-cluster-advertise-url)")
	}
	if conf.NodeID == "" {
		return nil, errors.New("cluster mode requires a stable node ID (set experimental-cluster-node-id)")
	}
	if conf.HeartbeatInterval == 0 {
		conf.HeartbeatInterval = defaultHeartbeatInterval
	}
	if conf.NodeTTL == 0 {
		conf.NodeTTL = defaultNodeTTL
	}
	// Everything above is the config contract the mesh relies on. The mesh itself is the next
	// piece, and cmd refuses to start a server with experimental-cluster-listen set, so nothing
	// but a test reaches this line.
	return nil, errors.New("clustering is not implemented in this build")
}
