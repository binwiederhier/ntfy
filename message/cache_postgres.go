package message

import (
	"database/sql"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
	"heckel.io/ntfy/v2/model"
)

// PostgreSQL runtime query constants
const (
	// postgresInsertMessagesQuery writes a whole batch in one statement with one array parameter
	// per column, in the order of insertMessageArgs. The statement text is the same for every batch
	// size: pgx prepares each distinct text server-side and keeps up to 512 per connection, and a
	// multi-row INSERT per batch size left hundreds of multi-MB plans in every pooled backend.
	postgresInsertMessagesQuery = `
		INSERT INTO message (mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user_id, content_type, encoding, apple, published)
		SELECT * FROM unnest($1::TEXT[], $2::TEXT[], $3::BIGINT[], $4::TEXT[], $5::BIGINT[], $6::TEXT[], $7::TEXT[], $8::TEXT[], $9::INT[], $10::TEXT[], $11::TEXT[], $12::TEXT[], $13::TEXT[], $14::TEXT[], $15::TEXT[], $16::BIGINT[], $17::BIGINT[], $18::TEXT[], $19::BOOLEAN[], $20::TEXT[], $21::TEXT[], $22::TEXT[], $23::TEXT[], $24::TEXT[], $25::BOOLEAN[])
	`
	postgresSelectScheduledMessageIDsBySeqIDQuery = `SELECT mid FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresDeleteScheduledBySequenceIDQuery      = `DELETE FROM message WHERE topic = $1 AND sequence_id = $2 AND published = FALSE`
	postgresUpdateMessagesForTopicExpiryQuery     = `UPDATE message SET expires = $1 WHERE topic = $2`
	postgresSelectMessagesByIDQuery               = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE mid = $1
	`
	postgresSelectMessagesSinceTimeQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE topic = $1 AND time >= $2 AND published = TRUE
		ORDER BY time DESC, id DESC
	`
	postgresSelectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE topic = $1 AND time >= $2
		ORDER BY time DESC, id DESC
	`
	postgresSelectMessageRowIDQuery    = `SELECT id FROM message WHERE mid = $1 LIMIT 1`
	postgresSelectMessagesSinceIDQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE topic = $1
		  AND id > $2
		  AND published = TRUE
		ORDER BY time DESC, id DESC
	`
	postgresSelectMessagesSinceIDIncludeScheduledQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE topic = $1
		  AND (id > $2 OR published = FALSE)
		ORDER BY time DESC, id DESC
	`
	postgresSelectMessagesLatestQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE topic = $1 AND published = TRUE
		ORDER BY time DESC, id DESC
		LIMIT 1
	`
	postgresSelectMessagesDueQuery = `
		SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
		FROM message
		WHERE time <= $1 AND published = FALSE
		ORDER BY time, id
	`
	postgresUpdateMessagePublishedQuery = `UPDATE message SET published = TRUE WHERE mid = $1`
	// Planner estimate, since a COUNT(*) scans the whole table; reltuples is -1 if never analyzed
	postgresSelectMessagesCountQuery = `
		SELECT CASE WHEN reltuples < 0 THEN (SELECT COUNT(*) FROM message) ELSE reltuples::BIGINT END
		FROM pg_class
		WHERE oid = 'message'::regclass
	`
	postgresSelectTopicsQuery = `SELECT topic FROM message GROUP BY topic`

	postgresDeleteExpiredMessagesQuery         = `DELETE FROM message WHERE mid IN (SELECT mid FROM message WHERE expires <= $1 AND published = TRUE LIMIT $2)`
	postgresMarkExpiredAttachmentsDeletedQuery = `UPDATE message SET attachment_deleted = TRUE WHERE mid IN (SELECT mid FROM message WHERE attachment_expires > 0 AND attachment_expires <= $1 AND attachment_deleted = FALSE LIMIT $2)`
	postgresSelectAttachmentsSizeBySenderQuery = `SELECT COALESCE(SUM(attachment_size), 0) FROM message WHERE user_id = '' AND sender = $1 AND attachment_expires >= $2`
	postgresSelectAttachmentsSizeByUserIDQuery = `SELECT COALESCE(SUM(attachment_size), 0) FROM message WHERE user_id = $1 AND attachment_expires >= $2`
	postgresSelectAttachmentsWithSizesQuery    = `SELECT mid, attachment_size FROM message WHERE attachment_expires > $1 AND attachment_deleted = FALSE`

	postgresSelectStatsQuery = `SELECT value FROM message_stats WHERE key = 'messages'`
	// postgresClaimMessagesDueQuery claims due rows in one statement: the inner SELECT locks what
	// it picks and skips what another node is claiming, so concurrent senders get disjoint sets.
	// Unclaimed rows have claimed_at 0, so the cutoff ($2) matches them too.
	postgresClaimMessagesDueQuery = `
		WITH due AS (
			SELECT id FROM message
			WHERE time <= $1 AND published = FALSE AND claimed_at <= $2
			ORDER BY time, id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE message SET claimed_at = $4 WHERE id IN (SELECT id FROM due)
		RETURNING mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, apple
	`

	postgresUpdateStatsQuery       = `UPDATE message_stats SET value = $1 WHERE key = 'messages'`
	postgresUpdateMessageTimeQuery = `UPDATE message SET time = $1 WHERE mid = $2`
)

var postgresQueries = queries{
	insertMessages:                   postgresInsertMessages,
	selectScheduledMessageIDsBySeqID: postgresSelectScheduledMessageIDsBySeqIDQuery,
	deleteScheduledBySequenceID:      postgresDeleteScheduledBySequenceIDQuery,
	updateMessagesForTopicExpiry:     postgresUpdateMessagesForTopicExpiryQuery,
	selectMessagesByID:               postgresSelectMessagesByIDQuery,
	selectMessagesSinceTime:          postgresSelectMessagesSinceTimeQuery,
	selectMessagesSinceTimeScheduled: postgresSelectMessagesSinceTimeIncludeScheduledQuery,
	selectMessageRowID:               postgresSelectMessageRowIDQuery,
	selectMessagesSinceID:            postgresSelectMessagesSinceIDQuery,
	selectMessagesSinceIDScheduled:   postgresSelectMessagesSinceIDIncludeScheduledQuery,
	selectMessagesLatest:             postgresSelectMessagesLatestQuery,
	selectMessagesDue:                postgresSelectMessagesDueQuery,
	claimMessagesDue:                 postgresClaimMessagesDueQuery,
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
	if err := schema.Migrate(d.Primary(), schema.Postgres, schemaStore, postgresCurrentSchemaVersion, postgresCreateTables, postgresMigrations); err != nil {
		return nil, err
	}
	return newCache(d, postgresQueries, nil, batchSize, batchTimeout, false), nil
}

// postgresInsertMessages transposes the batch into one array per column and writes it with
// postgresInsertMessagesQuery in a single round trip
func postgresInsertMessages(tx *sql.Tx, ms []*model.Message) error {
	columns := make([][]any, insertMessageColumns)
	for _, m := range ms {
		args, err := insertMessageArgs(m)
		if err != nil {
			return err
		}
		for i, arg := range args {
			columns[i] = append(columns[i], arg)
		}
	}
	params := make([]any, 0, len(columns))
	for _, column := range columns {
		params = append(params, column)
	}
	_, err := tx.Exec(postgresInsertMessagesQuery, params...)
	return err
}
