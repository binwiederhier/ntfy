package limit

import (
	"context"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	// rateVisitorTouchInterval is how often a touch of a topic's rate visitor is written at most; an open
	// stream is touched about this often, so other nodes may see it up to this much older
	rateVisitorTouchInterval = 5 * time.Minute

	// rateVisitorPruneAfter is how long a rate visitor row is kept after it was last written. It must be well
	// above the server's 404 threshold for an abandoned UnifiedPush topic (7 days), so the row
	// that starts the countdown is still there when the threshold is reached.
	rateVisitorPruneAfter = 30 * oneDay
)

// SetRateVisitor makes p the topic's rate visitor; the latest subscriber wins. p.SeenAt is the node's clock.
func (s *Service) SetRateVisitor(topic string, p *RateVisitor) {
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	s.rateVisitors.entries[topic] = &rateVisitorEntry{
		key:    p.Key,
		userID: p.UserID,
		seenAt: p.SeenAt.Unix(),
		write:  rateVisitorWriteSet,
	}
}

// TouchRateVisitor marks subscriber activity on the topic now (an open or closed stream), keeping its
// rate visitor. It does nothing if the topic was seen within rateVisitorTouchInterval.
func (s *Service) TouchRateVisitor(topic string) {
	now := s.now().Unix()
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	e, ok := s.rateVisitors.entries[topic]
	if !ok {
		s.rateVisitors.addNoLock(topic, now)
		return
	} else if now-e.seenAt < int64(rateVisitorTouchInterval/time.Second) {
		return
	}
	e.seenAt = now
	e.write = max(e.write, rateVisitorWriteTouch)
}

// NoteTopic remembers a topic this node does not know yet, as seen now and without a rate visitor, e.g.
// on a refused push, so that the topic's abandonment can be told apart from a new topic. It is
// written as a touch: a row this node has not read yet (written in the last seconds, or while its
// load failed) keeps its rate visitor, and its seen_at moves to now at most.
func (s *Service) NoteTopic(topic string) {
	now := s.now().Unix()
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	if _, ok := s.rateVisitors.entries[topic]; !ok {
		s.rateVisitors.addNoLock(topic, now)
	}
}

// RateVisitor returns the topic's rate visitor, or nil if the topic is unknown
func (s *Service) RateVisitor(topic string) *RateVisitor {
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	e, ok := s.rateVisitors.entries[topic]
	if !ok {
		return nil
	}
	return &RateVisitor{
		Key:    e.key,
		UserID: e.userID,
		SeenAt: time.Unix(e.seenAt, 0).UTC(),
	}
}

// writeRateVisitors writes the rate visitors changed here. A failed write marks them changed again, for the
// next round; rewriting the ones that were written is harmless.
func (s *Service) writeRateVisitors(ctx context.Context) {
	rows := s.unwrittenRateVisitors()
	if len(rows) == 0 {
		return
	}
	if err := s.store.writeRateVisitors(ctx, rows, s.now().Unix()); err != nil {
		s.markRateVisitorsUnwritten(rows)
		log.Tag(tag).Err(err).Warn("Cannot write %d rate visitor(s); retrying in the next round", len(rows))
	}
}

// unwrittenRateVisitors returns the rows of the rate visitors changed here, and marks them written
func (s *Service) unwrittenRateVisitors() []rateVisitorRow {
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	rows := make([]rateVisitorRow, 0)
	for topic, e := range s.rateVisitors.entries {
		if e.write == rateVisitorWriteNone {
			continue
		}
		r := rateVisitorRow{
			topic:  topic,
			seenAt: e.seenAt,
		}
		if e.write == rateVisitorWriteSet {
			r.key, r.userID = e.key, e.userID
		}
		rows = append(rows, r)
		e.write = rateVisitorWriteNone
	}
	return rows
}

// markRateVisitorsUnwritten marks the rate visitors of rows that failed to write as changed again
func (s *Service) markRateVisitorsUnwritten(rows []rateVisitorRow) {
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	for _, r := range rows {
		e, ok := s.rateVisitors.entries[r.topic]
		if !ok {
			continue
		}
		write := rateVisitorWriteTouch
		if r.key != "" {
			write = rateVisitorWriteSet
		}
		e.write = max(e.write, write)
	}
}

// readRateVisitors applies the rate visitor rows written since the given time, except to entries with a change
// not written yet (that change is newer). It returns how many rows it applied.
func (s *Service) readRateVisitors(ctx context.Context, since int64) (int, error) {
	applied := 0
	err := s.store.readRateVisitorsSince(ctx, since, func(r *rateVisitorRow) {
		s.rateVisitors.mu.Lock()
		defer s.rateVisitors.mu.Unlock()
		e, ok := s.rateVisitors.entries[r.topic]
		if ok && e.write != rateVisitorWriteNone {
			return
		} else if !ok {
			e = &rateVisitorEntry{}
			s.rateVisitors.entries[r.topic] = e
		}
		e.key, e.userID, e.seenAt = r.key, r.userID, r.seenAt
		applied++
	})
	return applied, err
}

// pruneRateVisitors forgets the rate visitors not seen for rateVisitorPruneAfter
func (s *Service) pruneRateVisitors(now time.Time) {
	cutoff := now.Add(-rateVisitorPruneAfter).Unix()
	s.rateVisitors.mu.Lock()
	defer s.rateVisitors.mu.Unlock()
	for topic, e := range s.rateVisitors.entries {
		if e.seenAt < cutoff {
			delete(s.rateVisitors.entries, topic)
		}
	}
}

// addNoLock adds the topic as seen at now without a rate visitor, to be written as a touch. The caller
// must hold pt.mu.
func (pt *rateVisitorTable) addNoLock(topic string, now int64) {
	pt.entries[topic] = &rateVisitorEntry{
		seenAt: now,
		write:  rateVisitorWriteTouch,
	}
}
