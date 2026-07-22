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

// FanoutPath is the HTTP path peer nodes POST fan-out envelopes to. The server routes requests
// for this path to Cluster.ServeFanout before its normal authentication pipeline.
const FanoutPath = "/v1/internal/fanout"

// secretHeader carries the shared secret authenticating node-to-node fan-out requests.
const secretHeader = "X-Cluster-Secret"

// originHeader carries the sending node's ID on fan-out requests, so a node can skip requests
// that carry its own broadcasts (loop prevention).
const originHeader = "X-Cluster-Origin"

// contentTypeNDJSON is the content type of fan-out request bodies (one JSON message per line,
// matching the framing of ntfy's own /topic/json subscribe stream). Future node-to-node request
// types get their own paths on the cluster listener; an old node answering 404 on an unknown
// path keeps mixed-version clusters working during rolling deploys.
const contentTypeNDJSON = "application/x-ndjson"

const (
	defaultHeartbeatInterval = 3 * time.Second  // How often a node refreshes its registry heartbeat
	defaultNodeTTL           = 10 * time.Second // A node counts as live if its heartbeat is newer than this

	// DefaultBatchLinger is how long a fan-out message may wait in a peer's queue for more
	// messages to arrive, so they are delivered as one batch. It trades up to this much
	// cross-node latency for a bounded request rate per peer.
	DefaultBatchLinger = 500 * time.Millisecond
)

// Config configures the cluster. It is assembled by the server from its own config, which keeps
// this package free of server types.
type Config struct {
	Enabled           bool          // Master switch; when false, New returns the nop cluster
	NodeID            string        // Stable per-node identifier; required
	AdvertiseURL      string        // Base URL peers use to reach this node's fan-out endpoint
	Secret            string        // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration // Registry rows older than this do not count as live peers
	BatchLinger       time.Duration // How long messages wait in a peer queue to form a batch; 0 = send immediately
	MaxMessageBytes   int64         // Upper bound for a single message on the wire (batch limits derive from this)
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// Cluster fans published messages out to peer cluster nodes and receives their fan-out requests.
// Local delivery to a node's own subscribers still happens inline in the server; the cluster
// only covers the cross-node hop.
type Cluster interface {
	// Broadcast publishes a message to peer nodes. It is fire-and-forget and must not block the
	// caller's request path.
	Broadcast(m *model.Message) error
	// ServeFanout handles an inbound fan-out request from a peer node.
	ServeFanout(w http.ResponseWriter, r *http.Request)
	// IsLeader reports whether this node holds the cluster leader lock. Singleton background
	// jobs (e.g. the Firebase keepaliver) are gated on the leader.
	IsLeader() bool
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
	return newMeshCluster(conf, pool, deliver)
}
