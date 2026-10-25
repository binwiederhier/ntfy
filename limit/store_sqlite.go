package limit

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	sqliteSelectClockQuery    = `SELECT CAST(strftime('%s', 'now') AS INTEGER)`
	sqliteSelectSinceQuery    = `SELECT key, kind, count FROM usage WHERE day = ? AND updated_at >= ?`
	sqliteSelectKeysInQuery   = `SELECT key, kind, count FROM usage WHERE day = ? AND key IN (` // Completed per chunk
	sqliteUpsertPrefixQuery   = `INSERT INTO usage (key, day, kind, count, updated_at) VALUES `
	sqliteUpsertRowQuery      = `(?, ?, ?, ?, CAST(strftime('%s', 'now') AS INTEGER))`
	sqliteUpsertConflictQuery = ` ON CONFLICT (key, day, kind) DO UPDATE SET count = count + excluded.count, updated_at = excluded.updated_at`
	sqlitePruneQuery          = `DELETE FROM usage WHERE day < ?`
)

// Schema
var (
	sqliteCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS usage (
			key TEXT NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			count INT NOT NULL DEFAULT 0,
			updated_at INT NOT NULL,
			PRIMARY KEY (key, day, kind)
		);
		CREATE INDEX IF NOT EXISTS idx_usage_day_updated_at ON usage (day, updated_at);
	`)
)

// NewSQLiteStore opens (or creates) the usage database in the given file
func NewSQLiteStore(filename string) (*Store, error) {
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := schema.Migrate(d, schema.SQLite, schemaStore, sqliteCreateTables, nil); err != nil {
		return nil, err
	}
	return &Store{
		db:      db.New(&db.Host{DB: d}, nil),
		dialect: schema.SQLite,
		queries: queries{
			selectClock:    sqliteSelectClockQuery,
			selectSince:    sqliteSelectSinceQuery,
			selectKeysIn:   sqliteSelectKeysInQuery,
			upsertPrefix:   sqliteUpsertPrefixQuery,
			upsertRow:      sqliteUpsertRowQuery,
			upsertConflict: sqliteUpsertConflictQuery,
			prune:          sqlitePruneQuery,
		},
		closer: d,
	}, nil
}
