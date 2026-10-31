package limit

import (
	"context"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	// payerTouchInterval is how often a touch of a topic's payer is written at most; an open
	// stream is touched about this often, so other nodes may see it up to this much older
	payerTouchInterval = 5 * time.Minute

	// payerPruneAfter is how long a payer row is kept after it was last written. It must be well
	// above the server's 404 threshold for an abandoned UnifiedPush topic (7 days), so the row
	// that starts the countdown is still there when the threshold is reached.
	payerPruneAfter = 30 * oneDay
)

// SetPayer makes p the topic's payer; the latest subscriber wins. p.SeenAt is the node's clock.
func (s *Service) SetPayer(topic string, p *Payer) {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	s.payers.entries[topic] = &payerEntry{
		key:    p.Key,
		userID: p.UserID,
		seenAt: p.SeenAt.Unix(),
		write:  payerWriteSet,
	}
}

// TouchPayer marks subscriber activity on the topic now (an open or closed stream), keeping its
// payer. It does nothing if the topic was seen within payerTouchInterval.
func (s *Service) TouchPayer(topic string) {
	now := s.now().Unix()
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	e, ok := s.payers.entries[topic]
	if !ok {
		s.payers.addNoLock(topic, now)
		return
	} else if now-e.seenAt < int64(payerTouchInterval/time.Second) {
		return
	}
	e.seenAt = now
	e.write = max(e.write, payerWriteTouch)
}

// NoteTopic remembers a topic this node does not know yet, as seen now and without a payer, e.g.
// on a refused push, so that the topic's abandonment can be told apart from a new topic. It is
// written as a touch: a row this node has not read yet (written in the last seconds, or while its
// load failed) keeps its payer, and its seen_at moves to now at most.
func (s *Service) NoteTopic(topic string) {
	now := s.now().Unix()
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	if _, ok := s.payers.entries[topic]; !ok {
		s.payers.addNoLock(topic, now)
	}
}

// Payer returns the topic's payer, or nil if the topic is unknown
func (s *Service) Payer(topic string) *Payer {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	e, ok := s.payers.entries[topic]
	if !ok {
		return nil
	}
	return &Payer{
		Key:    e.key,
		UserID: e.userID,
		SeenAt: time.Unix(e.seenAt, 0).UTC(),
	}
}

// writePayers writes the payers changed here. A failed write marks them changed again, for the
// next round; rewriting the ones that were written is harmless.
func (s *Service) writePayers(ctx context.Context) {
	rows := s.unwrittenPayers()
	if len(rows) == 0 {
		return
	}
	if err := s.store.writePayers(ctx, rows, s.now().Unix()); err != nil {
		s.markPayersUnwritten(rows)
		log.Tag(tag).Err(err).Warn("Cannot write %d payer(s); retrying in the next round", len(rows))
	}
}

// unwrittenPayers returns the rows of the payers changed here, and marks them written
func (s *Service) unwrittenPayers() []payerRow {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	rows := make([]payerRow, 0)
	for topic, e := range s.payers.entries {
		if e.write == payerWriteNone {
			continue
		}
		r := payerRow{
			topic:  topic,
			seenAt: e.seenAt,
		}
		if e.write == payerWriteSet {
			r.key, r.userID = e.key, e.userID
		}
		rows = append(rows, r)
		e.write = payerWriteNone
	}
	return rows
}

// markPayersUnwritten marks the payers of rows that failed to write as changed again
func (s *Service) markPayersUnwritten(rows []payerRow) {
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	for _, r := range rows {
		e, ok := s.payers.entries[r.topic]
		if !ok {
			continue
		}
		write := payerWriteTouch
		if r.key != "" {
			write = payerWriteSet
		}
		e.write = max(e.write, write)
	}
}

// readPayers applies the payer rows written since the given time, except to entries with a change
// not written yet (that change is newer). It returns how many rows it applied.
func (s *Service) readPayers(ctx context.Context, since int64) (int, error) {
	applied := 0
	err := s.store.readPayersSince(ctx, since, func(r *payerRow) {
		s.payers.mu.Lock()
		defer s.payers.mu.Unlock()
		e, ok := s.payers.entries[r.topic]
		if ok && e.write != payerWriteNone {
			return
		} else if !ok {
			e = &payerEntry{}
			s.payers.entries[r.topic] = e
		}
		e.key, e.userID, e.seenAt = r.key, r.userID, r.seenAt
		applied++
	})
	return applied, err
}

// prunePayers forgets the payers not seen for payerPruneAfter
func (s *Service) prunePayers(now time.Time) {
	cutoff := now.Add(-payerPruneAfter).Unix()
	s.payers.mu.Lock()
	defer s.payers.mu.Unlock()
	for topic, e := range s.payers.entries {
		if e.seenAt < cutoff {
			delete(s.payers.entries, topic)
		}
	}
}

// addNoLock adds the topic as seen at now without a payer, to be written as a touch. The caller
// must hold pt.mu.
func (pt *payerTable) addNoLock(topic string, now int64) {
	pt.entries[topic] = &payerEntry{
		seenAt: now,
		write:  payerWriteTouch,
	}
}
