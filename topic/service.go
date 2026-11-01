// Package topic keeps the cluster-wide state of a topic that every node needs but no node owns:
// when it was first seen, when a subscriber was last active, and who pays for the pushes to it
// (its rate visitor; see package server for the UnifiedPush rules built on it). Each is a
// per-topic fact with a merge rule, so the nodes can write it in any order: the earliest first
// observation, the latest subscriber activity, and the last written rate visitor.
//
// With a store, a sync worker writes the topics changed here (a dirty set, so a round costs
// what changed, not the table) every syncInterval and reads back the rows changed since the last
// round, applying the same merge rules in memory; at startup it loads every row. A publish
// never reads the database (State is memory only); Refresh is the one cold-path read, for a
// decision too final to make from a stale row. Rows not written for pruneAfter are pruned from
// the table and from memory alike.
//
// The worker is the service's own, not the usage counters' (package limit): a stuck topic sync
// must not stall the daily limits. Timestamps are the node's clock (unix seconds), so the nodes
// must keep their clocks in sync (NTP); a skew beyond readOverlap may skip rows in the read-back.
//
// The live topics of a node (its subscribers and the delivery to them, server.topic and the
// server's topics map today) are meant to move into this package too, as topic.Topic and
// topic.Subscriber, with Subscribe and Unsubscribe calling SetRateVisitor, Touch and Keepalive
// themselves. Until then, put new per-topic state here rather than in the server.
package topic

import (
	"context"
	"slices"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tag = "topic"

	// syncInterval is how often a node writes its changed topics and reads the others'; a change
	// reaches the other nodes within about two intervals (one to be written, one to be read)
	syncInterval = 5 * time.Second

	// statementTimeout bounds each store call of a sync. A row's updated_at is the writing node's
	// time before its statement, so it may commit up to this much later; reads re-read readOverlap
	// behind the watermark so such a row (or one stamped by a node clock a little behind ours) is
	// not skipped (the merge rules make a re-read harmless).
	statementTimeout = 5 * time.Second
	readOverlap      = statementTimeout + time.Second

	// loadWait is how long Start waits for the startup load before serving from memory, while
	// the load goes on in the background; loadTimeout bounds one load attempt
	loadWait    = 30 * time.Second
	loadTimeout = 2 * time.Minute

	// pruneInterval is how often a node deletes rows not written for pruneAfter, from the table
	// and from memory. pruneAfter must be well above the server's 404 threshold for an abandoned
	// UnifiedPush topic (7 days), so the row that starts the countdown is still there when the
	// threshold is reached: a pruned row restarts the countdown on the next push, so a topic
	// pushed to less often than every pruneAfter never gets its 404.
	pruneInterval = time.Hour
	pruneAfter    = 14 * 24 * time.Hour

	// keepaliveInterval is how often an open stream's subscriber activity is recorded at most
	// (see Keepalive), so a long-lived stream costs one write per interval, not one per call
	keepaliveInterval = 5 * time.Minute

	// maxNoted caps the topics known only from refused pushes (see Note), so a flood of pushes to
	// topics nobody subscribed to cannot fill the memory of every node, or the table. Such a topic
	// is only an entry of ~100 bytes, but it is kept for pruneAfter.
	maxNoted = 100_000
)

// Service keeps the topics' state in memory and syncs it through the store; see the package comment
type Service struct {
	conf      *Config
	now       func() time.Time    // Injectable clock, for tests
	store     *Store              // Nil: memory only
	entries   map[string]*entry   // By topic
	dirty     map[string]struct{} // Topics with an unwritten change
	noted     int                 // Entries without subscriber activity so far, capped at maxNoted
	watermark int64               // Time of the last read, unix seconds; the sync worker's only
	prunedAt  int64               // Time of the last prune, unix seconds; the sync worker's only
	interval  time.Duration       // How often the worker syncs (syncInterval); tests change it
	ctx       context.Context     // Done on Close; the sync worker's store calls derive from it, so Close never waits on a stuck one
	cancel    context.CancelFunc
	closeOnce sync.Once
	wg        sync.WaitGroup
	mu        sync.Mutex // Protects entries, dirty and noted
}

// New creates the service
func New(conf *Config) *Service {
	return newService(conf, time.Now)
}

func newService(conf *Config, now func() time.Time) *Service {
	s := &Service{
		conf:     conf,
		now:      now,
		store:    conf.Store,
		entries:  make(map[string]*entry),
		dirty:    make(map[string]struct{}),
		interval: syncInterval,
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start loads the current rows from the store, if there is one, and starts the sync worker. It
// returns once the load is done, or after loadWait (or when ctx is done), whichever is first;
// the load then goes on in the background while the service serves from memory.
func (s *Service) Start(ctx context.Context) error {
	loaded := make(chan struct{})
	s.wg.Add(1)
	go s.runSync(loaded)
	if s.store == nil {
		return nil
	}
	select {
	case <-loaded:
	case <-time.After(loadWait):
		log.Tag(tag).Warn("Loading topics takes longer than %v; serving from memory, loading in the background", loadWait)
	case <-ctx.Done():
		log.Tag(tag).Warn("Loading topics was not done in time; serving from memory, loading in the background")
	}
	return nil
}

// Close stops the sync worker, cancelling its store call (a stuck load or sync), and writes what
// is unwritten. The store's database is its owner's to close (the user manager's, or the shared
// pool), after this.
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

// SetRateVisitor makes rv the topic's rate visitor and records subscriber activity now. The last
// one set, on any node, wins.
func (s *Service) SetRateVisitor(topic string, rv *RateVisitor) {
	now := s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.touchNoLock(topic, now)
	e.userID, e.ip = rv.UserID, rv.IP
	e.write = writeRateVisitor
}

// Touch records subscriber activity on the topic now (a subscribe or poll by someone who may not
// pay, or a stream that closed), keeping its rate visitor
func (s *Service) Touch(topic string) {
	now := s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touchNoLock(topic, now)
}

// Keepalive records that a stream is still open on the topic, as a Touch at most once per
// keepaliveInterval, so the other nodes see the subscriber as active up to that much late
func (s *Service) Keepalive(topic string) {
	now := s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[topic]; ok && now-e.subscribedAt < int64(keepaliveInterval/time.Second) {
		return
	}
	s.touchNoLock(topic, now)
}

// Note makes a topic this node does not know yet known, as first seen now, e.g. on a refused
// push, so that an abandoned topic can be told apart from a new one. It is insert-only: a known
// topic is left alone, so a push never counts as subscriber activity and never revives a
// rate visitor. At most maxNoted such topics are kept.
func (s *Service) Note(topic string) {
	now := s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[topic]; ok || s.noted >= maxNoted {
		return
	}
	s.entries[topic] = &entry{
		firstSeenAt: now,
		updatedAt:   now,
		write:       writeTouch,
	}
	s.dirty[topic] = struct{}{}
	s.noted++
}

// State returns the topic's state as this node knows it, from memory, or nil if it is unknown
func (s *Service) State(topic string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateNoLock(topic)
}

// Refresh reads the topic's row from the store (a primary key read, with the caller's context),
// merges it into memory, and returns the topic's state like State. It is for a decision too final
// to make from a stale row, on a cold path only. Without a store it is State.
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

// RateVisitorUserIDs returns the distinct user IDs of the rate visitors of the topics with
// subscriber activity since the given time, sorted, e.g. to refresh their users' tiers
func (s *Service) RateVisitorUserIDs(since time.Time) []string {
	cutoff := since.Unix()
	ids := make(map[string]struct{})
	s.mu.Lock()
	for _, e := range s.entries {
		if e.userID != "" && e.subscribedAt >= cutoff {
			ids[e.userID] = struct{}{}
		}
	}
	s.mu.Unlock()
	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	slices.Sort(sorted)
	return sorted
}

// Size returns the number of topics the service knows
func (s *Service) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// SetForTest replaces the topic's state with the given one, as if it had been observed here, and
// has it written. It exists so tests can age a topic; it is not used in production code.
func (s *Service) SetForTest(topic string, t *State) {
	now := s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entryNoLock(topic, now)
	e.firstSeenAt, e.subscribedAt = t.FirstSeenAt.Unix(), 0
	if !t.SubscribedAt.IsZero() {
		e.subscribedAt = t.SubscribedAt.Unix()
	}
	e.userID, e.ip = "", ""
	if t.RateVisitor != nil {
		e.userID, e.ip = t.RateVisitor.UserID, t.RateVisitor.IP
	}
	e.updatedAt = now
	e.write = max(e.write, writeTouch)
	s.dirty[topic] = struct{}{}
	s.countNotedNoLock()
}

// runSync loads the current rows (if there is a store), retrying until it succeeds, then syncs
// every interval until Close. It is the only caller of the store's writes, so writes and reads
// never interleave.
func (s *Service) runSync(loaded chan struct{}) {
	defer s.wg.Done()
	for s.store != nil && !s.load() {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(s.interval):
		}
	}
	close(loaded)
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

// load reads every row not pruned yet, and reports whether it succeeded
func (s *Service) load() bool {
	ctx, cancel := context.WithTimeout(s.ctx, loadTimeout)
	defer cancel()
	started := time.Now()
	now := s.now().Unix()
	n, err := s.read(ctx, now-int64(pruneAfter/time.Second))
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot load topics; retrying in %v", s.interval)
		return false
	}
	s.watermark, s.prunedAt = now, now
	log.Tag(tag).Info("Loaded %d topic(s) in %v", n, time.Since(started).Round(time.Millisecond))
	return true
}

// sync is one round of the sync worker: write the topics changed here, read back the rows changed
// since the last round, and prune once in a while, in memory and (with a store) in the table.
// Every error is logged and leaves the state as it was, for the next round.
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

// write writes the topics changed here, as full rows. A failed write marks them changed again,
// for the next round; rows are state, not deltas, so rewriting the ones that were written is
// harmless.
func (s *Service) write(ctx context.Context) {
	rows := s.dirtyRows()
	if len(rows) == 0 {
		return
	}
	now := s.now().Unix()
	if err := s.store.write(ctx, rows, now); err != nil {
		s.markDirty(rows)
		log.Tag(tag).Err(err).Warn("Cannot write %d topic(s); retrying in the next round", len(rows))
		return
	}
	s.markWritten(rows, now)
}

// dirtyRows returns the rows of the topics changed here, and marks them written
func (s *Service) dirtyRows() []row {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]row, 0, len(s.dirty))
	for topic := range s.dirty {
		e, ok := s.entries[topic]
		if !ok {
			continue
		}
		rows = append(rows, row{
			topic:        topic,
			firstSeenAt:  e.firstSeenAt,
			subscribedAt: e.subscribedAt,
			userID:       e.userID,
			ip:           e.ip,
		})
		e.write = writeNone
	}
	clear(s.dirty)
	return rows
}

// markDirty marks the topics of rows that failed to write as changed again. A row with a rate
// visitor is marked as if it were set here, so the read-back keeps it until it is written.
func (s *Service) markDirty(rows []row) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		e, ok := s.entries[r.topic]
		if !ok {
			continue
		}
		write := writeTouch
		if r.ip != "" {
			write = writeRateVisitor
		}
		e.write = max(e.write, write)
		s.dirty[r.topic] = struct{}{}
	}
}

// markWritten stamps the written rows' topics with the write time, which the prune goes by
func (s *Service) markWritten(rows []row, now int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		if e, ok := s.entries[r.topic]; ok && e.updatedAt < now {
			e.updatedAt = now
		}
	}
}

// read applies the rows written since the given time to memory, and returns how many it applied
func (s *Service) read(ctx context.Context, since int64) (int, error) {
	applied := 0
	err := s.store.readSince(ctx, since, func(r *row) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.applyNoLock(r)
		applied++
	})
	return applied, err
}

// applyNoLock merges a row read from the store into memory, by the store's own rules: the earliest
// first observation, the latest subscriber activity, the row's rate visitor unless it has none or
// one was set here and not written yet (that one is newer). The caller must hold s.mu.
func (s *Service) applyNoLock(r *row) {
	e, ok := s.entries[r.topic]
	if !ok {
		e = &entry{firstSeenAt: r.firstSeenAt}
		s.entries[r.topic] = e
		if r.subscribedAt == 0 {
			s.noted++
		}
	} else if r.firstSeenAt < e.firstSeenAt {
		e.firstSeenAt = r.firstSeenAt
	}
	if r.subscribedAt > e.subscribedAt {
		if e.subscribedAt == 0 && ok {
			s.noted--
		}
		e.subscribedAt = r.subscribedAt
	}
	if r.ip != "" && e.write != writeRateVisitor {
		e.userID, e.ip = r.userID, r.ip
	}
	if r.updatedAt > e.updatedAt {
		e.updatedAt = r.updatedAt
	}
}

// prune forgets the topics not written since the given time (unix seconds), unless they have an
// unwritten change (without a store, nothing is ever written, so there is nothing to wait for)
func (s *Service) prune(before int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, e := range s.entries {
		if (e.write == writeNone || s.store == nil) && e.updatedAt < before {
			delete(s.entries, topic)
		}
	}
	s.countNotedNoLock()
}

// touchNoLock records subscriber activity on the topic at now, creating the entry if needed, and
// returns the entry, marked to be written. The caller must hold s.mu.
func (s *Service) touchNoLock(topic string, now int64) *entry {
	e, ok := s.entries[topic]
	if !ok {
		e = &entry{firstSeenAt: now}
		s.entries[topic] = e
	} else if e.subscribedAt == 0 {
		s.noted-- // Known from pushes only until now
	}
	e.subscribedAt = max(e.subscribedAt, now)
	e.updatedAt = max(e.updatedAt, now)
	e.write = max(e.write, writeTouch)
	s.dirty[topic] = struct{}{}
	return e
}

// entryNoLock returns the topic's entry, creating it as first seen at now if needed. The caller
// must hold s.mu.
func (s *Service) entryNoLock(topic string, now int64) *entry {
	e, ok := s.entries[topic]
	if !ok {
		e = &entry{firstSeenAt: now}
		s.entries[topic] = e
	}
	return e
}

// stateNoLock returns the topic's state, or nil if it is unknown. The caller must hold s.mu.
func (s *Service) stateNoLock(topic string) *State {
	e, ok := s.entries[topic]
	if !ok {
		return nil
	}
	t := &State{
		FirstSeenAt: time.Unix(e.firstSeenAt, 0).UTC(),
	}
	if e.subscribedAt != 0 {
		t.SubscribedAt = time.Unix(e.subscribedAt, 0).UTC()
	}
	if e.ip != "" {
		t.RateVisitor = &RateVisitor{
			UserID: e.userID,
			IP:     e.ip,
		}
	}
	return t
}

// countNotedNoLock recounts the topics known from pushes only, after a change that may have
// made or unmade several at once (a prune, SetForTest). The caller must hold s.mu.
func (s *Service) countNotedNoLock() {
	s.noted = 0
	for _, e := range s.entries {
		if e.subscribedAt == 0 {
			s.noted++
		}
	}
}
