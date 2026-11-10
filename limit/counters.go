package limit

import (
	"context"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
)

// counterShard is one independently locked part of the daily counters
type counterShard struct {
	counters map[string]*counter
	mu       sync.Mutex // Protects counters and everything inside them
}

// counter is one key's daily usage in its current period
type counter struct {
	periodStart int64           // When the period started (unix seconds, see periodStart)
	base        [numDaily]int64 // The key's row as last read: all nodes' usage
	unflushed   [numDaily]int64 // Admitted here since the last write
	promoted    bool            // Written to the store: reached the threshold here, or has a row
}

// runSync loads the current counts, retrying until it succeeds, then syncs every interval until
// Close. It is the only caller of the store, so writes and reads never interleave.
func (s *Service) runSync(loaded chan struct{}) {
	defer s.wg.Done()
	for !s.load() {
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

// load reads every row written in the last day and keeps those of their key's current period,
// and reports whether it succeeded
func (s *Service) load() bool {
	ctx, cancel := context.WithTimeout(s.ctx, loadTimeout)
	defer cancel()
	started := time.Now()
	now := s.now().Unix()
	n, err := s.read(ctx, now-daySeconds)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot load usage; retrying in %v", s.interval)
		return false
	}
	s.watermark, s.prunedAt = now, now
	log.Tag(tag).Info("Loaded usage of %d key(s) in %v", n, time.Since(started).Round(time.Millisecond))
	return true
}

// sync is one round of the sync worker: write the unflushed counts of the promoted keys, read
// back the rows changed since the last round, and prune once in a while. Every error is logged
// and leaves the state as it was, for the next round.
func (s *Service) sync() {
	now := s.now().Unix()
	ctx, cancel := context.WithTimeout(s.ctx, statementTimeout)
	s.write(ctx)
	cancel()
	ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
	_, err := s.read(ctx, s.watermark-int64(readOverlap/time.Second))
	cancel()
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot read usage; counts of other nodes are stale until the next round")
	} else {
		s.watermark = now
	}
	if now-s.prunedAt >= int64(pruneInterval/time.Second) {
		s.prunedAt = now
		ctx, cancel = context.WithTimeout(s.ctx, statementTimeout)
		deleted, err := s.store.prune(ctx, now-int64(pruneAfter/time.Second))
		cancel()
		if err != nil {
			log.Tag(tag).Err(err).Warn("Cannot prune usage")
		} else {
			log.Tag(tag).Debug("Pruned %d usage row(s)", deleted)
		}
	}
}

// write writes the unflushed counts of the promoted keys. A written row moves its counts from
// unflushed to base, so the total does not change and nothing is written twice; the counts of a
// failed chunk stay unflushed for the next round. A failed chunk that may have been written anyway
// (a timeout after the commit) counts as written: if it was not, its counts are lost, which fails
// open like an outage, rather than counting them twice.
func (s *Service) write(ctx context.Context) {
	rows := s.unflushedRows()
	if len(rows) == 0 {
		return
	}
	written, err := s.store.write(ctx, rows, s.now().Unix())
	for i := range rows[:written] {
		s.retire(&rows[i])
	}
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write usage, %d of %d key(s) written or maybe written; the rest is retried", written, len(rows))
	}
}

// unflushedRows returns the unflushed counts of the promoted keys, each under its counter's period
func (s *Service) unflushedRows() []usageRow {
	rows := make([]usageRow, 0)
	for _, cs := range s.counters {
		cs.mu.Lock()
		for key, c := range cs.counters {
			if c.promoted && c.unflushed != [numDaily]int64{} {
				rows = append(rows, usageRow{
					key:         key,
					periodStart: c.periodStart,
					counts:      c.unflushed,
				})
			}
		}
		cs.mu.Unlock()
	}
	return rows
}

// retire moves a written row's counts from unflushed to base. A counter that rolled over since
// was started over; its new counts are not the row's.
func (s *Service) retire(r *usageRow) {
	cs := s.counters[shardIndex(r.key)]
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.counters[r.key]
	if !ok || c.periodStart != r.periodStart {
		return
	}
	for i := 0; i < numDaily; i++ {
		c.unflushed[i] -= r.counts[i]
		c.base[i] += r.counts[i]
	}
}

// read sets the base of every key with a row of its current period written since the given
// time, creating counters as needed; a key with a row is promoted, so this node's
// counts of it are written too. It returns how many rows it applied.
func (s *Service) read(ctx context.Context, since int64) (int, error) {
	now := s.now()
	applied := 0
	err := s.store.readSince(ctx, since, func(r *usageRow) {
		start := periodStart(r.key, now)
		if r.periodStart != start {
			return // Stale, or a period this node's clock has not reached yet (the next read has it)
		}
		cs := s.counters[shardIndex(r.key)]
		cs.mu.Lock()
		c := cs.counterNoLock(r.key, start)
		if c.periodStart == start {
			c.base = r.counts
			c.promoted = true
			applied++
		}
		cs.mu.Unlock()
	})
	return applied, err
}

// counterNoLock returns the key's counter for the period starting at start, creating it or
// starting it over (counts to zero, unpromoted) if its period has ended. A counter of a later
// period is kept: the wall clock may step back a little at the boundary. The caller must hold cs.mu.
func (cs *counterShard) counterNoLock(key string, start int64) *counter {
	c, ok := cs.counters[key]
	if !ok {
		c = &counter{periodStart: start}
		cs.counters[key] = c
	} else if c.periodStart < start {
		*c = counter{periodStart: start}
	}
	return c
}
