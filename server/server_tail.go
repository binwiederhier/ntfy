package server

import (
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/metrics"
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
				v := s.visitor(m.Sender, nil) // Like deliverFromBus; logging a failed send needs it
				metrics.TailMessagesMissed.Add(float64(t.PublishMissed(v, m)))
			}
		}
	}
}
