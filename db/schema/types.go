package schema

import "database/sql"

// Dialect selects the SQL flavor Migrate speaks to the version table.
type Dialect int

// Supported dialects; SQLite is the zero value
const (
	SQLite Dialect = iota
	Postgres
)

// MigrateFunc applies one schema change inside the setup transaction: the initial creation of
// a store's tables, or one step upgrading a store from version N to N+1. Migrations needing
// config capture it via closure, e.g. func migrations(cacheDuration time.Duration) Migrations.
type MigrateFunc func(tx *sql.Tx) error

// AsMigrateFunc converts a simple query to a migration function
func AsMigrateFunc(query string) MigrateFunc {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(query)
		return err
	}
}

// MigrationKey names a migration step by the versions it upgrades between; To is always From+1
type MigrationKey struct {
	From int
	To   int
}

// Migration is one step upgrading a store from version N to N+1
type Migration struct {
	Migrate  MigrateFunc
	Breaking bool // Older code cannot run on the result (see package doc): the store's minimum version becomes the step's To
}

// Migrations are a store's migration steps, keyed by the versions each upgrades between. Steps go
// one version at a time without gaps, which Migrate checks at startup.
type Migrations map[MigrationKey]Migration

// Nop is a migration step that does nothing, for versions where a dialect has no work to do (e.g.
// when only the other dialect's schema changed)
var Nop = Func(func(_ *sql.Tx) error {
	return nil
})

// Query is an additive migration step running a query
func Query(query string) Migration {
	return Func(AsMigrateFunc(query))
}

// Func is an additive migration step running a function
func Func(f MigrateFunc) Migration {
	return Migration{Migrate: f}
}

// Breaking marks a migration step as breaking: older code cannot run on its result (see package doc)
func Breaking(m Migration) Migration {
	m.Breaking = true
	return m
}
