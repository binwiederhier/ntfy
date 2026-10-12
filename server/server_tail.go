package server

import (
	"math"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	tagTail      = "tail"
	tailInterval = time.Second // How often the tail reads what was written; bounds how late a missed message arrives
)

// runTail reads every message written to the cache, by any node, and sends it to the local
// subscribers that missed it live: its fan-out to this node was lost, or the subscriber was
// reconnecting while the message was not written yet, so the database read missed it.
func (s *Server) runTail(tail *message.Tail) {
	for {
		select {
		case <-time.After(tailInterval):
		case <-s.tailClose:
			return
		}
		messages, err := tail.Next()
		if err != nil {
			log.Tag(tagTail).Err(err).Warn("Cannot read written messages")
			continue
		}
		metrics.TailMessagesRead.Add(float64(len(messages)))
		for _, m := range messages {
			s.mu.RLock()
			t, ok := s.topics[m.Topic]
			s.mu.RUnlock()
			if ok {
				metrics.TailMessagesMissed.Add(float64(t.PublishMissed(nil, m)))
			}
		}
	}
}

// clusterSince reads a since=<id> marker as a time a little before that message in a cluster.
// Nodes write their batches independently, so a message the client never got (its fan-out was
// late) can have a lower row id than the marker. Single-node, row order is publish order.
func (s *Server) clusterSince(since model.SinceMarker) model.SinceMarker {
	if s.config.ClusterListen == "" || !since.IsID() {
		return since
	}
	m, err := s.messageCache.Message(since.ID())
	if err != nil {
		return since // Not written yet, or expired: the row id lookup handles both
	}
	lag := s.config.CacheBatchTimeout + s.config.ClusterBatchLinger + tailInterval + time.Second
	return model.NewSinceTime(m.Time - int64(math.Ceil(lag.Seconds())))
}
