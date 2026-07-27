// Package schema tracks and migrates database schemas: each store records its version in the
// shared schema_version table (keyed by store name), and Migrate creates or upgrades the
// store's schema inside a single transaction. Works on PostgreSQL and SQLite.
package schema

import (
	"database/sql"
	"errors"
	"fmt"

	"heckel.io/ntfy/v2/db/pg"
)

// Queries by dialect; the version table is shared by all stores
const (
	createVersionTableQuery = `CREATE TABLE IF NOT EXISTS schema_version (store TEXT PRIMARY KEY, version INT NOT NULL)`

	postgresSelectVersionQuery = `SELECT version FROM schema_version WHERE store = $1`
	postgresUpsertVersionQuery = `INSERT INTO schema_version (store, version) VALUES ($1, $2) ON CONFLICT (store) DO UPDATE SET version = EXCLUDED.version`
	postgresAdvisoryLockQuery  = `SELECT pg_advisory_xact_lock($1)`

	sqliteSelectVersionQuery = `SELECT version FROM schema_version WHERE store = ?`
	sqliteUpsertVersionQuery = `INSERT INTO schema_version (store, version) VALUES (?, ?) ON CONFLICT (store) DO UPDATE SET version = excluded.version`
)

// Dialect selects the SQL flavor Migrate speaks to the version table.
type Dialect int

// Supported dialects; SQLite is the zero value
const (
	SQLite Dialect = iota
	Postgres
)

// MigrateFunc applies one schema change inside the setup transaction: the initial creation of
// a store's tables, or one step upgrading a store from version N to N+1. Migrations needing
// config capture it via closure, e.g. func migrations(cacheDuration time.Duration) map[int]MigrateFunc.
type MigrateFunc func(tx *sql.Tx) error

// Migrate creates or upgrades the named store's schema to targetVersion in one transaction: a
// fresh database gets the create func; an existing one is upgraded step by step through the
// migrations map (keyed by the FROM version; always append, never insert in the middle); a
// schema migrated by newer code is refused. A failed migration rolls back atomically. Caveats:
// statements that refuse transaction blocks (Postgres CREATE INDEX CONCURRENTLY, VACUUM) cannot
// go through Migrate, and DDL holds exclusive locks until commit, so keep migrations fast.
func Migrate(db *sql.DB, dialect Dialect, store string, targetVersion int, create MigrateFunc, migrations map[int]MigrateFunc) error {
	if dialect != Postgres && dialect != SQLite {
		return fmt.Errorf("unsupported schema dialect %d", dialect)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if dialect == Postgres {
		// Serialize setup across nodes: CREATE TABLE IF NOT EXISTS is not atomic, and
		// concurrently cold-booting nodes would otherwise race on DDL and crash
		if _, err := tx.Exec(postgresAdvisoryLockQuery, pg.SchemaLockKey); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(createVersionTableQuery); err != nil {
		return err
	}
	var version int
	err = tx.QueryRow(selectVersionQuery(dialect), store).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		// Fresh database: create the store's tables at the target version
		if err := create(tx); err != nil {
			return err
		}
		if err := writeVersion(tx, dialect, store, targetVersion); err != nil {
			return err
		}
		return tx.Commit()
	} else if err != nil {
		return err
	}
	if version == targetVersion {
		return tx.Commit()
	}
	if version > targetVersion {
		return fmt.Errorf("unexpected %s schema version %d, this version of ntfy supports up to %d", store, version, targetVersion)
	}
	for v := version; v < targetVersion; v++ {
		migrate, ok := migrations[v]
		if !ok {
			return fmt.Errorf("cannot find %s migration step from version %d to %d", store, v, v+1)
		}
		if err := migrate(tx); err != nil {
			return err
		}
	}
	if err := writeVersion(tx, dialect, store, targetVersion); err != nil {
		return err
	}
	return tx.Commit()
}

func selectVersionQuery(dialect Dialect) string {
	if dialect == Postgres {
		return postgresSelectVersionQuery
	}
	return sqliteSelectVersionQuery
}

func writeVersion(tx *sql.Tx, dialect Dialect, store string, version int) error {
	query := sqliteUpsertVersionQuery
	if dialect == Postgres {
		query = postgresUpsertVersionQuery
	}
	_, err := tx.Exec(query, store, version)
	return err
}
