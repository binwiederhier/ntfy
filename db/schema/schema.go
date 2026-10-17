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
// older code then refuses to start instead of misreading the data. The minimum is only checked
// when a store starts: nodes already running older code keep going, and code from before the
// minimum existed never checks it. So a breaking migration also needs a deploy that stops or
// upgrades every node running older code first; it cannot be rolled out node by node.
package schema

import (
	"database/sql"
	"errors"
	"fmt"
	"math"

	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/log"
)

const (
	tag = "schema"
)

const (
	sqliteCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schemaVersion (id INT PRIMARY KEY, version INT NOT NULL, min_version INT NOT NULL DEFAULT 1)`
	sqliteHasMinVersionQuery      = `SELECT COUNT(*) FROM pragma_table_info('schemaVersion') WHERE name = 'min_version'`
	sqliteAddMinVersionQuery      = `ALTER TABLE schemaVersion ADD COLUMN min_version INT NOT NULL DEFAULT 1`
	sqliteSelectVersionQuery      = `SELECT version, min_version FROM schemaVersion WHERE id = 1`
	sqliteUpsertVersionQuery      = `INSERT INTO schemaVersion (id, version, min_version) VALUES (1, ?, ?) ON CONFLICT (id) DO UPDATE SET version = excluded.version, min_version = excluded.min_version`

	postgresCreateVersionTableQuery = `CREATE TABLE IF NOT EXISTS schema_version (store TEXT PRIMARY KEY, version INT NOT NULL, min_version INT NOT NULL DEFAULT 1)`
	postgresHasMinVersionQuery      = `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'schema_version' AND column_name = 'min_version'`
	postgresAddMinVersionQuery      = `ALTER TABLE schema_version ADD COLUMN min_version INT NOT NULL DEFAULT 1`
	postgresSelectVersionQuery      = `SELECT version, min_version FROM schema_version WHERE store = $1`
	postgresUpsertVersionQuery      = `INSERT INTO schema_version (store, version, min_version) VALUES ($1, $2, $3) ON CONFLICT (store) DO UPDATE SET version = EXCLUDED.version, min_version = EXCLUDED.min_version`
	postgresAdvisoryLockQuery       = `SELECT pg_advisory_xact_lock($1)` // Transaction-scoped lock to avoid migration races
)

// Migrate creates or upgrades the named store's schema in one transaction, or creates a new
// database using the "create" function. Both versions follow from the steps: the target is the
// version after the last step (see targetVersionOf), the minimum the version after the newest
// breaking step, or 1 (see package doc).
func Migrate(db *sql.DB, dialect Dialect, store string, create MigrateFunc, migrations map[int]Migration) error {
	if dialect != Postgres && dialect != SQLite {
		return fmt.Errorf("unsupported schema dialect %d", dialect)
	}
	targetVersion, err := targetVersionOf(migrations)
	if err != nil {
		return fmt.Errorf("invalid %s migration steps: %w", store, err)
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
	minVersion := minVersionOf(migrations)
	if err := ensureVersionTable(tx, dialect); err != nil {
		return err
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

// targetVersionOf returns the schema version a store's steps lead to: the version after the last
// step, or 1 without steps. The steps must be contiguous, so a forgotten one fails at startup.
func targetVersionOf(migrations map[int]Migration) (int, error) {
	if len(migrations) == 0 {
		return 1, nil
	}
	first, last := math.MaxInt, 0
	for v := range migrations {
		first, last = min(first, v), max(last, v)
	}
	if first < 1 {
		return 0, fmt.Errorf("migration step %d is invalid, steps start at version 1", first)
	}
	for v := first; v < last; v++ {
		if _, ok := migrations[v]; !ok {
			return 0, fmt.Errorf("missing migration step from version %d to %d", v, v+1)
		}
	}
	return last + 1, nil
}

// minVersionOf returns the oldest schema version whose code still runs on the schema the steps
// lead to: the version after the newest breaking step, or 1 if no step is breaking
func minVersionOf(migrations map[int]Migration) int {
	minVersion := 1
	for v, step := range migrations {
		if step.Breaking && v+1 > minVersion {
			minVersion = v + 1
		}
	}
	return minVersion
}

// ensureVersionTable creates the version table, and adds the min_version column to one created
// before it existed. Older code writes only the version and leaves the column alone.
func ensureVersionTable(tx *sql.Tx, dialect Dialect) error {
	createQuery, hasQuery, addQuery := sqliteCreateVersionTableQuery, sqliteHasMinVersionQuery, sqliteAddMinVersionQuery
	if dialect == Postgres {
		createQuery, hasQuery, addQuery = postgresCreateVersionTableQuery, postgresHasMinVersionQuery, postgresAddMinVersionQuery
	}
	if _, err := tx.Exec(createQuery); err != nil {
		return fmt.Errorf("cannot create schema version table: %w", err)
	}
	var columns int
	if err := tx.QueryRow(hasQuery).Scan(&columns); err != nil {
		return fmt.Errorf("cannot inspect schema version table: %w", err)
	} else if columns > 0 {
		return nil
	}
	if _, err := tx.Exec(addQuery); err != nil {
		return fmt.Errorf("cannot add min_version to schema version table: %w", err)
	}
	return nil
}

func readVersion(tx *sql.Tx, dialect Dialect, store string) (version, minVersion int, err error) {
	if dialect == Postgres {
		err = tx.QueryRow(postgresSelectVersionQuery, store).Scan(&version, &minVersion)
	} else {
		err = tx.QueryRow(sqliteSelectVersionQuery).Scan(&version, &minVersion)
	}
	return
}

func writeVersion(tx *sql.Tx, dialect Dialect, store string, version, minVersion int) error {
	var err error
	if dialect == Postgres {
		_, err = tx.Exec(postgresUpsertVersionQuery, store, version, minVersion)
	} else {
		_, err = tx.Exec(sqliteUpsertVersionQuery, version, minVersion)
	}
	return err
}
