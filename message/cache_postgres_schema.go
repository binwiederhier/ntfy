package message

import (
	"heckel.io/ntfy/v2/db/schema"
)

// Initial PostgreSQL schema
const (
	postgresCurrentSchemaVersion = 15
	postgresCreateTablesQuery    = `
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
	`
)

// PostgreSQL schema migrations
const (
	// 14 -> 15
	postgresMigrate14To15CreateIndexQuery = `
		CREATE INDEX IF NOT EXISTS idx_message_attachment_expires ON message (attachment_expires) WHERE attachment_deleted = FALSE;
	`
)

var (
	postgresCreateTables = schema.AsMigrateFunc(postgresCreateTablesQuery)

	// postgresMigrations maps a schema version to the migration upgrading it to the next
	// version. Always append migrations at the end, never insert in the middle.
	postgresMigrations = map[int]schema.MigrateFunc{
		14: schema.AsMigrateFunc(postgresMigrate14To15CreateIndexQuery),
	}
)
