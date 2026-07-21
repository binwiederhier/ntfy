// Package cluster implements cross-node message delivery for a multi-node ntfy cluster. Nodes
// register themselves in a PostgreSQL node registry (control plane) and fan published messages
// out to each other directly over HTTP (data plane); PostgreSQL is never on the message path.
// The single-node default is the Nop broadcaster, which does nothing.
package cluster

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"os"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

// FanoutPath is the HTTP path peer nodes POST fan-out envelopes to. The server routes requests
// for this path to Broadcaster.ServeFanout before its normal authentication pipeline.
const FanoutPath = "/v1/internal/fanout"

// secretHeader carries the shared secret authenticating node-to-node fan-out requests.
const secretHeader = "X-Cluster-Secret"

// envelopeKindMessage identifies a fan-out envelope carrying a published message. Receivers
// ignore envelopes with unknown kinds (with a 200 response), so future envelope kinds can be
// introduced without breaking mixed-version clusters during rolling deploys.
const envelopeKindMessage = "message"

const (
	defaultHeartbeatInterval = 3 * time.Second  // How often a node refreshes its registry heartbeat
	defaultNodeTTL           = 10 * time.Second // A node counts as live if its heartbeat is newer than this
	randomNodeIDLength       = 12
)

// Config configures the cluster broadcaster. It is assembled by the server from its own config,
// which keeps this package free of server types.
type Config struct {
	Enabled           bool          // Master switch; when false, New returns the Nop broadcaster
	NodeID            string        // Stable per-node identifier; defaults to the hostname, then random
	AdvertiseURL      string        // Base URL peers use to reach this node's fan-out endpoint
	Secret            string        // Shared secret authenticating node-to-node fan-out requests
	HeartbeatInterval time.Duration // How often the node registry heartbeat is refreshed
	NodeTTL           time.Duration // Registry rows older than this do not count as live peers
	MaxMessageBytes   int64         // Upper bound for inbound fan-out request bodies
}

// DeliverFunc hands a message received from a peer node to this node's local subscribers. The
// server supplies it, which inverts the dependency: this package never imports the server.
type DeliverFunc func(m *model.Message)

// Broadcaster fans published messages out to peer cluster nodes and receives their fan-out
// requests. Local delivery to a node's own subscribers still happens inline in the server; the
// broadcaster only covers the cross-node hop.
type Broadcaster interface {
	// Broadcast publishes a message to peer nodes. It is fire-and-forget and must not block the
	// caller's request path.
	Broadcast(m *model.Message) error
	// ServeFanout handles an inbound fan-out request from a peer node.
	ServeFanout(w http.ResponseWriter, r *http.Request)
	// IsLeader reports whether this node holds the cluster leader lock. Singleton background
	// jobs (e.g. the Firebase keepaliver) are gated on the leader.
	IsLeader() bool
	// Close stops the broadcaster and releases its resources.
	Close() error
}

// New creates the broadcaster for the given config: the Nop broadcaster when clustering is
// disabled (the single-node default), or the peer-mesh broadcaster otherwise.
func New(conf Config, pool *db.DB, deliver DeliverFunc) (Broadcaster, error) {
	if !conf.Enabled {
		return &Nop{}, nil
	}
	if pool == nil {
		return nil, errors.New("cluster mode requires a PostgreSQL database (set database-url)")
	}
	if conf.AdvertiseURL == "" {
		return nil, errors.New("cluster mode requires an advertise URL (set cluster-advertise-url or base-url)")
	}
	if conf.NodeID == "" {
		conf.NodeID = defaultNodeID()
	}
	if conf.HeartbeatInterval == 0 {
		conf.HeartbeatInterval = defaultHeartbeatInterval
	}
	if conf.NodeTTL == 0 {
		conf.NodeTTL = defaultNodeTTL
	}
	return newMesh(conf, pool, deliver)
}

// defaultNodeID returns the hostname, or a random string if it is unavailable. The hostname is
// preferred because it is stable across restarts: a node that restarts under the same ID reuses
// its registry row instead of abandoning it, and log/metric labels stay traceable.
func defaultNodeID() string {
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}
	return util.RandomString(randomNodeIDLength)
}

// envelope is the wire format for node-to-node fan-out. It carries the origin node ID (so a node
// can skip its own broadcasts) plus the two fields that model.Message does not serialize to JSON
// (Sender and User), which are needed to reconstruct the visitor on the receiving node.
type envelope struct {
	Kind    string         `json:"kind"`
	Origin  string         `json:"origin"`
	Sender  string         `json:"sender,omitempty"`
	User    string         `json:"user,omitempty"`
	Message *model.Message `json:"message,omitempty"`
}

// marshalEnvelope serializes a message and its non-JSON fields for transport to peer nodes.
func marshalEnvelope(origin string, m *model.Message) ([]byte, error) {
	env := &envelope{Kind: envelopeKindMessage, Origin: origin, User: m.User, Message: m}
	if m.Sender.IsValid() {
		env.Sender = m.Sender.String()
	}
	return json.Marshal(env)
}

// unmarshalEnvelope parses a wire envelope and, for message envelopes, reattaches the non-JSON
// fields (Sender, User) onto the message. Callers must check the envelope kind and ignore kinds
// they do not understand.
func unmarshalEnvelope(data []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Kind == envelopeKindMessage {
		if env.Message == nil {
			return nil, errors.New("message envelope without message")
		}
		env.Message.User = env.User
		if env.Sender != "" {
			if addr, err := netip.ParseAddr(env.Sender); err == nil {
				env.Message.Sender = addr
			}
		}
	}
	return &env, nil
}

// Nop is the single-node default broadcaster: it drops all broadcasts, rejects fan-out requests,
// and reports this node as leader (a single node is trivially the leader, so leader-gated jobs
// need no special-casing in single-node mode).
type Nop struct{}

func (b *Nop) Broadcast(_ *model.Message) error { return nil }

func (b *Nop) ServeFanout(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (b *Nop) IsLeader() bool { return true }

func (b *Nop) Close() error { return nil }
