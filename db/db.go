package db

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tag                     = "db"
	healthCheckInitialDelay = 5 * time.Second
	healthCheckInterval     = 30 * time.Second
	healthCheckTimeout      = 10 * time.Second
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
// Replicas start unhealthy, the primary healthy (it was just opened); a background goroutine
// checks all of them, so the metrics show a node that is cut off from its database.
func New(primary *Host, replicas []*Host) *DB {
	ctx, cancel := context.WithCancel(context.Background())
	d := &DB{
		primary:  primary,
		replicas: replicas,
		cancel:   cancel,
	}
	primary.healthy.Store(true)
	go d.healthCheckLoop(ctx)
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

// ExecContext delegates to the primary database.
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.primary.DB.ExecContext(ctx, query, args...)
}

// QueryContext delegates to the primary database.
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.primary.DB.QueryContext(ctx, query, args...)
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

// Stats returns the connection pool statistics of the primary, then of each replica
func (d *DB) Stats() []*PoolStats {
	stats := []*PoolStats{
		{
			Role:    PoolRolePrimary,
			Addr:    d.primary.Addr,
			Healthy: d.primary.healthy.Load(),
			Stats:   d.primary.DB.Stats(),
		},
	}
	for _, r := range d.replicas {
		stats = append(stats, &PoolStats{
			Role:    PoolRoleReplica,
			Addr:    r.Addr,
			Healthy: r.healthy.Load(),
			Stats:   r.DB.Stats(),
		})
	}
	return stats
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

// MarkReplicasHealthyForTest immediately marks all configured replicas as healthy, bypassing the
// async health-check loop's initial delay. It exists so tests can deterministically route
// ReadOnly() to a replica without waiting; it is not used in production code.
func (d *DB) MarkReplicasHealthyForTest() {
	for _, r := range d.replicas {
		r.healthy.Store(true)
	}
}

// Close closes the primary database and all replicas, and stops the health-check goroutine.
func (d *DB) Close() error {
	d.cancel()
	for _, r := range d.replicas {
		r.DB.Close()
	}
	return d.primary.DB.Close()
}

// healthCheckLoop checks the primary and the replicas shortly after start, then periodically.
func (d *DB) healthCheckLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(healthCheckInitialDelay):
		d.checkHealth(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(healthCheckInterval):
			d.checkHealth(ctx)
		}
	}
}

// checkHealth pings the primary and each replica with a timeout and updates their health status.
func (d *DB) checkHealth(ctx context.Context) {
	checkHost(ctx, d.primary, "primary")
	for _, r := range d.replicas {
		checkHost(ctx, r, "replica")
	}
}

// checkHost pings the host and updates its health status, logging when it changes (and every
// failure, so a lasting outage stays visible in the logs)
func checkHost(ctx context.Context, h *Host, kind string) {
	wasHealthy := h.healthy.Load()
	pingCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	err := h.DB.PingContext(pingCtx)
	cancel()
	if err != nil {
		h.healthy.Store(false)
		log.Tag(tag).Error("Database %s %s is unhealthy: %s", kind, h.Addr, err)
	} else {
		h.healthy.Store(true)
		if !wasHealthy {
			log.Tag(tag).Info("Database %s %s is healthy", kind, h.Addr)
		}
	}
}
