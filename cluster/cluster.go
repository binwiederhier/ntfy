// Package cluster implements cross-node message delivery for a multi-node ntfy cluster. Nodes
// register themselves in a PostgreSQL node registry (control plane) and fan published messages
// out to each other directly over HTTP (data plane); PostgreSQL is never on the message path.
// The single-node default is the nop cluster, which does nothing.
package cluster

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/limit"
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
	// load balancers' agents: each LB maintains its own upstream list from it, with the
	// active members only.
	MembersPath = "/v1/cluster/members"
	// HealthPath reports a node's cluster health to its PEERS (200 healthy, 503 not): a node
	// that lost its registration probes it to find out whether anywhere better exists for its
	// subscribers. The server's public /v1/health is a separate endpoint for load balancers.
	HealthPath = "/v1/cluster/health"
	// StatusPath reports this node's view of the cluster for operators: its role and leadership,
	// and per peer what it sent there and received from there (see Status)
	StatusPath = "/v1/cluster/status"
	// UsagePath receives a peer's usage deltas (JSON apiUsage): what its keys used of the mesh
	// kinds since its last round, to be charged into this node's buckets. A node that does not
	// know the path (an older build) answers 404 and is left alone.
	UsagePath = "/v1/cluster/usage"
)

// NodeID identifies a cluster node; it keys the registry, the per-peer queues, and the peer
// state table.
//
// Naming convention: a "node" is any cluster member in the absolute sense (identity, registry,
// config); a "peer" is another node as seen from this one (Peers, peerQueue, peerState). A peer
// IS a node, which is why peer values carry a NodeID.
type NodeID string

// Role is a node's part in the cluster; the registry owns it because leadership is derived there
type Role = registry.Role

type (
	// Peer is a live peer node (see registry.Peer)
	Peer = registry.Peer

	// Peers is a list of live peer nodes (see registry.Peers)
	Peers = registry.Peers
)

// Node roles (see registry.Role)
const (
	RoleActive  = registry.RoleActive
	RolePassive = registry.RolePassive
)

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
	Role         Role   `json:"role"` // Agents route client traffic to active nodes only
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
	// BroadcastState pushes a state delta (first-subscriber hints, access changes, deleted users) to all
	// peers. Nop single-node.
	BroadcastState(state *State)
	// BroadcastUsage sends the usage deltas of this node's rate buckets (see limit.Service.TakeDeltas)
	// to all peers, which charge them into their own buckets (Config.UsageFunc). Each peer has a
	// single-slot queue: a round that finds the previous one still in flight to a peer is dropped
	// for that peer, so a wedged peer costs at most its own rounds. Nop single-node.
	BroadcastUsage(deltas []limit.Delta)
	// IsLeader reports whether this node is the one that should run the cluster's singleton jobs
	// (e.g. the Firebase keepaliver). It is derived from membership, not held as a lock, and it is
	// a belief with a lease rather than a fence: see registry.IsLeader for what it does and does
	// not promise. Always true single-node.
	IsLeader() bool
	// Members lists the live cluster members (this node plus its live peers); served on
	// MembersPath for the load balancers' agents.
	Members() []Member
	// Healthy reports whether this node is fit to serve: its registry heartbeat is fresh
	// enough (within NodeTTL) that peers still forward messages to it. Health checkers must
	// fail open (never pull ALL nodes): during a full database outage every node reports
	// unhealthy while the mesh keeps delivering on stale peer caches.
	Healthy() bool
	// Ready reports whether a load balancer should send clients here: an active node that is
	// Healthy, or that is not but has no other active node to take its clients. The only active
	// node stays ready with a stale heartbeat, since pulling it would leave clients nowhere to go.
	// A draining node is never ready: it acts as passive (see Role).
	Ready() bool
	// Role reports the role this node acts in: its configured role (active, or passive), or passive
	// while draining
	Role() Role
	// SetDraining marks this node as draining (moving its clients to other nodes, e.g. before a
	// shutdown), or not. While draining, it acts as passive: it is not ready, does not lead, and its
	// peers see it as passive. Nop single-node.
	SetDraining(on bool)
	// Draining reports whether this node is draining. Never single-node.
	Draining() bool
	// Peers returns the live peer nodes (as of the last registry refresh), excluding this node; e.g.
	// Peers().HasActive() tells whether there is somewhere for this node's clients to go. None
	// single-node.
	Peers() Peers
	// Close stops the cluster and releases its resources.
	Close() error
}

// New creates the cluster for the given config: the nop cluster when clustering is disabled (the
// single-node default), or the peer-mesh cluster otherwise.
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
	if conf.Role == "" {
		conf.Role = RoleActive
	} else if !conf.Role.Valid() {
		return nil, fmt.Errorf("invalid cluster role %q (set experimental-cluster-role to %s or %s)", conf.Role, RoleActive, RolePassive)
	}
	if conf.HeartbeatInterval == 0 {
		conf.HeartbeatInterval = defaultHeartbeatInterval
	}
	if conf.NodeTTL == 0 {
		conf.NodeTTL = defaultNodeTTL
	}
	return newMeshCluster(conf, pool, deliver)
}
