package server

import (
	"time"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	// usageGossipInterval is how often a node tells its peers what its keys used of the rate
	// buckets (requests, topic creations, account actions, auth failures, attachment bytes, ban
	// strikes), see docs/10-cluster.md, section 10. It bounds how long a burst spread over the
	// nodes goes unnoticed: with the debt capped at one burst, the cluster admits N bursts once
	// and then the configured rate.
	usageGossipInterval = 2 * time.Second
)

// deliverFromBus delivers a message received from a peer node (via the cluster) to this
// node's local subscribers. It is the receive-side counterpart to Cluster.ForwardMessage: local
// delivery and all global side effects (Firebase, email, web push, upstream) already ran on the
// origin node, so this only publishes to the local topic, and never re-relays.
func (s *Server) deliverFromBus(m *model.Message) {
	s.mu.RLock()
	t, ok := s.topics[m.Topic]
	s.mu.RUnlock()
	if !ok {
		metrics.ClusterMessagesWasted.Inc() // Relayed here needlessly: this node had no use for the message
		return
	}
	v := s.visitor(m.Sender, nil)
	if err := t.Publish(v, m); err != nil {
		logvm(v, m).Err(err).Warn("Cluster: unable to deliver fan-out message to local subscribers")
	}
}

// runUsageGossip sends the rate buckets' usage deltas to the peers every usageGossipInterval
// until shutdown (cluster mode only)
func (s *Server) runUsageGossip() {
	ticker := time.NewTicker(usageGossipInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cluster.BroadcastUsage(s.limiter.TakeDeltas())
		case <-s.closeChan:
			return
		}
	}
}

// handlePeerUsage charges what a peer's keys used into this node's buckets (see
// limit.Service.Burn); the cluster calls it with every usage round it receives
func (s *Server) handlePeerUsage(deltas []limit.Delta) {
	for _, d := range deltas {
		s.limiter.Burn(d.Key, d.Kind, d.N)
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
