package server

import (
	"sync"
	"time"

	"heckel.io/ntfy/v2/model"
)

// Catch-up replay: a subscriber reconnecting with since=<marker> can miss messages that were
// fanned out live while it was away but were not yet in the database when its initial replay
// ran (cache write batch, cross-node linger, replica lag), or that were stored with a lower row
// id than the marker (nodes flush their batches independently). Once those writes have landed,
// the connection replays again from shortly before the marker and delivers only what it has not
// sent yet. Official clients also dedupe by message ID, so the overlap is safe.

const (
	// catchUpOverlap is how far before the marker message the catch-up replay starts
	catchUpOverlap = 2 * time.Second

	// catchUpMargin is added to the batch timeout and linger to cover replica lag
	catchUpMargin = time.Second
)

// replayDeduper remembers the stored messages sent on one subscriber connection until the
// catch-up replay has run, so nothing is delivered twice on that connection
type replayDeduper struct {
	seen map[string]struct{} // nil once stopped
	mu   sync.Mutex
}

func newReplayDeduper(markerID string) *replayDeduper {
	d := &replayDeduper{seen: make(map[string]struct{})}
	if markerID != "" {
		d.seen[markerID] = struct{}{} // The client has the marker message by definition
	}
	return d
}

// wrap returns a subscriber that drops messages already sent on this connection
func (d *replayDeduper) wrap(sub subscriber) subscriber {
	return func(v *visitor, m *model.Message) error {
		if m.Event != model.OpenEvent && m.Event != model.KeepaliveEvent {
			d.mu.Lock()
			if d.seen != nil {
				if _, ok := d.seen[m.ID]; ok {
					d.mu.Unlock()
					return nil
				}
				d.seen[m.ID] = struct{}{}
			}
			d.mu.Unlock()
		}
		return sub(v, m)
	}
}

// stop ends deduplication (after the catch-up replay) and releases the remembered IDs
func (d *replayDeduper) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = nil
}

// needsCatchUp reports whether a subscription with this marker gets a catch-up replay
func needsCatchUp(since model.SinceMarker) bool {
	return !since.IsNone() && !since.IsLatest()
}

// catchUpSince returns the marker for the catch-up replay: since=<id> becomes a time marker
// shortly before the marker message; all other markers are replayed as they are
func (s *Server) catchUpSince(since model.SinceMarker) model.SinceMarker {
	if since.ID() == "" {
		return since
	}
	m, err := s.messageCache.Message(since.ID())
	if err != nil {
		return since // Unknown marker: same replay as the initial one
	}
	return model.NewSinceTime(m.Time - int64(catchUpOverlap.Seconds()))
}
