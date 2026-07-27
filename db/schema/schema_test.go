package schema_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/db/schema"
	dbtest "heckel.io/ntfy/v2/db/test"

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
		require.Nil(t, schema.Migrate(d, dialect, "things", 3, testCreate, nil))
		_, err := d.Exec(`INSERT INTO things (id, name) VALUES ('a', 'thing a')`)
		require.Nil(t, err)
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
		// Idempotent: a second node boots against the migrated schema
		require.Nil(t, schema.Migrate(d, dialect, "things", 3, testCreate, nil))
	})
}

func TestMigrate_AppliesMigrationSteps(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", 1, testCreate, nil))
		// A newer version of the code migrates 1 -> 3 step by step, in order
		migrations := map[int]schema.MigrateFunc{
			1: func(tx *sql.Tx) error {
				_, err := tx.Exec(`ALTER TABLE things ADD COLUMN color TEXT NOT NULL DEFAULT ''`)
				return err
			},
			2: func(tx *sql.Tx) error {
				_, err := tx.Exec(`ALTER TABLE things ADD COLUMN size INT NOT NULL DEFAULT 0`)
				return err
			},
		}
		require.Nil(t, schema.Migrate(d, dialect, "things", 3, testCreate, migrations))
		_, err := d.Exec(`INSERT INTO things (id, name, color, size) VALUES ('b', 'thing b', 'red', 2)`)
		require.Nil(t, err)
		require.Equal(t, 3, storeVersion(t, d, dialect, "things"))
	})
}

func TestMigrate_ClosureCarriesConfig(t *testing.T) {
	// Migrations needing config take it via closure at map-construction time; there is no
	// params plumbing in the framework itself
	migrationsFor := func(defaultName string) map[int]schema.MigrateFunc {
		return map[int]schema.MigrateFunc{
			1: schema.AsMigrateFunc(fmt.Sprintf(`ALTER TABLE things ADD COLUMN nick TEXT NOT NULL DEFAULT '%s'`, defaultName)),
		}
	}
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", 1, testCreate, nil))
		_, err := d.Exec(`INSERT INTO things (id, name) VALUES ('a', 'thing a')`)
		require.Nil(t, err)
		require.Nil(t, schema.Migrate(d, dialect, "things", 2, testCreate, migrationsFor("configured-default")))
		var nick string
		require.Nil(t, d.QueryRow(`SELECT nick FROM things WHERE id = 'a'`).Scan(&nick))
		require.Equal(t, "configured-default", nick)
	})
}

func TestMigrate_InvalidDialect(t *testing.T) {
	d := openTestSQLite(t)
	err := schema.Migrate(d, schema.Dialect(99), "things", 1, testCreate, nil)
	require.Error(t, err)
}

func TestMigrate_RefusesFutureVersion(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", 2, testCreate, map[int]schema.MigrateFunc{}))
		err := schema.Migrate(d, dialect, "things", 1, testCreate, nil)
		require.Error(t, err)
	})
}

func TestMigrate_MissingStepFails(t *testing.T) {
	forEachDialect(t, func(t *testing.T, d *sql.DB, dialect schema.Dialect) {
		require.Nil(t, schema.Migrate(d, dialect, "things", 1, testCreate, nil))
		err := schema.Migrate(d, dialect, "things", 3, testCreate, nil) // No step 1 -> 2 registered
		require.Error(t, err)
	})
}

func TestMigrate_StoresAreIndependent(t *testing.T) {
	// Postgres only: stores share one database, tracked as rows in schema_version. On SQLite
	// every store has its own database file, so independence is by file.
	d := openTestPostgres(t)
	require.Nil(t, schema.Migrate(d, schema.Postgres, "things", 1, testCreate, nil))
	require.Nil(t, schema.Migrate(d, schema.Postgres, "gadgets", 4, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS gadgets (id TEXT PRIMARY KEY)`)
		return err
	}, nil))
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
	migrations := map[int]schema.MigrateFunc{
		1: func(tx *sql.Tx) error {
			_, err := tx.Exec(`ALTER TABLE things ADD COLUMN color TEXT NOT NULL DEFAULT ''`)
			return err
		},
	}
	require.Nil(t, schema.Migrate(d, schema.SQLite, "things", 2, testCreate, migrations))
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
			errs <- schema.Migrate(host.DB, schema.Postgres, "things", 1, testCreate, nil)
		}()
	}
	for i := 0; i < n; i++ {
		require.Nil(t, <-errs)
	}
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
