package server

import (
	"context"
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
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

// deliverFromBus delivers a message received from a peer node (via the cluster) to this
// node's local subscribers. It is the receive-side counterpart to Cluster.ForwardMessage: local
// delivery and all global side effects (Firebase, email, web push, upstream) already ran on the
// origin node, so this only publishes to the local topic, and never re-relays.
//
// A message whose topic has no subscriber here is counted as wasted and not published: publishing
// would keep the idle topic alive (Keepalive), so a node would never expunge a topic the other
// nodes keep publishing to. A subscriber that comes along later gets it from its since= read of
// the cache, or from the tail (which sends what never went out here).
func (s *Server) deliverFromBus(m *model.Message) {
	s.mu.RLock()
	t, ok := s.topics[m.Topic]
	s.mu.RUnlock()
	if !ok || t.SubscribersCount() == 0 {
		metrics.ClusterMessagesWasted.Inc() // Relayed here needlessly: no subscriber here wanted the message
		return
	}
	v := s.visitor(m.Sender, nil)
	if err := t.Publish(v, m); err != nil {
		logvm(v, m).Err(err).Warn("Cluster: unable to deliver fan-out message to local subscribers")
	}
}

// maybeCloseLocalSubscribers closes every subscriber connection on this node. The cluster calls it
// while this node is isolated (lost its registration, peers healthy): peers no longer forward
// to it, so its subscribers would silently receive nothing; they reconnect to a healthy node.
//
// Not while the tail still reads the database: then the subscribers get every message anyway,
// a little late. Heartbeats time out when the database is slow, not only when it is gone, and
// closing tens of thousands of connections then sends them to nodes that are just as slow, as
// a reconnect storm that keeps the database slow.
func (s *Server) maybeCloseLocalSubscribers() {
	if s.tail != nil && time.Since(s.tail.LastRead()) < tailStaleAfter {
		log.Tag(tagTail).Info("Keeping local subscribers although the cluster registration is stale: the tail still reads the database")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.topics {
		t.CancelAllSubscribers()
	}
}

// testHookBucketGossip, if non-nil, is called by gossipBuckets between taking a round and sending
// it, so tests can stop a node at that point; it is always nil in production
var testHookBucketGossip func(s *Server, round *limit.BucketRound)

// restoreBuckets takes the visitors' buckets over from the live peers before this node serves.
// Every peer holds the whole cluster's buckets (see limit.Service.ChargeBuckets), so a rolling
// restart, or a crash, forgives nothing as long as one node is up; only what this node used in its
// last gossip round before it went down is lost.
func (s *Server) restoreBuckets() {
	ctx, cancel := context.WithTimeout(context.Background(), bucketRestoreTimeout)
	defer cancel()
	for origin, snapshot := range s.cluster.PeerBuckets(ctx) {
		s.limiter.RestoreBuckets(string(origin), snapshot)
	}
}

// chargePeerBuckets charges what a peer used of its visitors' buckets into this node's (see
// limit.Service.ChargeBuckets)
func (s *Server) chargePeerBuckets(origin cluster.NodeID, round *limit.BucketRound) {
	s.limiter.ChargeBuckets(string(origin), round)
}

// runBucketGossip sends the peers what this node used of its visitors' buckets every
// bucketGossipInterval, until stop ends it (before the last round, see stop)
func (s *Server) runBucketGossip() {
	defer s.bucketGossip.Done()
	for {
		select {
		case <-time.After(bucketGossipInterval):
			s.gossipBuckets()
		case <-s.bucketGossipStop:
			return
		}
	}
}

// gossipBuckets sends the peers what this node used of its visitors' buckets since the last round
// (see limit.Service.BucketDeltas), which they charge into their own buckets, so a client spread
// over the nodes gets its limits once, not once per node
func (s *Server) gossipBuckets() {
	round := s.limiter.BucketDeltas()
	if testHookBucketGossip != nil {
		testHookBucketGossip(s, round)
	}
	s.cluster.BroadcastBuckets(round)
}
