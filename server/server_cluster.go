package server

import (
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

// topicAnnouncer returns the first-subscriber hook for a topic: it tells peer nodes that this
// node now has a subscriber for it, so they can drop a cached rate-visitor miss (see
// Cluster.BroadcastState). Delivery does not depend on it; every message goes to every peer.
func (s *Server) topicAnnouncer(id string) func() {
	return func() {
		s.cluster.BroadcastState(&cluster.State{AddedTopics: []string{id}})
	}
}

// clearRateVisitorMisses drops cached rate-visitor misses for topics that just gained a subscriber
// on a peer. The peer persists the rate visitor before announcing the topic, so the next publish
// here finds it instead of answering 507 from the stale miss for up to rateVisitorMissTTL.
func (s *Server) clearRateVisitorMisses(topics []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range topics {
		if t, ok := s.topics[id]; ok {
			t.ClearRateVisitorMiss()
		}
	}
}

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
