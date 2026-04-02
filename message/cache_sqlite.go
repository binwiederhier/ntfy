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
	sqliteSelectScheduledMessageIDsBySeqIDQuery = `SELECT mid FROM messages WHERE topic = ? AND sequence_id = ? AND published = 0`
	sqliteDeleteScheduledBySequenceIDQuery      = `DELETE FROM messages WHERE topic = ? AND sequence_id = ? AND published = 0`
	sqliteUpdateMessagesForTopicExpiryQuery     = `UPDATE messages SET expires = ? WHERE topic = ?`
	sqliteSelectMessagesByIDQuery               = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, content_type, encoding
		FROM messages
		WHERE mid = ?
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
	sqliteMarkExpiredAttachmentsDeletedQuery = `UPDATE messages SET attachment_deleted = 1 WHERE mid IN (SELECT mid FROM messages WHERE attachment_expires > 0 AND attachment_expires <= ? AND attachment_deleted = 0 LIMIT ?)`
	sqliteSelectAttachmentsSizeBySenderQuery = `SELECT IFNULL(SUM(attachment_size), 0) FROM messages WHERE user = '' AND sender = ? AND attachment_expires >= ?`
	sqliteSelectAttachmentsSizeByUserIDQuery = `SELECT IFNULL(SUM(attachment_size), 0) FROM messages WHERE user = ? AND attachment_expires >= ?`
	sqliteSelectAttachmentsWithSizesQuery    = `SELECT mid, attachment_size FROM messages WHERE attachment_expires > ? AND attachment_deleted = 0`

	sqliteSelectStatsQuery       = `SELECT value FROM stats WHERE key = 'messages'`
	sqliteUpdateStatsQuery       = `UPDATE stats SET value = ? WHERE key = 'messages'`
	sqliteUpdateMessageTimeQuery = `UPDATE messages SET time = ? WHERE mid = ?`
)

var sqliteQueries = queries{
	insertMessage:                    sqliteInsertMessageQuery,
	selectScheduledMessageIDsBySeqID: sqliteSelectScheduledMessageIDsBySeqIDQuery,
	deleteScheduledBySequenceID:      sqliteDeleteScheduledBySequenceIDQuery,
	updateMessagesForTopicExpiry:     sqliteUpdateMessagesForTopicExpiryQuery,
	selectMessagesByID:               sqliteSelectMessagesByIDQuery,
	selectMessagesSinceTime:          sqliteSelectMessagesSinceTimeQuery,
	selectMessagesSinceTimeScheduled: sqliteSelectMessagesSinceTimeIncludeScheduledQuery,
	selectMessagesSinceID:            sqliteSelectMessagesSinceIDQuery,
	selectMessagesSinceIDScheduled:   sqliteSelectMessagesSinceIDIncludeScheduledQuery,
	selectMessagesLatest:             sqliteSelectMessagesLatestQuery,
	selectMessagesDue:                sqliteSelectMessagesDueQuery,
	deleteExpiredMessages:            sqliteDeleteExpiredMessagesQuery,
	updateMessagePublished:           sqliteUpdateMessagePublishedQuery,
	selectMessagesCount:              sqliteSelectMessagesCountQuery,
	selectTopics:                     sqliteSelectTopicsQuery,
	markExpiredAttachmentsDeleted:    sqliteMarkExpiredAttachmentsDeletedQuery,
	selectAttachmentsSizeBySender:    sqliteSelectAttachmentsSizeBySenderQuery,
	selectAttachmentsSizeByUserID:    sqliteSelectAttachmentsSizeByUserIDQuery,
	selectAttachmentsWithSizes:       sqliteSelectAttachmentsWithSizesQuery,
	selectStats:                      sqliteSelectStatsQuery,
	updateStats:                      sqliteUpdateStatsQuery,
	updateMessageTime:                sqliteUpdateMessageTimeQuery,
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
