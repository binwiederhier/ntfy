// Package registry implements cluster membership: each node upserts its own row into the
// node_registry table with a fresh heartbeat, and discovers its peers by reading the other
// fresh rows. Node IDs are plain strings here; the cluster package layers its NodeID type on
// top.
package registry

import (
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
)

// Registry queries. The node_registry table is control-plane state maintained by every node
// independently and idempotently, so it lives outside the message/user schema_version framework.
const (
	createTableQuery = `
		CREATE TABLE IF NOT EXISTS node_registry (
			node_id        TEXT PRIMARY KEY,
			advertise_url  TEXT NOT NULL,
			last_heartbeat BIGINT NOT NULL
		)
	`
	upsertNodeQuery = `
		INSERT INTO node_registry (node_id, advertise_url, last_heartbeat)
		VALUES ($1, $2, $3)
		ON CONFLICT (node_id) DO UPDATE SET advertise_url = EXCLUDED.advertise_url, last_heartbeat = EXCLUDED.last_heartbeat
	`
	selectPeersQuery         = `SELECT node_id, advertise_url FROM node_registry WHERE last_heartbeat >= $1 AND node_id != $2`
	pruneStaleNodesQuery     = `DELETE FROM node_registry WHERE last_heartbeat < $1`
	deleteNodeQuery          = `DELETE FROM node_registry WHERE node_id = $1`
	tryAdvisoryXactLockQuery = `SELECT pg_advisory_xact_lock($1)`
)

// schemaLockKey is the advisory-lock key serializing registry table creation across nodes. It is
// distinct from the cluster leader lock key, which is session-scoped and long-held.
const schemaLockKey = int64(0x6e746679c) // "ntfy" + c (create)

// Peer is a live remote node as read from the registry.
type Peer struct {
	NodeID       string
	AdvertiseURL string
}

// Registry is the node membership table (control plane): each node upserts its own row with a
// fresh heartbeat every few seconds, and peers are the other rows with a heartbeat newer than
// the TTL. Stale rows are pruned by the leader. The TTL bounds membership staleness in BOTH
// directions: how long a silent node still counts as live, and how long the cached peer list is
// served before a re-read -- so a new node may take up to a TTL to become visible.
type Registry struct {
	pool         *db.DB
	nodeID       string
	advertiseURL string
	ttl          time.Duration
	peers        []Peer // cached peer list
	peersFetched time.Time
	mu           sync.Mutex // Protects peers and peersFetched
}

// New creates the registry table if it does not exist and registers this node.
func New(pool *db.DB, nodeID, advertiseURL string, ttl time.Duration) (*Registry, error) {
	r := &Registry{
		pool:         pool,
		nodeID:       nodeID,
		advertiseURL: advertiseURL,
		ttl:          ttl,
	}
	if err := r.createTable(); err != nil {
		return nil, err
	}
	if err := r.Register(); err != nil {
		return nil, err
	}
	return r, nil
}

// Register upserts this node into the registry with a fresh heartbeat. It is a pure write: it
// does not touch the peer cache, because our own row is excluded from Peers() anyway.
func (r *Registry) Register() error {
	_, err := r.pool.Exec(upsertNodeQuery, r.nodeID, r.advertiseURL, time.Now().Unix())
	return err
}

// Peers returns the current set of live peer nodes (all registry rows with a fresh heartbeat,
// excluding this node), cached for the TTL.
func (r *Registry) Peers() ([]Peer, error) {
	r.mu.Lock()
	if r.peers != nil && time.Since(r.peersFetched) < r.ttl {
		peers := r.peers
		r.mu.Unlock()
		return peers, nil
	}
	r.mu.Unlock()
	peers, err := r.queryPeers()
	if err != nil {
		// Serve the last-known peer list during database hiccups: fan-out keeps flowing to
		// known peers instead of erroring (and logging) once per published message for the
		// duration of the outage. Dead peers in the stale list only cost failed sends.
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.peers != nil {
			return r.peers, nil
		}
		return nil, err
	}
	r.mu.Lock()
	r.peers = peers
	r.peersFetched = time.Now()
	r.mu.Unlock()
	return peers, nil
}

// Prune deletes registry rows whose heartbeat is long expired. Only the leader calls this; the
// grace period of 3x the TTL avoids deleting rows of nodes that are merely slow to heartbeat.
func (r *Registry) Prune() error {
	_, err := r.pool.Exec(pruneStaleNodesQuery, time.Now().Add(-3*r.ttl).Unix())
	return err
}

// Deregister deletes this node's registry row; called on shutdown.
func (r *Registry) Deregister() error {
	_, err := r.pool.Exec(deleteNodeQuery, r.nodeID)
	return err
}

// createTable creates the registry table, serialized via a transaction-scoped advisory lock:
// CREATE TABLE IF NOT EXISTS is not atomic in PostgreSQL, so multiple nodes cold-booting
// concurrently on a fresh database would otherwise race and crash with a duplicate-key error
// on pg_class.
func (r *Registry) createTable() error {
	tx, err := r.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(tryAdvisoryXactLockQuery, schemaLockKey); err != nil {
		return err
	}
	if _, err := tx.Exec(createTableQuery); err != nil {
		return err
	}
	return tx.Commit()
}

// queryPeers reads the current live peer set from the registry table.
func (r *Registry) queryPeers() ([]Peer, error) {
	cutoff := time.Now().Add(-r.ttl).Unix()
	rows, err := r.pool.Query(selectPeersQuery, cutoff, r.nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	peers := make([]Peer, 0)
	for rows.Next() {
		var p Peer
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
