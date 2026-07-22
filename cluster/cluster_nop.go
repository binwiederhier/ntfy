package cluster

import (
	"net/http"

	"heckel.io/ntfy/v2/model"
)

// nopCluster is the single-node default: it drops all broadcasts, rejects fan-out requests, and
// reports this node as leader (a single node is trivially the leader, so leader-gated jobs need
// no special-casing in single-node mode).
type nopCluster struct{}

func (c *nopCluster) Broadcast(_ *model.Message) error { return nil }

func (c *nopCluster) ServeDeliver(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (c *nopCluster) IsLeader() bool { return true }

func (c *nopCluster) Close() error { return nil }
