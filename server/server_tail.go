package server

import (
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	tagTail        = "tail"
	tailInterval   = time.Second      // How often the tail reads what was written; bounds how late a missed message arrives
	tailStaleAfter = 30 * time.Second // A tail that has not read for this long cannot stand in for the fan-out (see closeLocalSubscribers)

	// unwrittenMarkerMargin is how far before an unwritten marker's delivery here its replay
	// starts: the fan-out linger, and publish times having whole seconds (see sinceForTopics)
	unwrittenMarkerMargin = 2 * time.Second
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
		s.tailReadAt.Store(time.Now().Unix())
		metrics.TailMessagesRead.Add(float64(len(messages)))
		metrics.TailMessagesLagSeconds.Set(tailLag(messages, time.Now()).Seconds())
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

// tailLag is how old the newest message of a read is; zero if there was none (caught up)
func tailLag(messages []*model.Message, now time.Time) time.Duration {
	var newest int64
	for _, m := range messages {
		newest = max(newest, m.Time)
	}
	if newest == 0 {
		return 0
	}
	return now.Sub(time.Unix(newest, 0))
}

// sinceForTopics returns the replay marker to use per topic. A since=<id> marker that is not
// in the cache was not written yet (the node that accepted it may be cut off from the database,
// its write hanging) or has expired. If the message went out on this node, the replay starts a
// little before then, right away: waiting for the write, and polling the database meanwhile,
// is what made a fenced node's clients a database storm. Only a marker nobody here knows is
// waited for, and replays the topic's whole history if it never shows up, as it always did.
func (s *Server) sinceForTopics(since model.SinceMarker) (func(t *topic) (model.SinceMarker, error), error) {
	asIs := func(*topic) (model.SinceMarker, error) { return since, nil }
	if !since.IsID() {
		return asIs, nil
	}
	written, err := s.messageCache.MarkerWritten(since.ID(), false)
	if err != nil {
		return nil, err
	}
	if written {
		return asIs, nil
	}
	return func(t *topic) (model.SinceMarker, error) {
		if at, ok := t.DeliveredAt(since.ID()); ok {
			return model.NewSinceTime(at.Add(-unwrittenMarkerMargin).Unix()), nil
		}
		written, err := s.messageCache.MarkerWritten(since.ID(), true)
		if err != nil || written {
			return since, err
		}
		return model.SinceAllMessages, nil
	}, nil
}
