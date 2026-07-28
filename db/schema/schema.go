// Package schema tracks and migrates database schemas, and Migrate creates or upgrades a
// store's schema inside a single transaction. On PostgreSQL, all stores share one database, so
// versions live in a shared schema_version table keyed by store name. On SQLite, every store is
// its own database file, so the version lives in the schemaVersion table keyed by id = 1.
package schema

import (
	"database/sql"
	"errors"
	"fmt"

	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/log"
)

const (
	tag = "schema"
)

const (
	sqliteCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schemaVersion (id INT PRIMARY KEY, version INT NOT NULL)`
	sqliteSelectVersionQuery      = `SELECT version FROM schemaVersion WHERE id = 1`
	sqliteUpsertVersionQuery      = `INSERT INTO schemaVersion (id, version) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET version = excluded.version`

	postgresCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schema_version (store TEXT PRIMARY KEY, version INT NOT NULL)`
	postgresSelectVersionQuery      = `SELECT version FROM schema_version WHERE store = $1`
	postgresUpsertVersionQuery      = `INSERT INTO schema_version (store, version) VALUES ($1, $2) ON CONFLICT (store) DO UPDATE SET version = EXCLUDED.version`
	postgresAdvisoryLockQuery       = `SELECT pg_advisory_xact_lock($1)` // Transaction-scoped lock to avoid migration races
)

// Migrate creates or upgrades the named store's schema to targetVersion in one transaction, or
// creates a new database using the "create" function.
func Migrate(db *sql.DB, dialect Dialect, store string, targetVersion int, create MigrateFunc, migrations map[int]MigrateFunc) error {
	if dialect != Postgres && dialect != SQLite {
		return fmt.Errorf("unsupported schema dialect %d", dialect)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("cannot begin %s schema transaction: %w", store, err)
	}
	defer tx.Rollback()
	if dialect == Postgres {
		// Serialize setup across nodes: CREATE TABLE IF NOT EXISTS is not atomic, and
		// concurrently cold-booting nodes would otherwise race on DDL and crash
		if _, err := tx.Exec(postgresAdvisoryLockQuery, pg.SchemaLockKey); err != nil {
			return fmt.Errorf("cannot acquire %s schema advisory lock: %w", store, err)
		}
	}
	if _, err := tx.Exec(createVersionTableQuery(dialect)); err != nil {
		return fmt.Errorf("cannot create schema version table: %w", err)
	}
	version, err := readVersion(tx, dialect, store)
	if errors.Is(err, sql.ErrNoRows) {
		// Fresh database: create the store's tables at the target version
		if err := create(tx); err != nil {
			return fmt.Errorf("cannot create %s schema: %w", store, err)
		}
		if err := writeVersion(tx, dialect, store, targetVersion); err != nil {
			return fmt.Errorf("cannot write %s schema version: %w", store, err)
		}
		return tx.Commit()
	} else if err != nil {
		return fmt.Errorf("cannot read %s schema version: %w", store, err)
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
		log.Tag(tag).Info("Migrating %s database schema: from %d to %d", store, v, v+1)
		if err := migrate(tx); err != nil {
			return fmt.Errorf("%s migration step from version %d to %d failed: %w", store, v, v+1, err)
		}
	}
	if err := writeVersion(tx, dialect, store, targetVersion); err != nil {
		return fmt.Errorf("cannot write %s schema version: %w", store, err)
	}
	return tx.Commit()
}

func createVersionTableQuery(dialect Dialect) string {
	if dialect == Postgres {
		return postgresCreateVersionTableQuery
	}
	return sqliteCreateVersionTableQuery
}

func readVersion(tx *sql.Tx, dialect Dialect, store string) (version int, err error) {
	if dialect == Postgres {
		err = tx.QueryRow(postgresSelectVersionQuery, store).Scan(&version)
	} else {
		err = tx.QueryRow(sqliteSelectVersionQuery).Scan(&version)
	}
	return
}

func writeVersion(tx *sql.Tx, dialect Dialect, store string, version int) error {
	var err error
	if dialect == Postgres {
		_, err = tx.Exec(postgresUpsertVersionQuery, store, version)
	} else {
		_, err = tx.Exec(sqliteUpsertVersionQuery, version)
	}
	return err
}
