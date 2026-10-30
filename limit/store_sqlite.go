package limit

import (
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

// NewSQLiteStore returns the store on the user database (auth-file), which holds the usage table
// (see package user, which creates and migrates it)
func NewSQLiteStore(d *db.DB) *Store {
	return &Store{
		db:      d,
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
	}
}
