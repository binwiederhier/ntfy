// Package schema tracks and migrates database schemas, and Migrate creates or upgrades a
// store's schema inside a single transaction. On PostgreSQL, all stores share one database, so
// versions live in a shared schema_version table keyed by store name. On SQLite, every store is
// its own database file, so the version lives in the schemaVersion table keyed by id = 1.
//
// An older version of ntfy runs on a newer schema (after a rollback, or as an old node during a
// rolling cluster upgrade), so migrations must be additive: new tables, indexes, and columns that
// are nullable or have a default. Never drop or rename a column, add a NOT NULL column without a
// default, or change what existing data means. A change that cannot be additive takes two
// releases: the first adds the new shape and keeps the old one working, a later one removes the old.
//
// If a breaking migration is unavoidable, mark its step with Breaking: the store's minimum version
// (the oldest schema version whose code still runs on the schema) becomes the step's result, and
// older code then refuses to start instead of misreading the data.
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

// The minimum version is a second row next to the version, not a column, so that the table older
// ntfy versions read and write does not change: on SQLite the row with id = 2, on Postgres the row
// of the store name plus minVersionStoreSuffix. A missing row means 1.
const (
	sqliteVersionID    = 1
	sqliteMinVersionID = 2

	sqliteCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schemaVersion (id INT PRIMARY KEY, version INT NOT NULL)`
	sqliteSelectVersionQuery      = `SELECT version FROM schemaVersion WHERE id = ?`
	sqliteUpsertVersionQuery      = `INSERT INTO schemaVersion (id, version) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET version = excluded.version`

	minVersionStoreSuffix = ":min"

	postgresCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schema_version (store TEXT PRIMARY KEY, version INT NOT NULL)`
	postgresSelectVersionQuery      = `SELECT version FROM schema_version WHERE store = $1`
	postgresUpsertVersionQuery      = `INSERT INTO schema_version (store, version) VALUES ($1, $2) ON CONFLICT (store) DO UPDATE SET version = EXCLUDED.version`
	postgresAdvisoryLockQuery       = `SELECT pg_advisory_xact_lock($1)` // Transaction-scoped lock to avoid migration races
)

// Migrate creates or upgrades the named store's schema to targetVersion in one transaction, or
// creates a new database using the "create" function. The store's minimum version follows from the
// steps: the version after the newest breaking step, or 1 (see package doc).
func Migrate(db *sql.DB, dialect Dialect, store string, targetVersion int, create MigrateFunc, migrations map[int]Migration) error {
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
	minVersion := minVersionOf(targetVersion, migrations)
	if _, err := tx.Exec(createVersionTableQuery(dialect)); err != nil {
		return fmt.Errorf("cannot create schema version table: %w", err)
	}
	version, storedMinVersion, err := readVersion(tx, dialect, store)
	if errors.Is(err, sql.ErrNoRows) {
		// Fresh database: create the store's tables at the target version
		if err := create(tx); err != nil {
			return fmt.Errorf("cannot create %s schema: %w", store, err)
		}
		if err := writeVersion(tx, dialect, store, targetVersion, minVersion); err != nil {
			return fmt.Errorf("cannot write %s schema version: %w", store, err)
		}
		return tx.Commit()
	} else if err != nil {
		return fmt.Errorf("cannot read %s schema version: %w", store, err)
	}
	if storedMinVersion > targetVersion {
		return fmt.Errorf("the %s database schema version %d needs ntfy that supports at least version %d, this version supports up to %d", store, version, storedMinVersion, targetVersion)
	} else if version == targetVersion {
		return tx.Commit()
	} else if version > targetVersion {
		// Migrations are additive (see package doc), so this version still works on the newer schema
		log.Tag(tag).Warn("The %s database schema version %d is newer than this version of ntfy supports (%d); continuing, since schema changes are backwards compatible", store, version, targetVersion)
		return tx.Commit()
	}
	for v := version; v < targetVersion; v++ {
		step, ok := migrations[v]
		if !ok {
			return fmt.Errorf("cannot find %s migration step from version %d to %d", store, v, v+1)
		}
		log.Tag(tag).Info("Migrating %s database schema: from %d to %d", store, v, v+1)
		if err := step.Migrate(tx); err != nil {
			return fmt.Errorf("%s migration step from version %d to %d failed: %w", store, v, v+1, err)
		}
	}
	if err := writeVersion(tx, dialect, store, targetVersion, minVersion); err != nil {
		return fmt.Errorf("cannot write %s schema version: %w", store, err)
	}
	return tx.Commit()
}

// minVersionOf returns the oldest schema version whose code still runs on targetVersion's schema:
// the version after the newest breaking step, or 1 if no step is breaking
func minVersionOf(targetVersion int, migrations map[int]Migration) int {
	minVersion := 1
	for v, step := range migrations {
		if step.Breaking && v < targetVersion && v+1 > minVersion {
			minVersion = v + 1
		}
	}
	return minVersion
}

func createVersionTableQuery(dialect Dialect) string {
	if dialect == Postgres {
		return postgresCreateVersionTableQuery
	}
	return sqliteCreateVersionTableQuery
}

// readVersion reads the store's version and minimum version; sql.ErrNoRows means a fresh database
func readVersion(tx *sql.Tx, dialect Dialect, store string) (version, minVersion int, err error) {
	if err := readVersionRow(tx, dialect, store, false, &version); err != nil {
		return 0, 0, err
	}
	if err := readVersionRow(tx, dialect, store, true, &minVersion); errors.Is(err, sql.ErrNoRows) {
		minVersion = 1 // Written by code from before the minimum version existed
	} else if err != nil {
		return 0, 0, err
	}
	return version, minVersion, nil
}

func writeVersion(tx *sql.Tx, dialect Dialect, store string, version, minVersion int) error {
	if err := writeVersionRow(tx, dialect, store, false, version); err != nil {
		return err
	}
	return writeVersionRow(tx, dialect, store, true, minVersion)
}

func readVersionRow(tx *sql.Tx, dialect Dialect, store string, isMin bool, version *int) error {
	if dialect == Postgres {
		return tx.QueryRow(postgresSelectVersionQuery, versionKey(store, isMin)).Scan(version)
	}
	return tx.QueryRow(sqliteSelectVersionQuery, sqliteVersionRowID(isMin)).Scan(version)
}

func writeVersionRow(tx *sql.Tx, dialect Dialect, store string, isMin bool, version int) error {
	var err error
	if dialect == Postgres {
		_, err = tx.Exec(postgresUpsertVersionQuery, versionKey(store, isMin), version)
	} else {
		_, err = tx.Exec(sqliteUpsertVersionQuery, sqliteVersionRowID(isMin), version)
	}
	return err
}

func versionKey(store string, isMin bool) string {
	if isMin {
		return store + minVersionStoreSuffix
	}
	return store
}

func sqliteVersionRowID(isMin bool) int {
	if isMin {
		return sqliteMinVersionID
	}
	return sqliteVersionID
}
