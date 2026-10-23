package cluster

import (
	"net/http"

	"heckel.io/ntfy/v2/model"
)

// nopCluster is the single-node default: it drops all relayed messages, rejects peer API requests, and
// reports this node as leader (a single node is trivially the leader, so leader-gated jobs need
// no special-casing in single-node mode).
type nopCluster struct{}

func (c *nopCluster) ForwardMessage(_ *model.Message) error {
	return nil // No peers to forward to
}

func (c *nopCluster) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound) // The peer API does not exist on a single node
}

func (c *nopCluster) BroadcastState(_ *State) {
	// No peers to tell
}

func (c *nopCluster) IsLeader() bool {
	return true // A single node is trivially the leader
}

func (c *nopCluster) Members() []Member {
	return nil // No registry, so no membership to report
}

func (c *nopCluster) Healthy() bool {
	return true // Health here means "the registry heartbeat is fresh", and there is no registry
}

func (c *nopCluster) Role() Role {
	return RoleActive // A single node takes all client traffic
}

func (c *nopCluster) Maintenance() bool {
	return false // See SetMaintenance
}

func (c *nopCluster) Peers() Peers {
	return nil // A single node has no peers
}

func (c *nopCluster) SetMaintenance(_ bool) {
	// Nothing to tell: there are no peers, and a single node never enters maintenance mode
}

func (c *nopCluster) Ready() bool {
	return true // A single node takes all client traffic
}

func (c *nopCluster) Close() error {
	return nil // Nothing was started
}
