package server

import (
	"context"
	"sync"
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
)

const (
	tagDrain = "drain"

	// drainTickInterval is how often a drain closes its next share of streams; small enough that
	// the closes look continuous to the load balancers, rather than bursts
	drainTickInterval = 100 * time.Millisecond

	// drainLogInterval is how often a drain logs its progress
	drainLogInterval = 10 * time.Second
)

// streamRegistry tracks the open subscriber connections (HTTP streams and WebSockets), one entry
// per connection however many topics it subscribes to, so a drain can close them one at a time.
// Polls are not streams: they end on their own.
type streamRegistry struct {
	nextID  int
	cancels map[int]context.CancelFunc
	mu      sync.Mutex // Protects nextID and cancels
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{
		cancels: make(map[int]context.CancelFunc),
	}
}

// Add registers a stream and returns its ID for Remove
func (r *streamRegistry) Add(cancel context.CancelFunc) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.cancels[r.nextID] = cancel
	return r.nextID
}

// Remove forgets a stream that ended; a stream CancelSome already closed is gone already
func (r *streamRegistry) Remove(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

// Len returns the number of open streams
func (r *streamRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cancels)
}

// CancelSome closes up to n streams, picked in map order (i.e. at random), and returns how
// many it closed. They are removed right away, so the next call picks others.
func (r *streamRegistry) CancelSome(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	canceled := 0
	for id, cancel := range r.cancels {
		if canceled >= n {
			break
		}
		cancel()
		delete(r.cancels, id)
		canceled++
	}
	return canceled
}

// Drain moves this node's clients to the other nodes before a shutdown: it stops taking new
// subscriptions (503, which a load balancer retries on another node; /v1/ready fails too) and
// closes the open streams evenly over ClusterDrainDuration, so their clients reconnect to the
// other nodes a few at a time instead of all at once. Clients resume with since=, so nothing is
// lost. It returns when no streams are left, or right away when ctx is canceled.
//
// It drains only an active node with at least one other healthy active node: otherwise there is
// nowhere for the clients to go, and a drain would only delay the shutdown.
func (s *Server) Drain(ctx context.Context) {
	if !s.shouldDrain() {
		return
	}
	s.draining.Store(true)
	total := s.streams.Len()
	duration := s.config.ClusterDrainDuration
	log.Tag(tagDrain).Info("Draining: closing %d stream(s) over %v, new subscriptions are rejected", total, duration)
	start, lastLog := time.Now(), time.Now()
	closed := 0
	ticker := time.NewTicker(drainTickInterval)
	defer ticker.Stop()
	for s.streams.Len() > 0 {
		select {
		case <-ctx.Done():
			log.Tag(tagDrain).Info("Draining canceled, %d stream(s) left", s.streams.Len())
			return
		case <-ticker.C:
		}
		// Close the share of the streams that is due by now. Streams that opened right as the drain
		// started (past the draining check, but not counted in total) go at the end.
		elapsed := time.Since(start)
		due := int(float64(total)*elapsed.Seconds()/duration.Seconds()) - closed
		if elapsed >= duration {
			due = s.streams.Len()
		}
		if due > 0 {
			closed += s.streams.CancelSome(due)
		}
		if time.Since(lastLog) >= drainLogInterval {
			log.Tag(tagDrain).Info("Draining: closed %d stream(s), %d left", closed, s.streams.Len())
			lastLog = time.Now()
		}
	}
	log.Tag(tagDrain).Info("Drained: closed %d stream(s) in %v", closed, time.Since(start).Round(time.Millisecond))
}

func (s *Server) shouldDrain() bool {
	if s.config.ClusterDrainDuration <= 0 || s.cluster.Role() != cluster.RoleActive {
		return false
	}
	for _, m := range s.cluster.Members() {
		if string(m.NodeID) != s.config.ClusterNodeID && m.Role == cluster.RoleActive && m.Healthy {
			return true
		}
	}
	log.Tag(tagDrain).Info("Not draining: no other active node to take this node's clients")
	return false
}
