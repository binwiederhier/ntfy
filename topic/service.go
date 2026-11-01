// Package topic keeps the cluster-wide state of the UnifiedPush topics: when each was first seen,
// when a subscriber was last active, and who pays for its pushes (its rate visitor). Every node
// keeps it in memory and syncs it through the store, merging by per-field rules (earliest first
// observation, latest subscriber activity, last written rate visitor), so writes may land in any
// order. Timestamps are the nodes' clocks, so the nodes must keep them in sync (NTP).
//
// The live topics (server.topic and the server's topics map) are meant to move here later.
package topic

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tag = "topic"

	// syncInterval is how often a node writes its changed topics and reads the others'
	syncInterval = 5 * time.Second

	// statementTimeout bounds each store call. A row may commit up to that long after its
	// updated_at, so reads re-read readOverlap behind the watermark; re-reading is harmless.
	statementTimeout = 5 * time.Second
	readOverlap      = statementTimeout + time.Second
	loadTimeout      = 2 * time.Minute

	// pruneAfter must stay well above the server's 410 threshold (7 days), so the row that
	// starts the countdown is still there when it is reached
	pruneInterval = time.Hour
	pruneAfter    = 14 * 24 * time.Hour

	// keepaliveInterval is how often an open stream writes its subscriber activity at most
	keepaliveInterval = 5 * time.Minute

	// maxNoted caps the topics known only from refused pushes, so a push flood cannot fill memory
	maxNoted = 100_000
)

// Service keeps the topics' state in memory and syncs it through the store
type Service struct {
	store     *Store            // Nil: memory only
	now       func() time.Time  // Injectable clock, for tests
	interval  time.Duration     // How often the worker syncs; tests change it
	entries   map[string]*State // By topic
	pending   map[string]State  // Rows to write, by topic; always empty without a store
	noted     int               // Entries without subscriber activity, capped at maxNoted
	watermark int64             // Time of the last read; the worker's only
	prunedAt  int64             // Time of the last prune; the worker's only
	ctx       context.Context   // Done on Close, so Close never waits on a stuck store call
	cancel    context.CancelFunc
	closeOnce sync.Once
	wg        sync.WaitGroup
	mu        sync.Mutex // Protects entries, pending and noted
}

// New creates the service
func New(conf *Config) *Service {
	return newService(conf.Store, time.Now)
}

func newService(store *Store, now func() time.Time) *Service {
	s := &Service{
		store:    store,
		now:      now,
		interval: syncInterval,
		entries:  make(map[string]*State),
		pending:  make(map[string]State),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start loads the rows from the store, if there is one, and starts the sync worker
func (s *Service) Start(ctx context.Context) error {
	if s.store != nil {
		ctx, cancel := context.WithTimeout(ctx, loadTimeout)
		defer cancel()
		started := time.Now()
		now := s.now().Unix()
		n, err := s.read(ctx, now-int64(pruneAfter/time.Second))
		if err != nil {
			return fmt.Errorf("cannot load topics: %w", err)
		}
		s.watermark, s.prunedAt = now, now
		log.Tag(tag).Info("Loaded %d topic(s) in %v", n, time.Since(started).Round(time.Millisecond))
	}
	s.wg.Add(1)
	go s.runSync()
	return nil
}

// Close stops the sync worker and writes what is unwritten; the store's database is its owner's to close
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
		if s.store == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), statementTimeout)
		defer cancel()
		s.write(ctx)
	})
}

// SetRateVisitor makes the subscriber the topic's rate visitor and records subscriber activity
// now; the last one set, on any node, wins
func (s *Service) SetRateVisitor(topic, userID, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.touchNoLock(topic)
	e.RateVisitorUserID, e.RateVisitorIP = userID, ip
	s.addPendingNoLock(e, true)
}

// Touch records subscriber activity on the topic now, keeping its rate visitor
func (s *Service) Touch(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addPendingNoLock(s.touchNoLock(topic), false)
}

// Keepalive records that a stream is open on the topic, as a Touch at most once per keepaliveInterval
func (s *Service) Keepalive(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[topic]; ok && s.now().Unix()-e.SubscribedAt < int64(keepaliveInterval/time.Second) {
		return
	}
	s.addPendingNoLock(s.touchNoLock(topic), false)
}

// Note records a refused push and returns the topic's state. An unknown topic becomes known as
// first seen now (unless maxNoted are, then it stays unknown); a known one is left alone, since
// a push is not subscriber activity.
func (s *Service) Note(topic string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[topic]; !ok && s.noted < maxNoted {
		now := s.now().Unix()
		e := &State{
			Topic:       topic,
			FirstSeenAt: now,
			UpdatedAt:   now,
		}
		s.entries[topic] = e
		s.noted++
		s.addPendingNoLock(e, false)
	}
	return s.stateNoLock(topic)
}

// State returns a copy of the topic's state from memory, or nil if it is unknown
func (s *Service) State(topic string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateNoLock(topic)
}

// Refresh reads the topic's row from the store, merges it into memory, and returns the state like
// State. It is for decisions too final to make from a stale row; without a store it is State.
func (s *Service) Refresh(ctx context.Context, topic string) (*State, error) {
	if s.store == nil {
		return s.State(topic), nil
	}
	r, err := s.store.readOne(ctx, topic)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r != nil {
		s.applyNoLock(r)
	}
	return s.stateNoLock(topic), nil
}

// Size returns the number of topics the service knows
func (s *Service) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// runSync syncs every interval until Close; it is the only writer, so writes and reads never interleave
func (s *Service) runSync() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sync()
		case <-s.ctx.Done():
			return
		}
	}
}

// sync writes the pending rows, reads back the rows changed since the last round, and prunes once
// in a while. Errors are logged and retried in the next round.
func (s *Service) sync() {
	now := s.now().Unix()
	if s.store != nil {
		ctx, cancel := context.WithTimeout(s.ctx, statementTimeout)
		s.write(ctx)
		cancel()
		ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
		_, err := s.read(ctx, s.watermark-int64(readOverlap/time.Second))
		cancel()
		if err != nil {
			log.Tag(tag).Err(err).Warn("Cannot read topics; those of other nodes are stale until the next round")
		} else {
			s.watermark = now
		}
	}
	if now-s.prunedAt < int64(pruneInterval/time.Second) {
		return
	}
	s.prunedAt = now
	before := now - int64(pruneAfter/time.Second)
	s.prune(before)
	if s.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, statementTimeout)
	deleted, err := s.store.prune(ctx, before)
	cancel()
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot prune topics")
	} else {
		log.Tag(tag).Debug("Pruned %d topic row(s)", deleted)
	}
}

// write writes the pending rows outside the lock. Failed rows are pending again unless the topic
// changed meanwhile; rows are state, not deltas, so rewriting them is harmless.
func (s *Service) write(ctx context.Context) {
	s.mu.Lock()
	rows := slices.Collect(maps.Values(s.pending))
	clear(s.pending)
	s.mu.Unlock()
	if len(rows) == 0 {
		return
	}
	now := s.now().Unix()
	err := s.store.write(ctx, rows, now)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write %d topic(s); retrying in the next round", len(rows))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		if p, ok := s.pending[r.Topic]; ok && err != nil && p.RateVisitorIP == "" && r.RateVisitorIP != "" {
			p.RateVisitorUserID, p.RateVisitorIP = r.RateVisitorUserID, r.RateVisitorIP // A touch since then does not drop it
			s.pending[r.Topic] = p
		} else if ok {
			continue
		} else if err != nil {
			s.pending[r.Topic] = r
		} else if e, ok := s.entries[r.Topic]; ok {
			e.UpdatedAt = max(e.UpdatedAt, now)
		}
	}
}

// read merges the rows written since the given time into memory, and returns how many it read
func (s *Service) read(ctx context.Context, since int64) (int, error) {
	n := 0
	err := s.store.readSince(ctx, since, func(r *State) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.applyNoLock(r)
		n++
	})
	return n, err
}

// applyNoLock merges a row read from the store into memory by the store's rules, except that a
// rate visitor set here and not written yet is newer than the stored one. The caller must hold s.mu.
func (s *Service) applyNoLock(r *State) {
	e, ok := s.entries[r.Topic]
	if !ok {
		e = &State{
			Topic:       r.Topic,
			FirstSeenAt: r.FirstSeenAt,
		}
		s.entries[r.Topic] = e
		s.noted++
	}
	if e.SubscribedAt == 0 && r.SubscribedAt != 0 {
		s.noted--
	}
	e.FirstSeenAt = min(e.FirstSeenAt, r.FirstSeenAt)
	e.SubscribedAt = max(e.SubscribedAt, r.SubscribedAt)
	e.UpdatedAt = max(e.UpdatedAt, r.UpdatedAt)
	if p, pending := s.pending[r.Topic]; r.RateVisitorIP != "" && (!pending || p.RateVisitorIP == "") {
		e.RateVisitorUserID, e.RateVisitorIP = r.RateVisitorUserID, r.RateVisitorIP
	}
}

// prune forgets the topics not written since the given time, along with their unwritten changes
func (s *Service) prune(before int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noted = 0
	for topic, e := range s.entries {
		if e.UpdatedAt < before {
			delete(s.entries, topic)
			delete(s.pending, topic)
		} else if e.SubscribedAt == 0 {
			s.noted++
		}
	}
}

// touchNoLock records subscriber activity on the topic now, creating its entry if needed. The
// caller must hold s.mu.
func (s *Service) touchNoLock(topic string) *State {
	now := s.now().Unix()
	e, ok := s.entries[topic]
	if !ok {
		e = &State{
			Topic:       topic,
			FirstSeenAt: now,
		}
		s.entries[topic] = e
	} else if e.SubscribedAt == 0 {
		s.noted-- // Known from pushes only until now
	}
	e.SubscribedAt = max(e.SubscribedAt, now)
	e.UpdatedAt = max(e.UpdatedAt, now)
	return e
}

// addPendingNoLock has the entry's row written. Unless a rate visitor is set here, the row has none,
// so it keeps the stored one, which may be newer. Without a store nothing is ever written. The
// caller must hold s.mu.
func (s *Service) addPendingNoLock(e *State, rateVisitor bool) {
	if s.store == nil {
		return
	}
	r := *e
	if p, ok := s.pending[e.Topic]; !rateVisitor && (!ok || p.RateVisitorIP == "") {
		r.RateVisitorUserID, r.RateVisitorIP = "", ""
	}
	s.pending[e.Topic] = r
}

// stateNoLock returns a copy of the topic's state, or nil if it is unknown. The caller must hold s.mu.
func (s *Service) stateNoLock(topic string) *State {
	e, ok := s.entries[topic]
	if !ok {
		return nil
	}
	c := *e
	return &c
}
