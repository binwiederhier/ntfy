package db

import (
	"database/sql"
	"sync/atomic"
	"time"
)

// Beginner is an interface for types that can begin a database transaction.
// Both *sql.DB and *DB implement this.
type Beginner interface {
	Begin() (*sql.Tx, error)
}

// Querier is an interface for types that can execute SQL queries.
// *sql.DB, *sql.Tx, and *DB all implement this.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// Host pairs a *sql.DB with the host:port it was opened against.
type Host struct {
	Addr    string // "host:port"
	DB      *sql.DB
	healthy atomic.Bool
	ping    atomic.Int64 // Duration of the last health check ping, in nanoseconds
}

// Roles of a connection pool, see PoolStats
const (
	PoolRolePrimary = "primary"
	PoolRoleReplica = "replica"
)

// PoolStats is one connection pool's statistics: the primary's or a replica's
type PoolStats struct {
	Role    string // PoolRolePrimary or PoolRoleReplica
	Addr    string // "host:port"
	Healthy bool   // The last health check (a ping) succeeded
	Stats   sql.DBStats

	// PingDuration is how long the last health check ping took, including any wait for a free
	// connection, so a stalled database shows even when its pool is idle; 0 before the first check
	PingDuration time.Duration
}
