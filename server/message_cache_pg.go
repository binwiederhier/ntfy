package server

import (
	"database/sql"
	_ "github.com/lib/pq" // PostgreSQL driver
	"heckel.io/ntfy/v2/util"
	"time"
)

// Messages cache
const (
	pgCreateMessagesTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			mid TEXT NOT NULL,
			time INT NOT NULL,
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
			attachment_deleted BOOLEAN NOT NULL,
			sender TEXT NOT NULL,
			"user" TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published BOOLEAN NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages (mid);
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_sender ON messages (sender);
		CREATE INDEX IF NOT EXISTS idx_user ON messages ("user");
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
		CREATE TABLE IF NOT EXISTS stats (
			key TEXT PRIMARY KEY,
			value INT
		);
		INSERT INTO stats (key, value) VALUES ('messages', 0);
		COMMIT;
	`

	pgSelectMessagesCountQuery = `SELECT COUNT(*) FROM messages`
)

var (
	pgMessageCacheQueries = &messageCacheQueries{
		insertMessage: `
			INSERT INTO messages (mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, "user", content_type, encoding, published)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		`,
		deleteMessage:                `DELETE FROM messages WHERE mid = $1`,
		updateMessagesForTopicExpiry: `UPDATE messages SET expires = $1 WHERE topic = $2`,
		selectRowIDFromMessageID:     `SELECT id FROM messages WHERE mid = $1`, // Do not include topic, see #336 and TestServer_PollSinceID_MultipleTopics
		selectMessagesByID: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE mid = $1
	`,
		selectMessagesSinceTime: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE topic = $1 AND time >= $2 AND published = TRUE
		ORDER BY time, id
	`,
		selectMessagesSinceTimeIncludeScheduled: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE topic = $1 AND time >= $2
		ORDER BY time, id
	`,
		selectMessagesSinceID: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE topic = $1 AND id > $2 AND published = TRUE
		ORDER BY time, id
	`,
		selectMessagesSinceIDIncludeScheduled: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE topic = $1 AND (id > $2 OR published = FALSE)
		ORDER BY time, id
	`,
		selectMessagesDue: `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, "user", content_type, encoding
		FROM messages
		WHERE time <= $1 AND published = FALSE
		ORDER BY time, id
	`,
		selectMessagesExpired:      `SELECT mid FROM messages WHERE expires <= $1 AND published = TRUE`,
		updateMessagePublished:     `UPDATE messages SET published = TRUE WHERE mid = $1`,
		selectMessageCountPerTopic: `SELECT topic, COUNT(*) FROM messages GROUP BY topic`,
		selectTopics:               `SELECT topic FROM messages GROUP BY topic`,

		updateAttachmentDeleted:       `UPDATE messages SET attachment_deleted = TRUE WHERE mid = $1`,
		selectAttachmentsExpired:      `SELECT mid FROM messages WHERE attachment_expires > 0 AND attachment_expires <= $1 AND attachment_deleted = FALSE`,
		selectAttachmentsSizeBySender: `SELECT COALESCE(SUM(attachment_size), 0) FROM messages WHERE "user" = '' AND sender = $1 AND attachment_expires >= $2`,
		selectAttachmentsSizeByUserID: `SELECT COALESCE(SUM(attachment_size), 0) FROM messages WHERE "user" = $1 AND attachment_expires >= $2`,

		selectStats: `SELECT value FROM stats WHERE key = 'messages'`,
		updateStats: `UPDATE stats SET value = $1 WHERE key = 'messages'`,
	}
)

type pgMessageCache struct {
	*commonMessageCache
}

var _ MessageCache = (*pgMessageCache)(nil)

func newPgMessageCache(connectionString, startupQueries string, batchSize int, batchTimeout time.Duration) (*pgMessageCache, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	if err := setupPgMessagesDB(db, startupQueries); err != nil {
		return nil, err
	}
	var queue *util.BatchingQueue[*message]
	if batchSize > 0 || batchTimeout > 0 {
		queue = util.NewBatchingQueue[*message](batchSize, batchTimeout)
	}
	cache := &pgMessageCache{
		commonMessageCache: &commonMessageCache{
			db:      db,
			queue:   queue,
			queries: pgMessageCacheQueries,
		},
	}
	go cache.processMessageBatches()
	return cache, nil
}

func setupPgMessagesDB(db *sql.DB, startupQueries string) error {
	// Run startup queries
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}

	// If 'messages' table does not exist, this must be a new database
	rowsMC, err := db.Query(pgSelectMessagesCountQuery)
	if err != nil {
		return setupNewPgCacheDB(db)
	}
	rowsMC.Close()

	return nil

	// FIXME schema migration
}

func setupNewPgCacheDB(db *sql.DB) error {
	if _, err := db.Exec(pgCreateMessagesTableQuery); err != nil {
		return err
	}
	/*
		// FIXME
		if _, err := db.Exec(pgCreateSchemaVersionTableQuery); err != nil {
			return err
		}
		if _, err := db.Exec(insertSchemaVersion, currentSchemaVersion); err != nil {
			return err
		}
	*/
	return nil
}
