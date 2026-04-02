package db

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tag                            = "db"
	replicaHealthCheckInitialDelay = 5 * time.Second
	replicaHealthCheckInterval     = 30 * time.Second
	replicaHealthCheckTimeout      = 10 * time.Second
)

// DB wraps a primary *sql.DB and optional read replicas. All standard query/exec methods
// delegate to the primary. The ReadOnly() method returns a *sql.DB from a healthy replica
// (round-robin), falling back to the primary if no replicas are configured or all are unhealthy.
type DB struct {
	primary  *Host
	replicas []*Host
	counter  atomic.Uint64
	cancel   context.CancelFunc
}

// New creates a new DB that wraps the given primary and optional replica connections.
// If replicas is nil or empty, ReadOnly() simply returns the primary.
// Replicas start unhealthy and are checked immediately by a background goroutine.
func New(primary *Host, replicas []*Host) *DB {
	ctx, cancel := context.WithCancel(context.Background())
	d := &DB{
		primary:  primary,
		replicas: replicas,
		cancel:   cancel,
	}
	if len(d.replicas) > 0 {
		go d.healthCheckLoop(ctx)
	}
	return d
}

// Query delegates to the primary database.
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.primary.DB.Query(query, args...)
}

// QueryRow delegates to the primary database.
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.primary.DB.QueryRow(query, args...)
}

// Exec delegates to the primary database.
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.primary.DB.Exec(query, args...)
}

// Begin delegates to the primary database.
func (d *DB) Begin() (*sql.Tx, error) {
	return d.primary.DB.Begin()
}

// Ping delegates to the primary database.
func (d *DB) Ping() error {
	return d.primary.DB.Ping()
}

// Primary returns the underlying primary *sql.DB. This is only intended for
// one-time schema setup during store initialization, not for regular queries.
func (d *DB) Primary() *sql.DB {
	return d.primary.DB
}

// ReadOnly returns a *sql.DB suitable for read-only queries. It round-robins across healthy
// replicas. If all replicas are unhealthy or none are configured, the primary is returned.
func (d *DB) ReadOnly() *sql.DB {
	if len(d.replicas) == 0 {
		return d.primary.DB
	}
	n := len(d.replicas)
	start := int(d.counter.Add(1) - 1)
	for i := 0; i < n; i++ {
		r := d.replicas[(start+i)%n]
		if r.healthy.Load() {
			return r.DB
		}
	}
	return d.primary.DB
}

// Close closes the primary database and all replicas, and stops the health-check goroutine.
func (d *DB) Close() error {
	d.cancel()
	for _, r := range d.replicas {
		r.DB.Close()
	}
	return d.primary.DB.Close()
}

// healthCheckLoop checks replicas immediately, then periodically on a ticker.
func (d *DB) healthCheckLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(replicaHealthCheckInitialDelay):
		d.checkReplicas(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(replicaHealthCheckInterval):
			d.checkReplicas(ctx)
		}
	}
}

// checkReplicas pings each replica with a timeout and updates its health status.
func (d *DB) checkReplicas(ctx context.Context) {
	for _, r := range d.replicas {
		wasHealthy := r.healthy.Load()
		pingCtx, cancel := context.WithTimeout(ctx, replicaHealthCheckTimeout)
		err := r.DB.PingContext(pingCtx)
		cancel()
		if err != nil {
			r.healthy.Store(false)
			log.Tag(tag).Error("Database replica %s is unhealthy: %s", r.Addr, err)
		} else {
			r.healthy.Store(true)
			if !wasHealthy {
				log.Tag(tag).Info("Database replica %s is healthy", r.Addr)
			}
		}
	}
}
