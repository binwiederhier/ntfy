package cluster

import (
	"context"
	"encoding/json"
	"time"

	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
)

const (
	// bucketGossipInterval is how often a node sends its peers what it used of its visitors' buckets.
	// A client spread over the nodes gets at most about one burst per node more than its limit
	// before the peers know, once; after that, the cluster admits its configured rate.
	bucketGossipInterval = time.Second

	// bucketRestoreTimeout bounds how long a starting node waits for its peers' buckets; a peer that
	// has not answered by then is left out, and its buckets are forgiven (fail open)
	bucketRestoreTimeout = 10 * time.Second
)

// testHookBucketGossip, if non-nil, is called by gossipBuckets between taking a round and sending
// it, so tests can close a node at that point; it is always nil in production
var testHookBucketGossip func(c *meshCluster, round *limit.Buckets)

// restoreBuckets takes the visitors' buckets over from the live peers before this node serves.
// Every peer holds the whole cluster's buckets (see limit.Service.ChargeBuckets), so a rolling
// restart, or a crash, forgives nothing as long as one node is up; only what this node used in its
// last gossip round before it went down is lost.
func (c *meshCluster) restoreBuckets() {
	ctx, cancel := context.WithTimeout(c.ctx, bucketRestoreTimeout)
	defer cancel()
	for origin, snapshot := range c.peerBuckets(ctx) {
		c.conf.Limiter.RestoreBuckets(string(origin), snapshot)
	}
}

// runBucketGossip sends the peers what this node used of its visitors' buckets every
// bucketGossipInterval, until Close ends it (before the last round, see stopBucketGossip)
func (c *meshCluster) runBucketGossip() {
	defer c.bucketGossip.Done()
	for {
		select {
		case <-time.After(bucketGossipInterval):
			c.gossipBuckets()
		case <-c.bucketGossipStop:
			return
		}
	}
}

// stopBucketGossip ends the gossip loop and sends the last round. The server closes its listeners
// before it closes the cluster; a request still running (closing a listener does not wait for its
// handlers) may use a bucket after the last round, which the peers then never learn of; draining
// (see SetDraining) leaves few such requests.
func (c *meshCluster) stopBucketGossip() {
	close(c.bucketGossipStop)
	c.bucketGossip.Wait() // Its round, if it took one, is sent before the last one
	c.gossipBuckets()
}

// gossipBuckets sends the peers what this node used of its visitors' buckets since the last round
// (see limit.Service.BucketDeltas), which they charge into their own buckets, so a client spread
// over the nodes gets its limits once, not once per node
func (c *meshCluster) gossipBuckets() {
	round := c.conf.Limiter.BucketDeltas()
	if testHookBucketGossip != nil {
		testHookBucketGossip(c, round)
	}
	c.broadcastBuckets(round)
}

// broadcastBuckets sends every live peer the round of buckets. A failed send is not retried: the peer undercounts that round, and a
// peer that was down catches up from a snapshot when it starts.
func (c *meshCluster) broadcastBuckets(round *limit.Buckets) {
	peers := c.registry.Peers()
	if len(round.Usages) == 0 || len(peers) == 0 {
		return
	}
	body, err := json.Marshal(round)
	if err != nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return // Close is waiting for the sends, or done; a send added now would race it
	}
	c.bucketSends.Add(len(peers))
	c.mu.Unlock()
	for _, p := range peers {
		go func(p *registry.Peer) {
			defer c.bucketSends.Done()
			ctx, cancel := context.WithTimeout(c.ctx, bucketsSendTimeout)
			defer cancel()
			if err := c.client.SendBuckets(ctx, p.AdvertiseURL, body); err != nil && c.ctx.Err() == nil {
				metrics.ClusterSendErrors.WithLabelValues(BucketsPath).Inc()            // Apart from the message and state paths, which alert: every restart of a peer fails a few rounds
				log.Tag(tag).Err(err).Debug("Cannot send buckets to peer %s", p.NodeID) // Every second while it is down, and expected while it restarts
			}
		}(p)
	}
}

// peerBuckets reads the buckets that are not full from every live peer, in parallel: one per peer
// that answered before ctx is done
func (c *meshCluster) peerBuckets(ctx context.Context) map[NodeID]*limit.Buckets {
	peers := c.registry.Peers()
	results := make(chan *peerSnapshot, len(peers))
	for _, p := range peers {
		go func(p *registry.Peer) {
			snapshot, err := c.client.Buckets(ctx, p.AdvertiseURL)
			if err != nil {
				log.Tag(tag).Err(err).Warn("Cannot read buckets from peer %s", p.NodeID)
				results <- nil
				return
			}
			log.Tag(tag).Info("Read buckets of %d visitor key(s) from peer %s", len(snapshot.Usages), p.NodeID)
			results <- &peerSnapshot{
				nodeID:   NodeID(p.NodeID),
				snapshot: snapshot,
			}
		}(p)
	}
	snapshots := make(map[NodeID]*limit.Buckets)
	for range peers {
		if r := <-results; r != nil {
			snapshots[r.nodeID] = r.snapshot
		}
	}
	return snapshots
}
