package message

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/util"
)

// SQLite runtime query constants
const (
	sqliteInsertMessageQuery = `
		INSERT INTO messages (mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user, content_type, encoding, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	sqliteInsertAttachmentQuery = `
		INSERT INTO message_attachments (mid, aid, position, name, type, size, expires, url, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	sqliteSelectScheduledMessageIDsBySeqIDQuery    = `SELECT mid FROM messages WHERE topic = ? AND sequence_id = ? AND published = 0`
	sqliteSelectScheduledAttachmentIDsBySeqIDQuery = `SELECT DISTINCT COALESCE(NULLIF(a.aid, ''), m.mid) FROM messages m LEFT JOIN message_attachments a ON a.mid = m.mid WHERE m.topic = ? AND m.sequence_id = ? AND m.published = 0`
	sqliteDeleteScheduledBySequenceIDQuery         = `DELETE FROM messages WHERE topic = ? AND sequence_id = ? AND published = 0`
	sqliteUpdateMessagesForTopicExpiryQuery        = `UPDATE messages SET expires = ? WHERE topic = ?`
	sqliteSelectMessagesByIDQuery                  = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE mid = ?
	`
	sqliteSelectMessagesByAttachmentIDQuery = `
		SELECT m.mid, m.sequence_id, m.time, m.event, m.expires, m.topic, m.message, m.title, m.priority, m.tags, m.click, m.icon, m.actions, m.attachment_name, m.attachment_type, m.attachment_size, m.attachment_expires, m.attachment_url, m.sender, m.user, m.content_type, m.encoding
		FROM messages m
		JOIN message_attachments a ON a.mid = m.mid
		WHERE a.aid = ?
	`
	sqliteSelectAttachmentsByMessageIDQuery = `
		SELECT aid, name, type, size, expires, url
		FROM message_attachments
		WHERE mid = ? AND deleted = 0
		ORDER BY position, id
	`
	sqliteSelectMessagesSinceTimeQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE topic = ? AND time >= ? AND published = 1
		ORDER BY time, id
	`
	sqliteSelectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE topic = ? AND time >= ?
		ORDER BY time, id
	`
	sqliteSelectMessagesSinceIDQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE topic = ? AND id > COALESCE((SELECT id FROM messages WHERE mid = ?), 0) AND published = 1
		ORDER BY time, id
	`
	sqliteSelectMessagesSinceIDIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE topic = ? AND (id > COALESCE((SELECT id FROM messages WHERE mid = ?), 0) OR published = 0)
		ORDER BY time, id
	`
	sqliteSelectMessagesLatestQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE topic = ? AND published = 1
		ORDER BY time DESC, id DESC
		LIMIT 1
	`
	sqliteSelectMessagesDueQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE time <= ? AND published = 0
		ORDER BY time, id
	`
	sqliteUpdateMessagePublishedQuery = `UPDATE messages SET published = 1 WHERE mid = ?`
	sqliteSelectMessagesCountQuery    = `SELECT COUNT(*) FROM messages`
	sqliteSelectTopicsQuery           = `SELECT topic FROM messages GROUP BY topic`

	sqliteDeleteExpiredMessagesQuery         = `DELETE FROM messages WHERE mid IN (SELECT mid FROM messages WHERE expires <= ? AND published = 1 LIMIT ?)`
	sqliteMarkExpiredAttachmentsDeletedQuery = `UPDATE message_attachments SET deleted = 1 WHERE id IN (SELECT id FROM message_attachments WHERE expires > 0 AND expires <= ? AND deleted = 0 LIMIT ?)`
	sqliteSelectAttachmentsSizeBySenderQuery = `SELECT IFNULL(SUM(a.size), 0) FROM message_attachments a JOIN messages m ON m.mid = a.mid WHERE m.user = '' AND m.sender = ? AND a.expires >= ? AND a.deleted = 0`
	sqliteSelectAttachmentsSizeByUserIDQuery = `SELECT IFNULL(SUM(a.size), 0) FROM message_attachments a JOIN messages m ON m.mid = a.mid WHERE m.user = ? AND a.expires >= ? AND a.deleted = 0`
	sqliteSelectAttachmentsWithSizesQuery    = `SELECT a.aid, a.size FROM message_attachments a JOIN messages m ON m.mid = a.mid WHERE a.expires > ? AND a.deleted = 0 AND a.aid <> ''`

	sqliteSelectStatsQuery       = `SELECT value FROM stats WHERE key = 'messages'`
	sqliteUpdateStatsQuery       = `UPDATE stats SET value = ? WHERE key = 'messages'`
	sqliteUpdateMessageTimeQuery = `UPDATE messages SET time = ? WHERE mid = ?`
)

var sqliteQueries = queries{
	insertMessage:                       sqliteInsertMessageQuery,
	insertAttachment:                    sqliteInsertAttachmentQuery,
	selectScheduledMessageIDsBySeqID:    sqliteSelectScheduledMessageIDsBySeqIDQuery,
	selectScheduledAttachmentIDsBySeqID: sqliteSelectScheduledAttachmentIDsBySeqIDQuery,
	deleteScheduledBySequenceID:         sqliteDeleteScheduledBySequenceIDQuery,
	updateMessagesForTopicExpiry:        sqliteUpdateMessagesForTopicExpiryQuery,
	selectMessagesByID:                  sqliteSelectMessagesByIDQuery,
	selectMessagesByAttachmentID:        sqliteSelectMessagesByAttachmentIDQuery,
	selectAttachmentsByMessageID:        sqliteSelectAttachmentsByMessageIDQuery,
	selectMessagesSinceTime:             sqliteSelectMessagesSinceTimeQuery,
	selectMessagesSinceTimeScheduled:    sqliteSelectMessagesSinceTimeIncludeScheduledQuery,
	selectMessagesSinceID:               sqliteSelectMessagesSinceIDQuery,
	selectMessagesSinceIDScheduled:      sqliteSelectMessagesSinceIDIncludeScheduledQuery,
	selectMessagesLatest:                sqliteSelectMessagesLatestQuery,
	selectMessagesDue:                   sqliteSelectMessagesDueQuery,
	deleteExpiredMessages:               sqliteDeleteExpiredMessagesQuery,
	updateMessagePublished:              sqliteUpdateMessagePublishedQuery,
	selectMessagesCount:                 sqliteSelectMessagesCountQuery,
	selectTopics:                        sqliteSelectTopicsQuery,
	markExpiredAttachmentsDeleted:       sqliteMarkExpiredAttachmentsDeletedQuery,
	selectAttachmentsSizeBySender:       sqliteSelectAttachmentsSizeBySenderQuery,
	selectAttachmentsSizeByUserID:       sqliteSelectAttachmentsSizeByUserIDQuery,
	selectAttachmentsWithSizes:          sqliteSelectAttachmentsWithSizesQuery,
	selectStats:                         sqliteSelectStatsQuery,
	updateStats:                         sqliteUpdateStatsQuery,
	updateMessageTime:                   sqliteUpdateMessageTimeQuery,
}

// NewSQLiteStore creates a SQLite file-backed cache
func NewSQLiteStore(filename, startupQueries string, cacheDuration time.Duration, batchSize int, batchTimeout time.Duration, nop bool) (*Cache, error) {
	parentDir := filepath.Dir(filename)
	if !util.FileExists(parentDir) {
		return nil, fmt.Errorf("cache database directory %s does not exist or is not accessible", parentDir)
	}
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSQLite(d, startupQueries, cacheDuration); err != nil {
		return nil, err
	}
	return newCache(db.New(&db.Host{DB: d}, nil), sqliteQueries, &sync.Mutex{}, batchSize, batchTimeout, nop), nil
}

// NewMemStore creates an in-memory cache
func NewMemStore() (*Cache, error) {
	return NewSQLiteStore(createMemoryFilename(), "", 0, 0, 0, false)
}

// NewNopStore creates an in-memory cache that discards all messages;
// it is always empty and can be used if caching is entirely disabled
func NewNopStore() (*Cache, error) {
	return NewSQLiteStore(createMemoryFilename(), "", 0, 0, 0, true)
}

// createMemoryFilename creates a unique memory filename to use for the SQLite backend.
// From mattn/go-sqlite3: "Each connection to ":memory:" opens a brand new in-memory
// sql database, so if the stdlib's sql engine happens to open another connection and
// you've only specified ":memory:", that connection will see a brand new database.
// A workaround is to use "file::memory:?cache=shared" (or "file:foobar?mode=memory&cache=shared").
// Every connection to this string will point to the same in-memory database."
func createMemoryFilename() string {
	return fmt.Sprintf("file:%s?mode=memory&cache=shared", util.RandomString(10))
}
