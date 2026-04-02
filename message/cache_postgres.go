package message

import (
	"time"

	"heckel.io/ntfy/v2/db"
)

// PostgreSQL runtime query constants
const (
	postgresInsertMessageQuery = `
		INSERT INTO message (mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user_id, content_type, encoding, published)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
	`
	postgresSelectScheduledMessageIDsBySeqIDQuery = `SELECT mid FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresDeleteScheduledBySequenceIDQuery      = `DELETE FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresUpdateMessagesForTopicExpiryQuery     = `UPDATE message SET expires = $1 WHERE topic = $2`
	postgresSelectMessagesByIDQuery               = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE mid = $1
	`
	postgresSelectMessagesSinceTimeQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE topic = $1 AND time >= $2 AND published = TRUE
		ORDER BY time, id
	`
	postgresSelectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE topic = $1 AND time >= $2
		ORDER BY time, id
	`
	postgresSelectMessagesSinceIDQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE topic = $1
		  AND id > COALESCE((SELECT id FROM message WHERE mid = $2), 0)
		  AND published = TRUE
		ORDER BY time, id
	`
	postgresSelectMessagesSinceIDIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE topic = $1
		  AND (id > COALESCE((SELECT id FROM message WHERE mid = $2), 0) OR published = FALSE)
		ORDER BY time, id
	`
	postgresSelectMessagesLatestQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE topic = $1 AND published = TRUE
		ORDER BY time DESC, id DESC
		LIMIT 1
	`
	postgresSelectMessagesDueQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE time <= $1 AND published = FALSE
		ORDER BY time, id
	`
	postgresUpdateMessagePublishedQuery = `UPDATE message SET published = TRUE WHERE mid = $1`
	postgresSelectMessagesCountQuery    = `SELECT COUNT(*) FROM message`
	postgresSelectTopicsQuery           = `SELECT topic FROM message GROUP BY topic`

	postgresDeleteExpiredMessagesQuery         = `DELETE FROM message WHERE mid IN (SELECT mid FROM message WHERE expires <= $1 AND published = TRUE LIMIT $2)`
	postgresMarkExpiredAttachmentsDeletedQuery = `UPDATE message SET attachment_deleted = TRUE WHERE mid IN (SELECT mid FROM message WHERE attachment_expires > 0 AND attachment_expires <= $1 AND attachment_deleted = FALSE LIMIT $2)`
	postgresSelectAttachmentsSizeBySenderQuery = `SELECT COALESCE(SUM(attachment_size), 0) FROM message WHERE user_id = '' AND sender = $1 AND attachment_expires >= $2`
	postgresSelectAttachmentsSizeByUserIDQuery = `SELECT COALESCE(SUM(attachment_size), 0) FROM message WHERE user_id = $1 AND attachment_expires >= $2`
	postgresSelectAttachmentsWithSizesQuery    = `SELECT mid, attachment_size FROM message WHERE attachment_expires > $1 AND attachment_deleted = FALSE`

	postgresSelectStatsQuery       = `SELECT value FROM message_stats WHERE key = 'messages'`
	postgresUpdateStatsQuery       = `UPDATE message_stats SET value = $1 WHERE key = 'messages'`
	postgresUpdateMessageTimeQuery = `UPDATE message SET time = $1 WHERE mid = $2`
)

var postgresQueries = queries{
	insertMessage:                    postgresInsertMessageQuery,
	selectScheduledMessageIDsBySeqID: postgresSelectScheduledMessageIDsBySeqIDQuery,
	deleteScheduledBySequenceID:      postgresDeleteScheduledBySequenceIDQuery,
	updateMessagesForTopicExpiry:     postgresUpdateMessagesForTopicExpiryQuery,
	selectMessagesByID:               postgresSelectMessagesByIDQuery,
	selectMessagesSinceTime:          postgresSelectMessagesSinceTimeQuery,
	selectMessagesSinceTimeScheduled: postgresSelectMessagesSinceTimeIncludeScheduledQuery,
	selectMessagesSinceID:            postgresSelectMessagesSinceIDQuery,
	selectMessagesSinceIDScheduled:   postgresSelectMessagesSinceIDIncludeScheduledQuery,
	selectMessagesLatest:             postgresSelectMessagesLatestQuery,
	selectMessagesDue:                postgresSelectMessagesDueQuery,
	deleteExpiredMessages:            postgresDeleteExpiredMessagesQuery,
	updateMessagePublished:           postgresUpdateMessagePublishedQuery,
	selectMessagesCount:              postgresSelectMessagesCountQuery,
	selectTopics:                     postgresSelectTopicsQuery,
	markExpiredAttachmentsDeleted:    postgresMarkExpiredAttachmentsDeletedQuery,
	selectAttachmentsSizeBySender:    postgresSelectAttachmentsSizeBySenderQuery,
	selectAttachmentsSizeByUserID:    postgresSelectAttachmentsSizeByUserIDQuery,
	selectAttachmentsWithSizes:       postgresSelectAttachmentsWithSizesQuery,
	selectStats:                      postgresSelectStatsQuery,
	updateStats:                      postgresUpdateStatsQuery,
	updateMessageTime:                postgresUpdateMessageTimeQuery,
}

// NewPostgresStore creates a new PostgreSQL-backed message cache store using an existing database connection pool.
func NewPostgresStore(d *db.DB, batchSize int, batchTimeout time.Duration) (*Cache, error) {
	if err := setupPostgres(d.Primary()); err != nil {
		return nil, err
	}
	return newCache(d, postgresQueries, nil, batchSize, batchTimeout, false), nil
}
