package message

import (
	"database/sql"
	"fmt"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
)

// Initial PostgreSQL schema
const (
	postgresCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS message (
			id BIGSERIAL PRIMARY KEY,
			mid TEXT NOT NULL,
			sequence_id TEXT NOT NULL,
			time BIGINT NOT NULL,
			event TEXT NOT NULL,
			expires BIGINT NOT NULL,
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
			attachment_size BIGINT NOT NULL,
			attachment_expires BIGINT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_deleted BOOLEAN NOT NULL DEFAULT FALSE,
			sender TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published BOOLEAN NOT NULL DEFAULT FALSE
		);
		CREATE INDEX IF NOT EXISTS idx_message_mid ON message (mid);
		CREATE INDEX IF NOT EXISTS idx_message_sequence_id ON message (sequence_id);
		CREATE INDEX IF NOT EXISTS idx_message_topic_published_time ON message (topic, published, time, id);
		CREATE INDEX IF NOT EXISTS idx_message_published_expires ON message (published, expires);
		CREATE INDEX IF NOT EXISTS idx_message_attachment_expires ON message (attachment_expires) WHERE attachment_deleted = FALSE;
		CREATE INDEX IF NOT EXISTS idx_message_sender_attachment_expires ON message (sender, attachment_expires) WHERE user_id = '';
		CREATE INDEX IF NOT EXISTS idx_message_user_id_attachment_expires ON message (user_id, attachment_expires);
		CREATE TABLE IF NOT EXISTS message_stats (
			key TEXT PRIMARY KEY,
			value BIGINT
		);
		INSERT INTO message_stats (key, value) VALUES ('messages', 0);
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
	`
)

// PostgreSQL schema management queries
const (
	postgresCurrentSchemaVersion     = 15
	postgresInsertSchemaVersionQuery = `INSERT INTO schema_version (store, version) VALUES ('message', $1)`
	postgresUpdateSchemaVersionQuery = `UPDATE schema_version SET version = $1 WHERE store = 'message'`
	postgresSelectSchemaVersionQuery = `SELECT version FROM schema_version WHERE store = 'message'`
)

// PostgreSQL schema migrations
const (
	// 14 -> 15
	postgresMigrate14To15CreateIndexQuery = `
		CREATE INDEX IF NOT EXISTS idx_message_attachment_expires ON message (attachment_expires) WHERE attachment_deleted = FALSE;
	`
)

var postgresMigrations = map[int]func(d *sql.DB) error{
	14: postgresMigrateFrom14,
}

func setupPostgres(d *sql.DB) error {
	var schemaVersion int
	if err := d.QueryRow(postgresSelectSchemaVersionQuery).Scan(&schemaVersion); err != nil {
		return setupNewPostgresDB(d)
	} else if schemaVersion == postgresCurrentSchemaVersion {
		return nil
	} else if schemaVersion > postgresCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, postgresCurrentSchemaVersion)
	}
	for i := schemaVersion; i < postgresCurrentSchemaVersion; i++ {
		fn, ok := postgresMigrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(d); err != nil {
			return err
		}
	}
	return nil
}

func postgresMigrateFrom14(d *sql.DB) error {
	log.Tag(tagMessageCache).Info("Migrating message cache database schema: from 14 to 15")
	return db.ExecTx(d, func(tx *sql.Tx) error {
		if _, err := tx.Exec(postgresMigrate14To15CreateIndexQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(postgresUpdateSchemaVersionQuery, 15); err != nil {
			return err
		}
		return nil
	})
}

func setupNewPostgresDB(sqlDB *sql.DB) error {
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(postgresCreateTablesQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(postgresInsertSchemaVersionQuery, postgresCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}
