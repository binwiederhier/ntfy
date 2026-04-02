package message

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	tagMessageCache = "message_cache"
)

var errNoRows = errors.New("no rows found")

// queries holds the database-specific SQL queries
type queries struct {
	insertMessage                    string
	selectScheduledMessageIDsBySeqID string
	deleteScheduledBySequenceID      string
	updateMessagesForTopicExpiry     string
	selectMessagesByID               string
	selectMessagesSinceTime          string
	selectMessagesSinceTimeScheduled string
	selectMessagesSinceID            string
	selectMessagesSinceIDScheduled   string
	selectMessagesLatest             string
	selectMessagesDue                string
	deleteExpiredMessages            string
	updateMessagePublished           string
	selectMessagesCount              string
	selectTopics                     string
	markExpiredAttachmentsDeleted    string
	selectAttachmentsSizeBySender    string
	selectAttachmentsSizeByUserID    string
	selectAttachmentsWithSizes       string
	selectStats                      string
	updateStats                      string
	updateMessageTime                string
}

// Cache stores published messages
type Cache struct {
	db      *db.DB
	queue   *util.BatchingQueue[*model.Message]
	nop     bool
	mu      *sync.Mutex // nil for PostgreSQL (concurrent writes supported), set for SQLite (single writer)
	queries queries
}

func newCache(db *db.DB, queries queries, mu *sync.Mutex, batchSize int, batchTimeout time.Duration, nop bool) *Cache {
	var queue *util.BatchingQueue[*model.Message]
	if batchSize > 0 || batchTimeout > 0 {
		queue = util.NewBatchingQueue[*model.Message](batchSize, batchTimeout)
	}
	c := &Cache{
		db:      db,
		queue:   queue,
		nop:     nop,
		mu:      mu,
		queries: queries,
	}
	go c.processMessageBatches()
	return c
}

func (c *Cache) maybeLock() {
	if c.mu != nil {
		c.mu.Lock()
	}
}

func (c *Cache) maybeUnlock() {
	if c.mu != nil {
		c.mu.Unlock()
	}
}

// AddMessage stores a message to the message cache synchronously, or queues it to be stored at a later date asynchronously.
// The message is queued only if "batchSize" or "batchTimeout" are passed to the constructor.
func (c *Cache) AddMessage(m *model.Message) error {
	if c.queue != nil {
		c.queue.Enqueue(m)
		return nil
	}
	return c.addMessages([]*model.Message{m})
}

// AddMessages synchronously stores a batch of messages to the message cache
func (c *Cache) AddMessages(ms []*model.Message) error {
	return c.addMessages(ms)
}

func (c *Cache) addMessages(ms []*model.Message) error {
	c.maybeLock()
	defer c.maybeUnlock()
	if c.nop {
		return nil
	}
	if len(ms) == 0 {
		return nil
	}
	start := time.Now()
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(c.queries.insertMessage)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range ms {
		if m.Event != model.MessageEvent && m.Event != model.MessageDeleteEvent && m.Event != model.MessageClearEvent {
			return model.ErrUnexpectedMessageType
		}
		published := m.Time <= time.Now().Unix()
		tags := util.SanitizeUTF8(strings.Join(m.Tags, ","))
		var attachmentName, attachmentType, attachmentURL string
		var attachmentSize, attachmentExpires int64
		var attachmentDeleted bool
		if m.Attachment != nil {
			attachmentName = util.SanitizeUTF8(m.Attachment.Name)
			attachmentType = util.SanitizeUTF8(m.Attachment.Type)
			attachmentSize = m.Attachment.Size
			attachmentExpires = m.Attachment.Expires
			attachmentURL = util.SanitizeUTF8(m.Attachment.URL)
		}
		var actionsStr string
		if len(m.Actions) > 0 {
			actionsBytes, err := json.Marshal(m.Actions)
			if err != nil {
				return err
			}
			actionsStr = string(actionsBytes)
		}
		var sender string
		if m.Sender.IsValid() {
			sender = m.Sender.String()
		}
		_, err := stmt.Exec(
			m.ID,
			m.SequenceID,
			m.Time,
			m.Event,
			m.Expires,
			util.SanitizeUTF8(m.Topic),
			util.SanitizeUTF8(m.Message),
			util.SanitizeUTF8(m.Title),
			m.Priority,
			tags,
			util.SanitizeUTF8(m.Click),
			util.SanitizeUTF8(m.Icon),
			actionsStr,
			attachmentName,
			attachmentType,
			attachmentSize,
			attachmentExpires,
			attachmentURL,
			attachmentDeleted, // Always zero
			sender,
			m.User,
			util.SanitizeUTF8(m.ContentType),
			m.Encoding,
			published,
		)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		log.Tag(tagMessageCache).Err(err).Error("Writing %d message(s) failed (took %v)", len(ms), time.Since(start))
		return err
	}
	log.Tag(tagMessageCache).Debug("Wrote %d message(s) in %v", len(ms), time.Since(start))
	return nil
}

// Messages returns messages for a topic since the given marker, optionally including scheduled messages
func (c *Cache) Messages(topic string, since model.SinceMarker, scheduled bool) ([]*model.Message, error) {
	if since.IsNone() {
		return make([]*model.Message, 0), nil
	} else if since.IsLatest() {
		return c.messagesLatest(topic)
	} else if since.IsID() {
		return c.messagesSinceID(topic, since, scheduled)
	}
	return c.messagesSinceTime(topic, since, scheduled)
}

func (c *Cache) messagesSinceTime(topic string, since model.SinceMarker, scheduled bool) ([]*model.Message, error) {
	var rows *sql.Rows
	var err error
	rdb := c.db.ReadOnly()
	if scheduled {
		rows, err = rdb.Query(c.queries.selectMessagesSinceTimeScheduled, topic, since.Time().Unix())
	} else {
		rows, err = rdb.Query(c.queries.selectMessagesSinceTime, topic, since.Time().Unix())
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *Cache) messagesSinceID(topic string, since model.SinceMarker, scheduled bool) ([]*model.Message, error) {
	var rows *sql.Rows
	var err error
	rdb := c.db.ReadOnly()
	if scheduled {
		rows, err = rdb.Query(c.queries.selectMessagesSinceIDScheduled, topic, since.ID())
	} else {
		rows, err = rdb.Query(c.queries.selectMessagesSinceID, topic, since.ID())
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *Cache) messagesLatest(topic string) ([]*model.Message, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectMessagesLatest, topic)
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

// MessagesDue returns all messages that are due for publishing
func (c *Cache) MessagesDue() ([]*model.Message, error) {
	rows, err := c.db.Query(c.queries.selectMessagesDue, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

// DeleteExpiredMessages deletes up to `limit` expired messages in a single query
// and returns the number of deleted rows.
func (c *Cache) DeleteExpiredMessages(limit int) (int64, error) {
	c.maybeLock()
	defer c.maybeUnlock()
	result, err := c.db.Exec(c.queries.deleteExpiredMessages, time.Now().Unix(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Message returns the message with the given ID, or ErrMessageNotFound if not found
func (c *Cache) Message(id string) (*model.Message, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectMessagesByID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, model.ErrMessageNotFound
	}
	return readMessage(rows)
}

// UpdateMessageTime updates the time column for a message by ID. This is only used for testing.
func (c *Cache) UpdateMessageTime(messageID string, timestamp int64) error {
	c.maybeLock()
	defer c.maybeUnlock()
	_, err := c.db.Exec(c.queries.updateMessageTime, timestamp, messageID)
	return err
}

// MarkPublished marks a message as published
func (c *Cache) MarkPublished(m *model.Message) error {
	c.maybeLock()
	defer c.maybeUnlock()
	_, err := c.db.Exec(c.queries.updateMessagePublished, m.ID)
	return err
}

// MessagesCount returns the total number of messages in the cache
func (c *Cache) MessagesCount() (int, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectMessagesCount)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Topics returns a list of all topics with messages in the cache
func (c *Cache) Topics() ([]string, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectTopics)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readStrings(rows)
}

// DeleteScheduledBySequenceID deletes unpublished (scheduled) messages with the given topic and sequence ID.
// It returns the message IDs of the deleted messages, which can be used to clean up attachment files.
func (c *Cache) DeleteScheduledBySequenceID(topic, sequenceID string) ([]string, error) {
	c.maybeLock()
	defer c.maybeUnlock()
	return db.QueryTx(c.db, func(tx *sql.Tx) ([]string, error) {
		rows, err := tx.Query(c.queries.selectScheduledMessageIDsBySeqID, topic, sequenceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		ids, err := readStrings(rows)
		if err != nil {
			return nil, err
		}
		rows.Close() // Close rows before executing delete in same transaction
		if _, err := tx.Exec(c.queries.deleteScheduledBySequenceID, topic, sequenceID); err != nil {
			return nil, err
		}
		return ids, nil
	})
}

// ExpireMessages marks messages in the given topics as expired
func (c *Cache) ExpireMessages(topics ...string) error {
	c.maybeLock()
	defer c.maybeUnlock()
	return db.ExecTx(c.db, func(tx *sql.Tx) error {
		for _, t := range topics {
			if _, err := tx.Exec(c.queries.updateMessagesForTopicExpiry, time.Now().Unix()-1, t); err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkExpiredAttachmentsDeleted marks up to `limit` expired attachments as deleted in a single
// query and returns the number of updated rows.
func (c *Cache) MarkExpiredAttachmentsDeleted(limit int) (int64, error) {
	c.maybeLock()
	defer c.maybeUnlock()
	result, err := c.db.Exec(c.queries.markExpiredAttachmentsDeleted, time.Now().Unix(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// AttachmentBytesUsedBySender returns the total size of active attachments sent by the given sender
func (c *Cache) AttachmentBytesUsedBySender(sender string) (int64, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectAttachmentsSizeBySender, sender, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

// AttachmentBytesUsedByUser returns the total size of active attachments for the given user
func (c *Cache) AttachmentBytesUsedByUser(userID string) (int64, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectAttachmentsSizeByUserID, userID, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

// AttachmentsWithSizes returns a map of message ID to attachment size for all active
// (non-expired, non-deleted) attachments. This is used to hydrate the attachment store's
// size tracking on startup and during periodic sync.
func (c *Cache) AttachmentsWithSizes() (map[string]int64, error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectAttachmentsWithSizes, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attachments := make(map[string]int64)
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			return nil, err
		}
		attachments[id] = size
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attachments, nil
}

func (c *Cache) readAttachmentBytesUsed(rows *sql.Rows) (int64, error) {
	defer rows.Close()
	var size int64
	if !rows.Next() {
		return 0, errors.New("no rows found")
	}
	if err := rows.Scan(&size); err != nil {
		return 0, err
	} else if err := rows.Err(); err != nil {
		return 0, err
	}
	return size, nil
}

// UpdateStats updates the total message count statistic
func (c *Cache) UpdateStats(messages int64) error {
	c.maybeLock()
	defer c.maybeUnlock()
	_, err := c.db.Exec(c.queries.updateStats, messages)
	return err
}

// Stats returns the total message count statistic
func (c *Cache) Stats() (messages int64, err error) {
	rows, err := c.db.ReadOnly().Query(c.queries.selectStats)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	if err := rows.Scan(&messages); err != nil {
		return 0, err
	}
	return messages, nil
}

// Close closes the underlying database connection
func (c *Cache) Close() error {
	return c.db.Close()
}

func (c *Cache) processMessageBatches() {
	if c.queue == nil {
		return
	}
	for messages := range c.queue.Dequeue() {
		if err := c.addMessages(messages); err != nil {
			log.Tag(tagMessageCache).Err(err).Error("Cannot write message batch")
		}
	}
}

func readMessages(rows *sql.Rows) ([]*model.Message, error) {
	defer rows.Close()
	messages := make([]*model.Message, 0)
	for rows.Next() {
		m, err := readMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func readMessage(rows *sql.Rows) (*model.Message, error) {
	var timestamp, expires, attachmentSize, attachmentExpires int64
	var priority int
	var id, sequenceID, event, topic, msg, title, tagsStr, click, icon, actionsStr, attachmentName, attachmentType, attachmentURL, sender, user, contentType, encoding string
	err := rows.Scan(
		&id,
		&sequenceID,
		&timestamp,
		&event,
		&expires,
		&topic,
		&msg,
		&title,
		&priority,
		&tagsStr,
		&click,
		&icon,
		&actionsStr,
		&attachmentName,
		&attachmentType,
		&attachmentSize,
		&attachmentExpires,
		&attachmentURL,
		&sender,
		&user,
		&contentType,
		&encoding,
	)
	if err != nil {
		return nil, err
	}
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}
	var actions []*model.Action
	if actionsStr != "" {
		if err := json.Unmarshal([]byte(actionsStr), &actions); err != nil {
			return nil, err
		}
	}
	senderIP, err := netip.ParseAddr(sender)
	if err != nil {
		senderIP = netip.Addr{} // if no IP stored in database, return invalid address
	}
	var att *model.Attachment
	if attachmentName != "" && attachmentURL != "" {
		att = &model.Attachment{
			Name:    attachmentName,
			Type:    attachmentType,
			Size:    attachmentSize,
			Expires: attachmentExpires,
			URL:     attachmentURL,
		}
	}
	return &model.Message{
		ID:          id,
		SequenceID:  sequenceID,
		Time:        timestamp,
		Expires:     expires,
		Event:       event,
		Topic:       topic,
		Message:     msg,
		Title:       title,
		Priority:    priority,
		Tags:        tags,
		Click:       click,
		Icon:        icon,
		Actions:     actions,
		Attachment:  att,
		Sender:      senderIP,
		User:        user,
		ContentType: contentType,
		Encoding:    encoding,
	}, nil
}

func readStrings(rows *sql.Rows) ([]string, error) {
	strs := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		strs = append(strs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return strs, nil
}
