// Package registry implements cluster membership: each node upserts its own row into the
// node_registry table with a fresh heartbeat, and discovers its peers by reading the other
// fresh rows. Node IDs are plain strings here; the cluster package layers its NodeID type on
// top.
package registry

import (
	"context"
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

// Registry queries
const (
	upsertNodeQuery = `
		INSERT INTO node_registry (node_id, advertise_url, last_heartbeat)
		VALUES ($1, $2, $3)
		ON CONFLICT (node_id) DO UPDATE SET advertise_url = EXCLUDED.advertise_url, last_heartbeat = EXCLUDED.last_heartbeat
	`
	selectPeersQuery     = `SELECT node_id, advertise_url FROM node_registry WHERE last_heartbeat >= $1 AND node_id != $2`
	pruneStaleNodesQuery = `DELETE FROM node_registry WHERE last_heartbeat < $1`
	deleteNodeQuery      = `DELETE FROM node_registry WHERE node_id = $1`
)

// Schema version and queries

const (
	maxOpTimeout = 5 * time.Second // Upper bound for one registry database call (ttl/2 below that)

	schemaVersion  = 1
	schemaStoreKey = "node_registry"
)

var (
	createTable = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS node_registry (
			node_id        TEXT PRIMARY KEY,
			advertise_url  TEXT NOT NULL,
			last_heartbeat BIGINT NOT NULL
		)
	`)
)

// Peer is a live remote node as read from the registry.
type Peer struct {
	NodeID       string
	AdvertiseURL string
}

// Registry is the node membership table (control plane): each node upserts its own row with a
// fresh heartbeat every few seconds, and peers are the other rows with a heartbeat newer than
// the TTL. Stale rows are pruned by the leader.
//
// Reads are split in two: Refresh queries the table and is called from the heartbeat loop, and
// Peers only ever returns the snapshot of the last Refresh. The publish path therefore never
// waits on the database, not even during an outage, when the snapshot simply goes stale.
type Registry struct {
	pool         *db.DB
	nodeID       string
	advertiseURL string
	ttl          time.Duration
	peers        []*Peer    // Snapshot of the last Refresh; nil until the first one
	mu           sync.Mutex // Protects peers
}

// New creates or migrates the registry schema and returns this node's membership handle. It
// does NOT register the node: joining the cluster is an explicit Register call, owned by the
// caller, so read-only uses of the registry stay side-effect free.
func New(pool *db.DB, nodeID, advertiseURL string, ttl time.Duration) (*Registry, error) {
	if err := schema.Migrate(pool.Primary(), schema.Postgres, schemaStoreKey, schemaVersion, createTable, nil); err != nil {
		return nil, err
	}
	return &Registry{
		pool:         pool,
		nodeID:       nodeID,
		advertiseURL: advertiseURL,
		ttl:          ttl,
	}, nil
}

// Register upserts this node into the registry with a fresh heartbeat. It is a pure write: it
// does not touch the peer cache, because our own row is excluded from Peers() anyway.
func (r *Registry) Register() error {
	ctx, cancel := r.opContext()
	defer cancel()
	_, err := r.pool.ExecContext(ctx, upsertNodeQuery, r.nodeID, r.advertiseURL, time.Now().Unix())
	return err
}

// Peers returns the live peer nodes as of the last Refresh (all registry rows with a fresh
// heartbeat then, excluding this node). It never touches the database: before the first Refresh
// there are no peers, and during an outage the snapshot is served as is (dead peers in it only
// cost failed sends).
func (r *Registry) Peers() []*Peer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.peers
}

// Prune deletes registry rows whose heartbeat is long expired. Only the leader calls this; the
// grace period of 3x the TTL avoids deleting rows of nodes that are merely slow to heartbeat.
func (r *Registry) Prune() error {
	ctx, cancel := r.opContext()
	defer cancel()
	_, err := r.pool.ExecContext(ctx, pruneStaleNodesQuery, time.Now().Add(-3*r.ttl).Unix())
	return err
}

// Deregister deletes this node's registry row; called on shutdown.
func (r *Registry) Deregister() error {
	ctx, cancel := r.opContext()
	defer cancel()
	_, err := r.pool.ExecContext(ctx, deleteNodeQuery, r.nodeID)
	return err
}

// Refresh reads the live peer set from the database and replaces the snapshot Peers serves.
// The mesh calls it once at startup and on every heartbeat tick, so peer-set staleness is
// bounded by the heartbeat interval. On error the previous snapshot stays in place.
func (r *Registry) Refresh() ([]*Peer, error) {
	peers, err := r.queryPeers()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.peers = peers
	r.mu.Unlock()
	return peers, nil
}

// queryPeers reads the current live peer set from the registry table.
func (r *Registry) queryPeers() ([]*Peer, error) {
	ctx, cancel := r.opContext()
	defer cancel()
	cutoff := time.Now().Add(-r.ttl).Unix()
	rows, err := r.pool.QueryContext(ctx, selectPeersQuery, cutoff, r.nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	peers := make([]*Peer, 0)
	for rows.Next() {
		p := &Peer{}
		if err := rows.Scan(&p.NodeID, &p.AdvertiseURL); err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return peers, nil
}

// opContext bounds a registry database call, so a partition (dropped packets) makes it fail
// instead of hanging the heartbeat loop and shutdown
func (r *Registry) opContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), min(r.ttl/2, maxOpTimeout))
}
