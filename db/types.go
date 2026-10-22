package db

import (
	"database/sql"
	"sync/atomic"
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
}

// Roles of a connection pool, see PoolStats
const (
	PoolRolePrimary = "primary"
	PoolRoleReplica = "replica"
)

// PoolStats is one connection pool's statistics: the primary's or a replica's
type PoolStats struct {
	Role  string // PoolRolePrimary or PoolRoleReplica
	Addr  string // "host:port"
	Stats sql.DBStats
}
