package monitor

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/util"
)

const (
	sqliteCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS monitor (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			key TEXT NOT NULL,
			period_seconds INT NOT NULL,
			grace_seconds INT NOT NULL,
			alert_topic TEXT NOT NULL,
			alert_priority INT NOT NULL DEFAULT 4,
			state TEXT NOT NULL DEFAULT 'pending',
			last_seen_at INT NOT NULL DEFAULT 0,
			created_at INT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_monitor_user_key ON monitor (user_id, key);
		CREATE INDEX IF NOT EXISTS idx_monitor_user ON monitor (user_id);
		CREATE INDEX IF NOT EXISTS idx_monitor_state ON monitor (state);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
	`
	sqliteBuiltinStartupQueries = `PRAGMA foreign_keys = ON;`

	sqliteInsertMonitorQuery = `
		INSERT INTO monitor (user_id, key, period_seconds, grace_seconds, alert_topic, alert_priority, state, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	sqliteSelectMonitorColumns = `id, user_id, key, period_seconds, grace_seconds, alert_topic, alert_priority, state, last_seen_at, created_at`
	sqliteSelectMonitorByUserAndKeyQuery = `
		SELECT ` + sqliteSelectMonitorColumns + `
		FROM monitor
		WHERE user_id = ? AND key = ?
	`
	sqliteSelectMonitorsByUserQuery = `
		SELECT ` + sqliteSelectMonitorColumns + `
		FROM monitor
		WHERE user_id = ?
		ORDER BY key
	`
	sqliteSelectStaleMonitorsQuery = `
		SELECT ` + sqliteSelectMonitorColumns + `
		FROM monitor
		WHERE state = 'up' AND last_seen_at + period_seconds + grace_seconds < ?
	`
	sqliteUpdateMonitorHeartbeatQuery = `UPDATE monitor SET last_seen_at = ?, state = ? WHERE id = ?`
	sqliteUpdateMonitorStateQuery     = `UPDATE monitor SET state = ? WHERE id = ?`
	sqliteDeleteMonitorQuery          = `DELETE FROM monitor WHERE user_id = ? AND key = ?`
)

// SQLite schema management queries
const (
	sqliteCurrentSchemaVersion     = 1
	sqliteInsertSchemaVersionQuery = `INSERT INTO schemaVersion VALUES (1, ?)`
	sqliteSelectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
)

// NewSQLiteManager creates a new Manager backed by a SQLite database file.
func NewSQLiteManager(filename, startupQueries string) (*Manager, error) {
	parentDir := filepath.Dir(filename)
	if !util.FileExists(parentDir) {
		return nil, fmt.Errorf("monitor database directory %s does not exist or is not accessible", parentDir)
	}
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSQLite(d); err != nil {
		return nil, err
	}
	if err := runSQLiteStartupQueries(d, startupQueries); err != nil {
		return nil, err
	}
	return &Manager{db: db.New(&db.Host{DB: d}, nil)}, nil
}

func setupSQLite(sqlDB *sql.DB) error {
	var schemaVersion int
	if err := sqlDB.QueryRow(sqliteSelectSchemaVersionQuery).Scan(&schemaVersion); err != nil {
		return setupNewSQLite(sqlDB)
	}
	if schemaVersion > sqliteCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, sqliteCurrentSchemaVersion)
	}
	return nil
}

func setupNewSQLite(sqlDB *sql.DB) error {
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteCreateTablesQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteInsertSchemaVersionQuery, sqliteCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}

func runSQLiteStartupQueries(sqlDB *sql.DB, startupQueries string) error {
	if _, err := sqlDB.Exec(sqliteBuiltinStartupQueries); err != nil {
		return err
	}
	if startupQueries != "" {
		if _, err := sqlDB.Exec(startupQueries); err != nil {
			return err
		}
	}
	return nil
}

// isUniqueConstraintError reports whether err is a SQLite UNIQUE constraint violation.
// SQLite returns "UNIQUE constraint failed: ..." for these.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
