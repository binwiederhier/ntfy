package server

import (
	"slices"
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

// closeLocalSubscribers closes every subscriber connection on this node. The cluster calls it
// while this node is isolated (lost its registration, peers healthy): peers no longer forward
// to it, so its subscribers would silently receive nothing; they reconnect to a healthy node.
func (s *Server) closeLocalSubscribers() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.topics {
		t.CancelAllSubscribers()
	}
}

// handleDeliveryGap repairs what a peer could not deliver to this node (see cluster.GapFunc).
// A dated gap is replayed here, because the clients cannot ask for it: one that received a newer
// message on the same topic reconnects with that newer marker, and neither the initial replay
// (higher row id) nor the catch-up window (shortly before the marker) covers the older hole.
//
// Undated, cluster-wide and long-stale gaps close the subscribers instead. A gap that went
// unreported for minutes means the peer was unreachable for minutes, so those clients received
// nothing in between and their own markers are the better source of truth.
func (s *Server) handleDeliveryGap(topics []string, since int64) {
	if since <= 0 || slices.Contains(topics, cluster.GapAllTopics) || time.Since(time.Unix(since, 0)) > gapReplayMaxAge {
		log.Tag(tagCluster).Info("Closing the subscribers of %d gapped topic(s), so their clients replay", len(topics))
		s.closeGappedSubscribers(topics)
		return
	}
	log.Tag(tagCluster).Info("Replaying %d gapped topic(s) from %v", len(topics), time.Unix(since, 0).Format(time.RFC3339))
	go s.replayDeliveryGap(topics, since) // The peer's report must not wait for the replay
}

// replayDeliveryGap re-publishes a topic's messages from the peer's oldest lost message to this
// node's local subscribers, after the origin's cache batch and fan-out linger have had time to
// land (the same wait a reconnecting subscriber's catch-up uses). Subscribers that did receive
// part of the range see those messages twice; official clients dedupe by message ID, and the
// alternative is a hole that nothing closes.
func (s *Server) replayDeliveryGap(topics []string, since int64) {
	time.Sleep(s.catchUpDelay)
	if s.stopped.Load() {
		return
	}
	marker := model.NewSinceTime(since)
	for _, id := range topics {
		s.mu.RLock()
		t, ok := s.topics[id]
		s.mu.RUnlock()
		if !ok {
			continue // Nobody here is subscribed any more, so there is nothing to repair
		}
		messages, err := s.messageCache.Messages(id, marker, false)
		if err != nil {
			log.Tag(tagCluster).Err(err).Field("topic", id).Warn("Cannot read back a peer's delivery gap")
			continue
		}
		for _, m := range messages {
			v := s.visitor(m.Sender, nil)
			if err := t.Publish(v, m); err != nil {
				logvm(v, m).Err(err).Warn("Cluster: unable to replay gapped message to local subscribers")
				continue
			}
			metrics.ClusterGapsReplayed.Inc()
		}
	}
}

// closeGappedSubscribers closes the local subscribers of gapped topics, so their clients
// reconnect and replay from the message cache with since=<last message id>. Messages published
// with cache: no cannot be replayed; cross-node delivery is best-effort for those.
func (s *Server) closeGappedSubscribers(topics []string) {
	if slices.Contains(topics, cluster.GapAllTopics) {
		s.closeLocalSubscribers()
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range topics {
		if t, ok := s.topics[id]; ok {
			t.CancelAllSubscribers()
		}
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
