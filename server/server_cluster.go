package server

import (
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

// closeLocalSubscribers closes every subscriber connection on this node. The cluster calls it
// while this node is isolated (lost its registration, peers healthy): peers no longer forward
// to it, so its subscribers would silently receive nothing; they reconnect to a healthy node.
//
// Not while the tail still reads the database: then the subscribers get every message anyway,
// a little late. Heartbeats time out when the database is slow, not only when it is gone, and
// closing tens of thousands of connections then sends them to nodes that are just as slow, as
// a reconnect storm that keeps the database slow.
func (s *Server) closeLocalSubscribers() {
	if readAt := s.tailReadAt.Load(); readAt > 0 && time.Since(time.Unix(readAt, 0)) < tailStaleAfter {
		log.Tag(tagTail).Info("Keeping local subscribers although the cluster registration is stale: the tail still reads the database")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.topics {
		t.CancelAllSubscribers()
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
