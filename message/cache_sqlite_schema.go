package message

import (
	"database/sql"
	"time"

	"heckel.io/ntfy/v2/db/schema"
)

// Initial SQLite schema
const (
	sqliteCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			sequence_id TEXT NOT NULL,
			time INT NOT NULL,
			event TEXT NOT NULL,
			expires INT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			icon TEXT NOT NULL,
			actions TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size INT NOT NULL,
			attachment_expires INT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_deleted INT NOT NULL,
			sender TEXT NOT NULL,
			user TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			apple TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages (mid);
		CREATE INDEX IF NOT EXISTS idx_sequence_id ON messages (sequence_id);
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		CREATE INDEX IF NOT EXISTS idx_topic_published_time ON messages (topic, published, time);
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_sender ON messages (sender);
		CREATE INDEX IF NOT EXISTS idx_user ON messages (user);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
		CREATE TABLE IF NOT EXISTS stats (
			key TEXT PRIMARY KEY,
			value INT
		);
		INSERT INTO stats (key, value) VALUES ('messages', 0);
	`
)

// Schema migrations for SQLite. Databases older than schema version 1 (ntfy < v1.10.0,
// November 2021) can no longer be migrated.
const (
	// 1 -> 2
	sqliteMigrate1To2AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN published INT NOT NULL DEFAULT(1);
	`

	// 2 -> 3
	sqliteMigrate2To3AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN click TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_name TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_type TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_size INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_expires INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_owner TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_url TEXT NOT NULL DEFAULT('');
	`
	// 3 -> 4
	sqliteMigrate3To4AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN encoding TEXT NOT NULL DEFAULT('');
	`

	// 4 -> 5
	sqliteMigrate4To5AlterMessagesTableQuery = `
		CREATE TABLE IF NOT EXISTS messages_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			time INT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size INT NOT NULL,
			attachment_expires INT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_owner TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages_new (mid);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages_new (topic);
		INSERT
			INTO messages_new (
				mid, time, topic, message, title, priority, tags, click, attachment_name, attachment_type,
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published)
			SELECT
				id, time, topic, message, title, priority, tags, click, attachment_name, attachment_type,
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published
			FROM messages;
		DROP TABLE messages;
		ALTER TABLE messages_new RENAME TO messages;
	`

	// 5 -> 6
	sqliteMigrate5To6AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN actions TEXT NOT NULL DEFAULT('');
	`

	// 6 -> 7
	sqliteMigrate6To7AlterMessagesTableQuery = `
		ALTER TABLE messages RENAME COLUMN attachment_owner TO sender;
	`

	// 7 -> 8
	sqliteMigrate7To8AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN icon TEXT NOT NULL DEFAULT('');
	`

	// 8 -> 9
	sqliteMigrate8To9AlterMessagesTableQuery = `
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
	`

	// 9 -> 10
	sqliteMigrate9To10AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN user TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_deleted INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN expires INT NOT NULL DEFAULT('0');
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_sender ON messages (sender);
		CREATE INDEX IF NOT EXISTS idx_user ON messages (user);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
	`
	sqliteMigrate9To10UpdateMessageExpiryQuery = `UPDATE messages SET expires = time + ?`

	// 10 -> 11
	sqliteMigrate10To11AlterMessagesTableQuery = `
		CREATE TABLE IF NOT EXISTS stats (
			key TEXT PRIMARY KEY,
			value INT
		);
		INSERT INTO stats (key, value) VALUES ('messages', 0);
	`

	// 11 -> 12
	sqliteMigrate11To12AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN content_type TEXT NOT NULL DEFAULT('');
	`

	// 12 -> 13
	sqliteMigrate12To13AlterMessagesTableQuery = `
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
	`

	// 13 -> 14
	sqliteMigrate13To14AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN sequence_id TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN event TEXT NOT NULL DEFAULT('message');
		CREATE INDEX IF NOT EXISTS idx_sequence_id ON messages (sequence_id);
	`

	// 16 -> 17: serves the since=<id> replay, like Postgres' idx_message_topic_published_time
	sqliteMigrate16To17CreateIndexQuery = `
		CREATE INDEX IF NOT EXISTS idx_topic_published_time ON messages (topic, published, time);
	`

	// 17 -> 18: adds the apple column for the X-Apple-* headers
	sqliteMigrate17To18AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN apple TEXT NOT NULL DEFAULT('');
	`
)

var (
	sqliteCreateTables = schema.AsMigrateFunc(sqliteCreateTablesQuery)
)

// sqliteMigrations returns the migration steps, keyed by the version they upgrade FROM. The
// cache duration is carried into the 9 -> 10 step via closure (it backfills "expires" from it).
// Always append migrations at the end, never insert in the middle.
func sqliteMigrations(cacheDuration time.Duration) schema.Migrations {
	return schema.Migrations{
		{From: 1, To: 2}: schema.Query(sqliteMigrate1To2AlterMessagesTableQuery),
		{From: 2, To: 3}: schema.Query(sqliteMigrate2To3AlterMessagesTableQuery),
		{From: 3, To: 4}: schema.Query(sqliteMigrate3To4AlterMessagesTableQuery),
		{From: 4, To: 5}: schema.Query(sqliteMigrate4To5AlterMessagesTableQuery),
		{From: 5, To: 6}: schema.Query(sqliteMigrate5To6AlterMessagesTableQuery),
		{From: 6, To: 7}: schema.Query(sqliteMigrate6To7AlterMessagesTableQuery),
		{From: 7, To: 8}: schema.Query(sqliteMigrate7To8AlterMessagesTableQuery),
		{From: 8, To: 9}: schema.Query(sqliteMigrate8To9AlterMessagesTableQuery),
		{From: 9, To: 10}: schema.Func(func(tx *sql.Tx) error {
			if _, err := tx.Exec(sqliteMigrate9To10AlterMessagesTableQuery); err != nil {
				return err
			}
			_, err := tx.Exec(sqliteMigrate9To10UpdateMessageExpiryQuery, int64(cacheDuration.Seconds()))
			return err
		}),
		{From: 10, To: 11}: schema.Query(sqliteMigrate10To11AlterMessagesTableQuery),
		{From: 11, To: 12}: schema.Query(sqliteMigrate11To12AlterMessagesTableQuery),
		{From: 12, To: 13}: schema.Query(sqliteMigrate12To13AlterMessagesTableQuery),
		{From: 13, To: 14}: schema.Query(sqliteMigrate13To14AlterMessagesTableQuery),
		{From: 14, To: 15}: schema.Nop, // Corresponds to Postgres migration
		{From: 15, To: 16}: schema.Nop, // 15 -> 16 adds the Postgres claim column; SQLite is single-node
		{From: 16, To: 17}: schema.Query(sqliteMigrate16To17CreateIndexQuery),
		{From: 17, To: 18}: schema.Query(sqliteMigrate17To18AlterMessagesTableQuery),
	}
}

func runSQLiteStartupQueries(db *sql.DB, startupQueries string) error {
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}
	return nil
}
