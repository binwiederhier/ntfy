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
	postgresInsertAttachmentQuery = `
		INSERT INTO message_attachment (mid, aid, position, name, type, size, expires, url, deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	postgresSelectScheduledMessageIDsBySeqIDQuery    = `SELECT mid FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresSelectScheduledAttachmentIDsBySeqIDQuery = `SELECT DISTINCT COALESCE(NULLIF(a.aid, ''), m.mid) FROM message m LEFT JOIN message_attachment a ON a.mid = m.mid WHERE m.topic = $1 AND m.sequence_id = $2 AND m.published = FALSE`
	postgresDeleteScheduledBySequenceIDQuery         = `DELETE FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresUpdateMessagesForTopicExpiryQuery        = `UPDATE message SET expires = $1 WHERE topic = $2`
	postgresSelectMessagesByIDQuery                  = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding
		FROM message
		WHERE mid = $1
	`
	postgresSelectMessagesByAttachmentIDQuery = `
		SELECT m.mid, m.sequence_id, m.time, m.event, m.expires, m.topic, m.message, m.title, m.priority, m.tags, m.click, m.icon, m.actions, m.attachment_name, m.attachment_type, m.attachment_size, m.attachment_expires, m.attachment_url, m.sender, m.user_id, m.content_type, m.encoding
		FROM message m
		JOIN message_attachment a ON a.mid = m.mid
		WHERE a.aid = $1
	`
	postgresSelectAttachmentsByMessageIDQuery = `
		SELECT aid, name, type, size, expires, url
		FROM message_attachment
		WHERE mid = $1 AND deleted = FALSE
		ORDER BY position, id
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
	postgresMarkExpiredAttachmentsDeletedQuery = `UPDATE message_attachment SET deleted = TRUE WHERE id IN (SELECT id FROM message_attachment WHERE expires > 0 AND expires <= $1 AND deleted = FALSE LIMIT $2)`
	postgresSelectAttachmentsSizeBySenderQuery = `SELECT COALESCE(SUM(a.size), 0) FROM message_attachment a JOIN message m ON m.mid = a.mid WHERE m.user_id = '' AND m.sender = $1 AND a.expires >= $2 AND a.deleted = FALSE`
	postgresSelectAttachmentsSizeByUserIDQuery = `SELECT COALESCE(SUM(a.size), 0) FROM message_attachment a JOIN message m ON m.mid = a.mid WHERE m.user_id = $1 AND a.expires >= $2 AND a.deleted = FALSE`
	postgresSelectAttachmentsWithSizesQuery    = `SELECT a.aid, a.size FROM message_attachment a JOIN message m ON m.mid = a.mid WHERE a.expires > $1 AND a.deleted = FALSE AND a.aid <> ''`

	postgresSelectStatsQuery       = `SELECT value FROM message_stats WHERE key = 'messages'`
	postgresUpdateStatsQuery       = `UPDATE message_stats SET value = $1 WHERE key = 'messages'`
	postgresUpdateMessageTimeQuery = `UPDATE message SET time = $1 WHERE mid = $2`
)

var postgresQueries = queries{
	insertMessage:                       postgresInsertMessageQuery,
	insertAttachment:                    postgresInsertAttachmentQuery,
	selectScheduledMessageIDsBySeqID:    postgresSelectScheduledMessageIDsBySeqIDQuery,
	selectScheduledAttachmentIDsBySeqID: postgresSelectScheduledAttachmentIDsBySeqIDQuery,
	deleteScheduledBySequenceID:         postgresDeleteScheduledBySequenceIDQuery,
	updateMessagesForTopicExpiry:        postgresUpdateMessagesForTopicExpiryQuery,
	selectMessagesByID:                  postgresSelectMessagesByIDQuery,
	selectMessagesByAttachmentID:        postgresSelectMessagesByAttachmentIDQuery,
	selectAttachmentsByMessageID:        postgresSelectAttachmentsByMessageIDQuery,
	selectMessagesSinceTime:             postgresSelectMessagesSinceTimeQuery,
	selectMessagesSinceTimeScheduled:    postgresSelectMessagesSinceTimeIncludeScheduledQuery,
	selectMessagesSinceID:               postgresSelectMessagesSinceIDQuery,
	selectMessagesSinceIDScheduled:      postgresSelectMessagesSinceIDIncludeScheduledQuery,
	selectMessagesLatest:                postgresSelectMessagesLatestQuery,
	selectMessagesDue:                   postgresSelectMessagesDueQuery,
	deleteExpiredMessages:               postgresDeleteExpiredMessagesQuery,
	updateMessagePublished:              postgresUpdateMessagePublishedQuery,
	selectMessagesCount:                 postgresSelectMessagesCountQuery,
	selectTopics:                        postgresSelectTopicsQuery,
	markExpiredAttachmentsDeleted:       postgresMarkExpiredAttachmentsDeletedQuery,
	selectAttachmentsSizeBySender:       postgresSelectAttachmentsSizeBySenderQuery,
	selectAttachmentsSizeByUserID:       postgresSelectAttachmentsSizeByUserIDQuery,
	selectAttachmentsWithSizes:          postgresSelectAttachmentsWithSizesQuery,
	selectStats:                         postgresSelectStatsQuery,
	updateStats:                         postgresUpdateStatsQuery,
	updateMessageTime:                   postgresUpdateMessageTimeQuery,
}

// NewPostgresStore creates a new PostgreSQL-backed message cache store using an existing database connection pool.
func NewPostgresStore(d *db.DB, batchSize int, batchTimeout time.Duration) (*Cache, error) {
	if err := setupPostgres(d.Primary()); err != nil {
		return nil, err
	}
	return newCache(d, postgresQueries, nil, batchSize, batchTimeout, false), nil
}
