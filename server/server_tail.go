package server

import (
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
		s.publishMissed(messages)
	}
}

// publishMissed hands the messages to their topics, in write order per topic
func (s *Server) publishMissed(messages []*model.Message) {
	byTopic := make(map[string][]*model.Message)
	for _, m := range messages {
		byTopic[m.Topic] = append(byTopic[m.Topic], m)
	}
	for topicID, topicMessages := range byTopic {
		s.mu.RLock()
		t, ok := s.topics[topicID]
		s.mu.RUnlock()
		if ok {
			metrics.TailMessagesMissed.Add(float64(t.PublishMissed(topicMessages)))
		}
	}
}
