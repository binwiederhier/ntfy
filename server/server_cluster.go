package server

import (
	"slices"
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	// replayMargin is added to the cache batch timeout and the fan-out linger to cover replica lag
	replayMargin = time.Second
)

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

// handleDeliveryGap repairs what a peer could not deliver to this node (see cluster.GapFunc).
// A dated report is replayed from the cache onto the connections this node already has: the
// clients' own since= markers cannot ask for the hole, because an independent newer message on
// the same topic may already have moved a marker past it (from this node, or from another peer),
// and a reconnect lands on whichever node the load balancer picks. Only reports with no position
// (a peer on an older build, or more topics than are worth enumerating) fall back to closing the
// subscribers, so their clients replay from their own markers.
func (s *Server) handleDeliveryGap(topics []string, since int64) {
	if since <= 0 || slices.Contains(topics, cluster.GapAllTopics) {
		log.Tag(tagCluster).Info("Closing the subscribers of %d gapped topic(s), so their clients replay", len(topics))
		s.closeGappedSubscribers(topics)
		return
	}
	log.Tag(tagCluster).Info("Replaying %d gapped topic(s) from %v", len(topics), time.Unix(since, 0).Format(time.RFC3339))
	go s.replayDeliveryGap(topics, since) // The peer's report must not wait for the replay
}

// replayDeliveryGap re-publishes a gapped range to the local subscribers of those topics, after
// waiting for the lost messages to be readable: they were accepted on the peer and sit in its
// cache write batch for up to replayDelay. The range is collapsed to one version per edited
// message (see newestPerSequence). Subscribers that received part of the range get those
// messages twice and dedupe by id.
func (s *Server) replayDeliveryGap(topics []string, since int64) {
	time.Sleep(s.replayDelay)
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
		messages, _, err := s.messageCache.MessagesCapped(id, marker, false, s.config.MessagePollSizeLimit)
		if err != nil {
			log.Tag(tagCluster).Err(err).Field("topic", id).Warn("Cannot read back a peer's delivery gap")
			continue
		}
		for _, m := range newestPerSequence(messages) {
			v := s.visitor(m.Sender, nil)
			if err := t.Publish(v, m); err != nil {
				logvm(v, m).Err(err).Warn("Cluster: unable to replay gapped message to local subscribers")
				continue
			}
			metrics.ClusterGapsReplayed.Inc()
		}
	}
}

// newestPerSequence keeps one message per sequence id, the newest, in the order given (oldest
// first). A gap replay lands on clients that may already hold a newer version of an edited
// message, and the clients keep whichever version they STORED last: sent an older one, they
// retire the newer one and show the older one. A normal replay is not collapsed, since a poll is
// documented to return every version.
func newestPerSequence(messages []*model.Message) []*model.Message {
	sequence := func(m *model.Message) string {
		if m.SequenceID == "" {
			return m.ID // Never edited: a sequence of one
		}
		return m.SequenceID
	}
	newest := make(map[string]*model.Message, len(messages))
	for _, m := range messages {
		newest[sequence(m)] = m // Oldest first, so the last write per sequence is the newest
	}
	out := make([]*model.Message, 0, len(newest))
	for _, m := range messages {
		if newest[sequence(m)] == m {
			out = append(out, m)
		}
	}
	return out
}

// closeGappedSubscribers closes the subscribers of the given topics, so their clients reconnect
// with since= and replay from their own markers
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

// lagSince resolves a since=<id> marker to the message's time minus replayDelay, in a cluster.
// Nodes flush their write batches independently, so row ids are not in publish order across
// nodes and a marker's row id can sit above a message published before it; replaying by time
// with that much overlap covers the skew. Clients dedupe the overlap by id. Single-node replay
// is unchanged: there is one writer, and row order is publish order.
func (s *Server) lagSince(since model.SinceMarker) model.SinceMarker {
	if s.config.ClusterListen == "" || !since.IsID() {
		return since
	}
	m, err := s.messageCache.Message(since.ID())
	if err != nil {
		return since // Unknown marker: the replay falls back to the topic's history anyway
	}
	return model.NewSinceTime(m.Time - int64(s.replayDelay.Seconds()))
}
