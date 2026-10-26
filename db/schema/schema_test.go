package schema_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/db/schema"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/log"

	_ "github.com/mattn/go-sqlite3"
)

const testCreateQuery = `CREATE TABLE IF NOT EXISTS things (id TEXT PRIMARY KEY, name TEXT NOT NULL)`

func testCreate(tx *sql.Tx) error {
	_, err := tx.Exec(testCreateQuery)
	return err
}

func openTestPostgres(t *testing.T) *sql.DB {
	t.Helper()
	host, err := pg.Open(dbtest.CreateTestPostgresSchema(t))
	require.Nil(t, err)
	t.Cleanup(func() { host.DB.Close() })
	return host.DB
}

func openTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	require.Nil(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func forEachDialect(t *testing.T, f func(t *testing.T, d *sql.DB, dialect schema.Dialect)) {
	t.Run("postgres", func(t *testing.T) {
		f(t, openTestPostgres(t), schema.Postgres)
	})
	t.Run("sqlite", func(t *testing.T) {
		f(t, openTestSQLite(t), schema.SQLite)
	})
}

func TestMigrate_FreshCreate(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		// A fresh database jumps straight to the target version; migration steps are not consulted
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, stepsTo(3)))
		_, err := d.Exec(`INSERT INTO things (id, name) VALUES ('a', 'thing a')`)
		require.Nil(t, err)
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
		// Idempotent: a second node boots against the migrated schema
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, stepsTo(3)))
	})
}

func TestMigrate_AppliesMigrationSteps(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		// A newer version of the code migrates 1 -> 3 step by step, in order
		migrations := schema.Migrations{
			{From: 1, To: 2}: schema.Func(func(tx *sql.Tx) error {
				_, err := tx.Exec(`ALTER TABLE things ADD COLUMN color TEXT NOT NULL DEFAULT ''`)
				return err
			}),
			{From: 2, To: 3}: schema.Func(func(tx *sql.Tx) error {
				_, err := tx.Exec(`ALTER TABLE things ADD COLUMN size INT NOT NULL DEFAULT 0`)
				return err
			}),
		}
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, migrations))
		_, err := d.Exec(`INSERT INTO things (id, name, color, size) VALUES ('b', 'thing b', 'red', 2)`)
		require.Nil(t, err)
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_TargetVersionFollowsSteps(t *testing.T) {
	// The target version is the one after the last step: adding a step is all it takes, there is
	// no separate version number to bump (and to forget)
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		require.Equal(t, 1, storeVersion(t, d, dialect, "things"))
		migrations := schema.Migrations{
			{From: 1, To: 2}: schema.Query(`ALTER TABLE things ADD COLUMN color TEXT NOT NULL DEFAULT ''`),
			{From: 2, To: 3}: schema.Query(`ALTER TABLE things ADD COLUMN size INT NOT NULL DEFAULT 0`),
		}
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, migrations))
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
		_, err := d.Exec(`INSERT INTO things (id, name, color, size) VALUES ('a', 'thing a', 'red', 2)`)
		require.Nil(t, err)
	})
}

func TestMigrate_GapInStepsFails(t *testing.T) {
	// A missing step in the middle fails before anything is created or migrated
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		err := schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{{From: 1, To: 2}: schema.Nop, {From: 3, To: 4}: schema.Nop})
		require.ErrorContains(t, err, "missing migration step from version 2 to 3")
		_, err = d.Exec(`INSERT INTO things (id, name) VALUES ('a', 'thing a')`)
		require.Error(t, err) // Not created
	})
}

func TestMigrate_ClosureCarriesConfig(t *testing.T) {
	// Migrations needing config take it via closure at map-construction time; there is no
	// params plumbing in the framework itself
	migrationsFor := func(defaultName string) schema.Migrations {
		return schema.Migrations{
			{From: 1, To: 2}: schema.Query(fmt.Sprintf(`ALTER TABLE things ADD COLUMN nick TEXT NOT NULL DEFAULT '%s'`, defaultName)),
		}
	}
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		_, err := d.Exec(`INSERT INTO things (id, name) VALUES ('a', 'thing a')`)
		require.Nil(t, err)
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, migrationsFor("configured-default")))
		var nick string
		require.Nil(t, d.QueryRow(`SELECT nick FROM things WHERE id = 'a'`).Scan(&nick))
		require.Equal(t, "configured-default", nick)
	})
}

func TestMigrate_InvalidDialect(t *testing.T) {
	d := openTestSQLite(t)
	err := schema.Migrate(d, schema.Dialect(99), "things", testCreate, nil)
	require.Error(t, err)
}

func TestMigrate_NewerVersionWarnsAndStarts(t *testing.T) {
	// An older version of ntfy (a rollback, or an old node during a rolling upgrade) runs on a
	// newer schema: it warns, starts, and leaves the newer version in place
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		var out bytes.Buffer
		log.SetOutput(&out)
		t.Cleanup(func() { log.SetOutput(os.Stderr) })
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, stepsTo(2)))
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		require.Contains(t, out.String(), "things database schema version 2 is newer than this version of ntfy supports (1)")
		require.Equal(t, 2, storeVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_BreakingNewerSchemaFails(t *testing.T) {
	// A breaking migration raises the store's minimum version: older code must not run on it
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{{From: 1, To: 2}: schema.Nop, {From: 2, To: 3}: schema.Breaking(schema.Nop)}))
		err := schema.Migrate(d, dialect, "things", testCreate, stepsTo(2))
		require.ErrorContains(t, err, "things database schema version 3 needs ntfy that supports at least version 3, this version supports up to 2")
	})
}

func TestMigrate_BreakingStepRaisesMinVersion(t *testing.T) {
	// The minimum version follows from the steps: the version after the newest breaking step
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{
			{From: 1, To: 2}: schema.Breaking(schema.Nop),
			{From: 2, To: 3}: schema.Nop,
		}))
		require.Equal(t, 2, storeMinVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_FreshCreateTakesMinVersionFromSteps(t *testing.T) {
	// A fresh database is created at the target version directly, but its minimum still comes from the steps
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{
			{From: 1, To: 2}: schema.Nop,
			{From: 2, To: 3}: schema.Breaking(schema.Nop),
		}))
		require.Equal(t, 3, storeMinVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_AddsMinVersionToAnExistingVersionTable(t *testing.T) {
	// Version tables created before the minimum version existed get the column; older code,
	// which writes only the version, keeps working on the new table
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		createOld, upsertOld := `CREATE TABLE schema_version (store TEXT PRIMARY KEY, version INT NOT NULL)`, `INSERT INTO schema_version (store, version) VALUES ('things', 1) ON CONFLICT (store) DO UPDATE SET version = EXCLUDED.version`
		if dialect == schema.SQLite {
			createOld, upsertOld = `CREATE TABLE schemaVersion (id INT PRIMARY KEY, version INT NOT NULL)`, `INSERT INTO schemaVersion (id, version) VALUES (1, 1) ON CONFLICT (id) DO UPDATE SET version = excluded.version`
		}
		_, err := d.Exec(testCreateQuery)
		require.Nil(t, err)
		_, err = d.Exec(createOld)
		require.Nil(t, err)
		_, err = d.Exec(upsertOld)
		require.Nil(t, err)
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		require.Equal(t, 1, storeMinVersion(t, d, dialect, "things"))
		_, err = d.Exec(upsertOld) // Older code writing its version
		require.Nil(t, err)

		// A breaking migration to version 2 locks out code that supports only version 1
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{{From: 1, To: 2}: schema.Breaking(schema.Nop)}))
		require.Equal(t, 2, storeMinVersion(t, d, dialect, "things"))
		require.Error(t, schema.Migrate(d, dialect, "things", testCreate, nil))
	})
}

func TestMigrate_VersionTableCreatedWhenMissing(t *testing.T) {
	// A database without a version table gets one with both columns, filled from the steps
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Empty(t, versionTableColumns(t, d, dialect))
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{{From: 1, To: 2}: schema.Nop, {From: 2, To: 3}: schema.Breaking(schema.Nop)}))
		require.ElementsMatch(t, []string{versionTableKeyColumn(dialect), "version", "min_version"}, versionTableColumns(t, d, dialect))
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
		require.Equal(t, 3, storeMinVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_OldVersionTableGetsMinVersion(t *testing.T) {
	// A version table from before min_version (ntfy <= v2.31) gets the column; the stored version
	// is kept, and the minimum defaults to 1
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		createOld, insertOld := `CREATE TABLE schema_version (store TEXT PRIMARY KEY, version INT NOT NULL)`, `INSERT INTO schema_version (store, version) VALUES ('things', 2)`
		if dialect == schema.SQLite {
			createOld, insertOld = `CREATE TABLE schemaVersion (id INT PRIMARY KEY, version INT NOT NULL)`, `INSERT INTO schemaVersion (id, version) VALUES (1, 2)`
		}
		for _, query := range []string{testCreateQuery, createOld, insertOld} {
			_, err := d.Exec(query)
			require.Nil(t, err)
		}
		require.ElementsMatch(t, []string{versionTableKeyColumn(dialect), "version"}, versionTableColumns(t, d, dialect))
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, stepsTo(2)))
		require.ElementsMatch(t, []string{versionTableKeyColumn(dialect), "version", "min_version"}, versionTableColumns(t, d, dialect))
		require.Equal(t, 2, storeVersion(t, d, dialect, "things"))
		require.Equal(t, 1, storeMinVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_CurrentVersionTableIsLeftAlone(t *testing.T) {
	// A version table that already has min_version is not altered again, and its values stay
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		migrations := schema.Migrations{{From: 1, To: 2}: schema.Breaking(schema.Nop)}
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, migrations))
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, migrations))
		require.ElementsMatch(t, []string{versionTableKeyColumn(dialect), "version", "min_version"}, versionTableColumns(t, d, dialect))
		require.Equal(t, 2, storeVersion(t, d, dialect, "things"))
		require.Equal(t, 2, storeMinVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_MissingStepFails(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", testCreate, nil))
		err := schema.Migrate(d, dialect, "things", testCreate, schema.Migrations{{From: 2, To: 3}: schema.Nop}) // No step 1 -> 2 registered
		require.ErrorContains(t, err, "cannot find things migration step from version 1 to 2")
	})
}

func TestMigrate_StoresAreIndependent(t *testing.T) {
	// Postgres only: stores share one database, tracked as rows in schema_version. On SQLite
	// every store has its own database file, so independence is by file.
	d := openTestPostgres(t)
	require.Nil(t, schema.Migrate(d, schema.Postgres, "things", testCreate, nil))
	require.Nil(t, schema.Migrate(d, schema.Postgres, "gadgets", func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS gadgets (id TEXT PRIMARY KEY)`)
		return err
	}, stepsTo(4)))
	require.Equal(t, 1, storeVersion(t, d, schema.Postgres, "things"))
	require.Equal(t, 4, storeVersion(t, d, schema.Postgres, "gadgets"))
}

func TestMigrate_SQLiteReadsExistingSchemaVersionTable(t *testing.T) {
	// Existing ntfy SQLite databases (message, user, webpush) track their version in a
	// schemaVersion (id, version) table keyed by id = 1; the framework uses that table as-is
	// on SQLite, so existing databases migrate without any adoption step
	d := openTestSQLite(t)
	_, err := d.Exec(testCreateQuery)
	require.Nil(t, err)
	_, err = d.Exec(`CREATE TABLE schemaVersion (id INT PRIMARY KEY, version INT NOT NULL)`)
	require.Nil(t, err)
	_, err = d.Exec(`INSERT INTO schemaVersion VALUES (1, 1)`)
	require.Nil(t, err)
	migrations := schema.Migrations{
		{From: 1, To: 2}: schema.Func(func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE things ADD COLUMN color TEXT NOT NULL DEFAULT ''`)
			return err
		}),
	}
	require.Nil(t, schema.Migrate(d, schema.SQLite, "things", testCreate, migrations))
	_, err = d.Exec(`INSERT INTO things (id, name, color) VALUES ('a', 'thing a', 'red')`)
	require.Nil(t, err)
	require.Equal(t, 2, storeVersion(t, d, schema.SQLite, "things"))
}

func TestMigrate_ConcurrentFreshCreate(t *testing.T) {
	// Postgres only: concurrent cold-boots must not race on DDL (CREATE TABLE IF NOT EXISTS is
	// not atomic); Migrate serializes via an advisory lock. SQLite has a single writer.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			host, err := pg.Open(schemaDSN)
			if err != nil {
				errs <- err
				return
			}
			defer host.DB.Close()
			errs <- schema.Migrate(host.DB, schema.Postgres, "things", testCreate, nil)
		}()
	}
	for i := 0; i < n; i++ {
		require.Nil(t, <-errs)
	}
}

func storeMinVersion(t *testing.T, d *sql.DB, dialect schema.Dialect, store string) int {
	t.Helper()
	var minVersion int
	if dialect == schema.Postgres {
		require.Nil(t, d.QueryRow(`SELECT min_version FROM schema_version WHERE store = $1`, store).Scan(&minVersion))
	} else {
		require.Nil(t, d.QueryRow(`SELECT min_version FROM schemaVersion WHERE id = 1`).Scan(&minVersion))
	}
	return minVersion
}

func storeVersion(t *testing.T, d *sql.DB, dialect schema.Dialect, store string) int {
	t.Helper()
	var version int
	if dialect == schema.Postgres {
		require.Nil(t, d.QueryRow(`SELECT version FROM schema_version WHERE store = $1`, store).Scan(&version), fmt.Sprintf("store %s", store))
	} else {
		require.Nil(t, d.QueryRow(`SELECT version FROM schemaVersion WHERE id = 1`).Scan(&version), fmt.Sprintf("store %s", store))
	}
	return version
}

// versionTableColumns returns the version table's column names, or none if it does not exist
func versionTableColumns(t *testing.T, d *sql.DB, dialect schema.Dialect) []string {
	t.Helper()
	query := `SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'schema_version'`
	if dialect == schema.SQLite {
		query = `SELECT name FROM pragma_table_info('schemaVersion')`
	}
	rows, err := d.Query(query)
	require.Nil(t, err)
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var column string
		require.Nil(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	require.Nil(t, rows.Err())
	return columns
}

// versionTableKeyColumn returns the version table's key: one row per store on Postgres, one row on SQLite
func versionTableKeyColumn(dialect schema.Dialect) string {
	if dialect == schema.Postgres {
		return "store"
	}
	return "id"
}

// stepsTo returns no-op steps leading to the given schema version
func stepsTo(version int) schema.Migrations {
	migrations := make(schema.Migrations)
	for v := 1; v < version; v++ {
		migrations[schema.MigrationKey{From: v, To: v + 1}] = schema.Nop
	}
	return migrations
}
