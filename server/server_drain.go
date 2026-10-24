package server

import (
	"context"
	"errors"
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

var (
	errDrainNotActive    = errors.New("only an active node drains")
	errDrainNoActivePeer = errors.New("no other active node to take this node's clients")
)

// Drain moves this node's clients to the other nodes before a shutdown, in two phases, and returns
// when no streams are left, or right away when ctx is canceled (a second SIGTERM):
//
//   - Phase 1, withdrawing (ClusterDrainDelay): /v1/ready fails, so the readiness agents take the
//     node out of the load balancers, but it still serves everything, so requests already on their
//     way succeed. It acts as passive in the cluster from the start: leadership moves, and it claims
//     no delayed messages.
//   - Phase 2, read-only (ClusterDrainDuration): new subscriptions and publishes get 503, and its
//     streams are closed evenly (all at once with a duration of 0), so their clients reconnect to the
//     other nodes a few at a time instead of all at once. Clients resume with since=, so nothing is
//     lost. Polls and the mesh keep working.
//
// Only an active node with another active node drains: otherwise there is nowhere for its clients
// to go, and a drain would only delay the shutdown.
func (s *Server) Drain(ctx context.Context) {
	if err := s.canDrain(); err != nil {
		if s.config.ClusterListen != "" { // Single node: never possible, so nothing worth saying
			log.Tag(tagDrain).Info("Not draining: %s", err.Error())
		}
		return
	}
	// Phase 1: Withdrawing. Acts as passive (not ready, peers stop counting it, leadership moves),
	// but still serves everything, since requests already on their way must not fail
	s.cluster.SetDraining(true)
	if delay := s.config.ClusterDrainDelay; delay > 0 {
		log.Tag(tagDrain).Info("Draining: withdrawing, still serving for %v until the load balancers stop sending requests", delay)
		select {
		case <-ctx.Done():
			log.Tag(tagDrain).Info("Drain canceled")
			return
		case <-time.After(delay):
		}
	}

	// Phase 2: Read-only. The streams are closed evenly over the drain duration
	s.readOnly.Store(true)
	total := s.streams.Len()
	duration := s.config.ClusterDrainDuration
	log.Tag(tagDrain).Info("Draining: read-only, closing %d stream(s) over %v", total, duration)
	start, lastLog := time.Now(), time.Now()
	closed := 0
	ticker := time.NewTicker(drainTickInterval)
	defer ticker.Stop()
	for s.streams.Len() > 0 {
		select {
		case <-ctx.Done():
			log.Tag(tagDrain).Info("Drain canceled, %d stream(s) left", s.streams.Len())
			return
		case <-ticker.C:
		}
		// Close the share of the streams that is due by now (all of them with a duration of 0). Streams
		// that opened right as the drain started (past the read-only check, but not counted in total)
		// go at the end.
		elapsed := time.Since(start)
		due := s.streams.Len()
		if elapsed < duration {
			due = int(float64(total)*elapsed.Seconds()/duration.Seconds()) - closed
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

// canDrain returns why this node cannot drain, or nil if it can
func (s *Server) canDrain() error {
	if s.cluster.Role() != cluster.RoleActive {
		return errDrainNotActive
	} else if !s.cluster.Peers().HasActive() {
		return errDrainNoActivePeer
	}
	return nil
}
