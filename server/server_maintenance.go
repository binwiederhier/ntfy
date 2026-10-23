package server

import (
	"context"
	"errors"
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
)

const (
	tagMaintenance = "maintenance"

	// drainTickInterval is how often a drain closes its next share of streams; small enough that
	// the closes look continuous to the load balancers, rather than bursts
	drainTickInterval = 100 * time.Millisecond

	// drainLogInterval is how often a drain logs its progress
	drainLogInterval = 10 * time.Second
)

var (
	errMaintenanceDisabled     = errors.New("maintenance mode is off (experimental-cluster-drain-duration is 0)")
	errMaintenanceNotActive    = errors.New("only an active node can enter maintenance mode")
	errMaintenanceNoActivePeer = errors.New("no other active node to take this node's clients")
)

// maintenanceRun is maintenance mode while it is on: the goroutine running its phases, and how to stop it
type maintenanceRun struct {
	cancel context.CancelFunc
	done   chan struct{} // Closed when the phases end: drained, or turned off
}

// SetMaintenance turns maintenance mode on or off. It is a state, not a shutdown: a node can enter
// it, sit in it drained for as long as needed, and leave it again.
//
// On, it moves this node's clients to the other nodes in two phases, run in the background (it
// returns at once). Entering (ClusterDrainDelay): /v1/ready fails, so the readiness agents take the
// node out of the load balancers, but it still serves everything; it acts as passive in the
// cluster, so leadership moves and it claims no delayed messages. Draining (ClusterDrainDuration):
// read-only, i.e. new subscriptions and publishes get 503, and the open streams are closed evenly,
// so their clients reconnect to the other nodes a few at a time instead of all at once. Clients
// resume with since=, so nothing is lost. Polls and the cluster mesh keep working throughout. Then
// it stays drained. Only an active node with another active node can enter it: otherwise there is
// nowhere for its clients to go.
//
// Off, it stops the phases wherever they are and takes the node back into service: ready, writable,
// active in the cluster. Streams it closed already stay on the other nodes.
func (s *Server) SetMaintenance(on bool) error {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if !on {
		s.leaveMaintenanceNoLock()
		return nil
	} else if s.maintenance != nil {
		return nil // On already
	} else if s.config.ClusterDrainDuration <= 0 {
		return errMaintenanceDisabled
	} else if s.cluster.Role() != cluster.RoleActive {
		return errMaintenanceNotActive
	} else if !s.cluster.Peers().HasActive() {
		return errMaintenanceNoActivePeer
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.maintenance = &maintenanceRun{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.cluster.SetMaintenance(true) // Acts as passive: not ready, peers stop counting it, leadership moves
	go s.runMaintenance(ctx, s.maintenance.done)
	return nil
}

// WaitDrained blocks until maintenance mode has drained this node (or is turned off), or ctx is
// canceled. It returns at once if maintenance mode is off.
func (s *Server) WaitDrained(ctx context.Context) {
	s.maintenanceMu.Lock()
	run := s.maintenance
	s.maintenanceMu.Unlock()
	if run == nil {
		return
	}
	select {
	case <-run.done:
	case <-ctx.Done():
	}
}

// Drain moves this node's clients to the other nodes before a shutdown: it turns maintenance mode on
// and waits until drained, or until ctx is canceled (a second SIGTERM). A node that cannot enter
// maintenance mode (single node, last active node, passive node) returns at once.
func (s *Server) Drain(ctx context.Context) {
	if err := s.SetMaintenance(true); err != nil {
		if s.config.ClusterListen != "" { // Single node: never possible, so nothing worth saying
			log.Tag(tagMaintenance).Info("Not entering maintenance mode: %s", err.Error())
		}
		return
	}
	s.WaitDrained(ctx)
}

// runMaintenance runs the phases of maintenance mode (see SetMaintenance) until drained, or until
// ctx is canceled because maintenance mode was turned off
func (s *Server) runMaintenance(ctx context.Context, done chan struct{}) {
	defer close(done)

	// Entering: not ready, so the readiness agents take this node out of the load balancers, but
	// still serving everything, since requests already on their way must not fail
	if delay := s.config.ClusterDrainDelay; delay > 0 {
		log.Tag(tagMaintenance).Info("Entering maintenance mode: not ready, still serving for %v until the load balancers stop sending requests", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	// Draining: read-only, and the streams are closed evenly over the drain duration
	s.draining.Store(true)
	total := s.streams.Len()
	duration := s.config.ClusterDrainDuration
	log.Tag(tagMaintenance).Info("Maintenance mode: draining, closing %d stream(s) over %v, new subscriptions and publishes are rejected", total, duration)
	start, lastLog := time.Now(), time.Now()
	closed := 0
	ticker := time.NewTicker(drainTickInterval)
	defer ticker.Stop()
	for s.streams.Len() > 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// Close the share of the streams that is due by now. Streams that opened right as the drain
		// started (past the maintenance check, but not counted in total) go at the end.
		elapsed := time.Since(start)
		due := int(float64(total)*elapsed.Seconds()/duration.Seconds()) - closed
		if elapsed >= duration {
			due = s.streams.Len()
		}
		if due > 0 {
			closed += s.streams.CancelSome(due)
		}
		if time.Since(lastLog) >= drainLogInterval {
			log.Tag(tagMaintenance).Info("Maintenance mode: closed %d stream(s), %d left", closed, s.streams.Len())
			lastLog = time.Now()
		}
	}
	log.Tag(tagMaintenance).Info("Maintenance mode: drained, closed %d stream(s) in %v", closed, time.Since(start).Round(time.Millisecond))
}

// leaveMaintenanceNoLock stops the phases, waits for them to end (so draining is not set again
// after it is cleared), and takes the node back into service
func (s *Server) leaveMaintenanceNoLock() {
	run := s.maintenance
	if run == nil {
		return
	}
	run.cancel()
	<-run.done
	s.maintenance = nil
	s.draining.Store(false)
	s.cluster.SetMaintenance(false)
	log.Tag(tagMaintenance).Info("Left maintenance mode: ready, taking subscriptions and publishes again")
}
