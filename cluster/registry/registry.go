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
	// Heartbeats are stamped with the database clock, never a node's: liveness compares them
	// across nodes, and leadership is derived from liveness (see IsLeader)
	upsertNodeQuery = `
		INSERT INTO node_registry (node_id, advertise_url, last_heartbeat)
		VALUES ($1, $2, EXTRACT(EPOCH FROM now())::BIGINT)
		ON CONFLICT (node_id) DO UPDATE SET advertise_url = EXCLUDED.advertise_url, last_heartbeat = EXCLUDED.last_heartbeat
	`
	// selectLiveNodesQuery returns every live node INCLUDING this one: the caller needs the
	// whole set, because the leader is the live node with the lowest id
	selectLiveNodesQuery = `SELECT node_id, advertise_url FROM node_registry WHERE last_heartbeat >= EXTRACT(EPOCH FROM now())::BIGINT - $1 ORDER BY node_id`
	pruneStaleNodesQuery = `DELETE FROM node_registry WHERE last_heartbeat < EXTRACT(EPOCH FROM now())::BIGINT - $1`
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
//
// Leadership is derived from the same data, with no second mechanism: the leader is the live
// node with the lowest id (see IsLeader).
type Registry struct {
	pool         *db.DB
	nodeID       string
	advertiseURL string
	ttl          time.Duration
	peers        []*Peer    // Snapshot of the last Refresh; nil until the first one
	registeredAt time.Time  // When the last successful Register was ISSUED; zero = never
	lowestSince  time.Time  // When this node first saw itself as the lowest live id; zero = it is not
	mu           sync.Mutex // Protects peers, registeredAt and lowestSince
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

// Register upserts this node into the registry with a fresh heartbeat. The registration is
// dated from when the statement was ISSUED, not when it returned: that timestamp bounds how
// long this node may believe it is the leader, and a late response must not extend it.
func (r *Registry) Register() error {
	ctx, cancel := r.opContext()
	defer cancel()
	issued := time.Now()
	if _, err := r.pool.ExecContext(ctx, upsertNodeQuery, r.nodeID, r.advertiseURL); err != nil {
		return err
	}
	r.mu.Lock()
	r.registeredAt = issued
	r.mu.Unlock()
	return nil
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
	_, err := r.pool.ExecContext(ctx, pruneStaleNodesQuery, int64((3 * r.ttl).Seconds()))
	return err
}

// IsLeader reports whether this node should run the cluster's singleton jobs. Leadership is not
// a separate mechanism: the leader is the live node with the lowest id, which every node can
// work out from the membership data it already reads.
//
// Three conditions, and the last two are what keep two nodes from both believing:
//
//  1. This node is the lowest live id, as of its last Refresh.
//  2. Its own registration is fresh, dated from when the heartbeat was issued. A node that
//     cannot reach the database stops believing within one TTL, which is also the point at
//     which its peers stop seeing it as live, and it stops first because its own deadline is
//     measured from before the write the others observed.
//  3. It has been the lowest live id for a whole TTL (the promotion hold-off). A returning
//     lower-id node would otherwise believe at once, while the incumbent still believes until
//     its next read; the incumbent cannot believe longer than one TTL past its last heartbeat,
//     so waiting that out covers it.
func (r *Registry) IsLeader() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registeredAt.IsZero() || time.Since(r.registeredAt) >= r.ttl {
		return false // Our registration is stale: peers no longer count us as live
	}
	return !r.lowestSince.IsZero() && time.Since(r.lowestSince) >= r.ttl
}

// Deregister deletes this node's registry row; called on shutdown.
func (r *Registry) Deregister() error {
	ctx, cancel := r.opContext()
	defer cancel()
	_, err := r.pool.ExecContext(ctx, deleteNodeQuery, r.nodeID)
	return err
}

// Refresh reads the live node set from the database, replaces the snapshot Peers serves, and
// updates this node's leadership standing. The mesh calls it once at startup and on every
// heartbeat tick, so both are bounded by the heartbeat interval. On error the previous snapshot
// and standing stay in place; leadership then lapses on its own via the registration deadline.
func (r *Registry) Refresh() ([]*Peer, error) {
	peers, lowest, err := r.queryLiveNodes()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.peers = peers
	if lowest == r.nodeID {
		if r.lowestSince.IsZero() {
			r.lowestSince = time.Now() // Start of the promotion hold-off
		}
	} else {
		r.lowestSince = time.Time{}
	}
	r.mu.Unlock()
	return peers, nil
}

// queryLiveNodes reads the live nodes and returns this node's peers plus the lowest live id
// (which may be this node).
func (r *Registry) queryLiveNodes() ([]*Peer, string, error) {
	ctx, cancel := r.opContext()
	defer cancel()
	rows, err := r.pool.QueryContext(ctx, selectLiveNodesQuery, int64(r.ttl.Seconds()))
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	peers := make([]*Peer, 0)
	lowest := ""
	for rows.Next() { // Ordered by node_id, so the first row is the lowest
		p := &Peer{}
		if err := rows.Scan(&p.NodeID, &p.AdvertiseURL); err != nil {
			return nil, "", err
		}
		if lowest == "" {
			lowest = p.NodeID
		}
		if p.NodeID != r.nodeID {
			peers = append(peers, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return peers, lowest, nil
}

// opContext bounds a registry database call, so a partition (dropped packets) makes it fail
// instead of hanging the heartbeat loop and shutdown
func (r *Registry) opContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), min(r.ttl/2, maxOpTimeout))
}
