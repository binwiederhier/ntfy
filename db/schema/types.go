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
// config capture it via closure, e.g. func migrations(cacheDuration time.Duration) map[int]MigrateFunc.
type MigrateFunc func(tx *sql.Tx) error

// AsMigrateFunc converts a simple query to a migration function
func AsMigrateFunc(query string) MigrateFunc {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(query)
		return err
	}
}

// NopMigrateFunc is a migration step that does nothing, for versions where a dialect has no
// work to do (e.g. when only the other dialect's schema changed).
func NopMigrateFunc(_ *sql.Tx) error {
	return nil
}
