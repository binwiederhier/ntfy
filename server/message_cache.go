package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
	"net/netip"
	"strings"
	"time"
)

type MessageCache interface {
	AddMessage(m *message) error
	AddMessages(ms []*message) error
	Messages(topic string, since sinceMarker, scheduled bool) ([]*message, error)
	MessagesDue() ([]*message, error)
	MessagesExpired() ([]string, error)
	Message(id string) (*message, error)
	MarkPublished(m *message) error
	MessageCounts() (map[string]int, error)
	Topics() (map[string]*topic, error)
	DeleteMessages(ids ...string) error
	ExpireMessages(topics ...string) error
	AttachmentsExpired() ([]string, error)
	MarkAttachmentsDeleted(ids ...string) error
	AttachmentBytesUsedBySender(sender string) (int64, error)
	AttachmentBytesUsedByUser(userID string) (int64, error)
	UpdateStats(messages int64) error
	Stats() (messages int64, err error)
	DB() *sql.DB
	Close() error
}

type commonMessageCache struct {
	db      *sql.DB
	queue   *util.BatchingQueue[*message]
	queries *messageCacheQueries
}

var _ MessageCache = (*commonMessageCache)(nil)

type messageCacheQueries struct {
	insertMessage                           string
	deleteMessage                           string
	updateMessagesForTopicExpiry            string
	selectRowIDFromMessageID                string // Do not include topic, see #336 and TestServer_PollSinceID_MultipleTopics
	selectMessagesByID                      string
	selectMessagesSinceTime                 string
	selectMessagesSinceTimeIncludeScheduled string
	selectMessagesSinceID                   string
	selectMessagesSinceIDIncludeScheduled   string
	selectMessagesDue                       string
	selectMessagesExpired                   string
	updateMessagePublished                  string
	selectMessageCountPerTopic              string
	selectTopics                            string

	updateAttachmentDeleted       string
	selectAttachmentsExpired      string
	selectAttachmentsSizeBySender string
	selectAttachmentsSizeByUserID string

	selectStats string
	updateStats string
}

// AddMessage stores a message to the message cache synchronously, or queues it to be stored at a later date asyncronously.
// The message is queued only if "batchSize" or "batchTimeout" are passed to the constructor.
func (c *commonMessageCache) AddMessage(m *message) error {
	if c.queue != nil {
		c.queue.Enqueue(m)
		return nil
	}
	return c.AddMessages([]*message{m})
}

// AddMessages synchronously stores a match of messages. If the database is locked, the transaction waits until
// SQLite's busy_timeout is exceeded before erroring out.
func (c *commonMessageCache) AddMessages(ms []*message) error {
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
		if m.Event != messageEvent {
			return errUnexpectedMessageType
		}
		published := m.Time <= time.Now().Unix()
		tags := strings.Join(m.Tags, ",")
		var attachmentName, attachmentType, attachmentURL string
		var attachmentSize, attachmentExpires int64
		var attachmentDeleted bool
		if m.Attachment != nil {
			attachmentName = m.Attachment.Name
			attachmentType = m.Attachment.Type
			attachmentSize = m.Attachment.Size
			attachmentExpires = m.Attachment.Expires
			attachmentURL = m.Attachment.URL
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
			m.Time,
			m.Expires,
			m.Topic,
			m.Message,
			m.Title,
			m.Priority,
			tags,
			m.Click,
			m.Icon,
			actionsStr,
			attachmentName,
			attachmentType,
			attachmentSize,
			attachmentExpires,
			attachmentURL,
			attachmentDeleted, // Always false
			sender,
			m.User,
			m.ContentType,
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

func (c *commonMessageCache) Messages(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	if since.IsNone() {
		return make([]*message, 0), nil
	} else if since.IsID() {
		return c.messagesSinceID(topic, since, scheduled)
	}
	return c.messagesSinceTime(topic, since, scheduled)
}

func (c *commonMessageCache) messagesSinceTime(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	var rows *sql.Rows
	var err error
	if scheduled {
		rows, err = c.db.Query(c.queries.selectMessagesSinceTimeIncludeScheduled, topic, since.Time().Unix())
	} else {
		rows, err = c.db.Query(c.queries.selectMessagesSinceTime, topic, since.Time().Unix())
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *commonMessageCache) messagesSinceID(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	idrows, err := c.db.Query(c.queries.selectRowIDFromMessageID, since.ID())
	if err != nil {
		return nil, err
	}
	defer idrows.Close()
	if !idrows.Next() {
		return c.messagesSinceTime(topic, sinceAllMessages, scheduled)
	}
	var rowID int64
	if err := idrows.Scan(&rowID); err != nil {
		return nil, err
	}
	idrows.Close()
	var rows *sql.Rows
	if scheduled {
		rows, err = c.db.Query(c.queries.selectMessagesSinceIDIncludeScheduled, topic, rowID)
	} else {
		rows, err = c.db.Query(c.queries.selectMessagesSinceID, topic, rowID)
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *commonMessageCache) MessagesDue() ([]*message, error) {
	rows, err := c.db.Query(c.queries.selectMessagesDue, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

// MessagesExpired returns a list of IDs for messages that have expires (should be deleted)
func (c *commonMessageCache) MessagesExpired() ([]string, error) {
	rows, err := c.db.Query(c.queries.selectMessagesExpired, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (c *commonMessageCache) Message(id string) (*message, error) {
	rows, err := c.db.Query(c.queries.selectMessagesByID, id)
	if err != nil {
		return nil, err
	} else if !rows.Next() {
		return nil, errMessageNotFound
	}
	defer rows.Close()
	return readMessage(rows)
}

func (c *commonMessageCache) MarkPublished(m *message) error {
	_, err := c.db.Exec(c.queries.updateMessagePublished, m.ID)
	return err
}

func (c *commonMessageCache) MessageCounts() (map[string]int, error) {
	rows, err := c.db.Query(c.queries.selectMessageCountPerTopic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var topic string
	var count int
	counts := make(map[string]int)
	for rows.Next() {
		if err := rows.Scan(&topic, &count); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		counts[topic] = count
	}
	return counts, nil
}

func (c *commonMessageCache) Topics() (map[string]*topic, error) {
	rows, err := c.db.Query(c.queries.selectTopics)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	topics := make(map[string]*topic)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		topics[id] = newTopic(id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return topics, nil
}

func (c *commonMessageCache) DeleteMessages(ids ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(c.queries.deleteMessage, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *commonMessageCache) ExpireMessages(topics ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range topics {
		if _, err := tx.Exec(c.queries.updateMessagesForTopicExpiry, time.Now().Unix()-1, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *commonMessageCache) AttachmentsExpired() ([]string, error) {
	rows, err := c.db.Query(c.queries.selectAttachmentsExpired, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (c *commonMessageCache) MarkAttachmentsDeleted(ids ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(c.queries.updateAttachmentDeleted, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *commonMessageCache) AttachmentBytesUsedBySender(sender string) (int64, error) {
	rows, err := c.db.Query(c.queries.selectAttachmentsSizeBySender, sender, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

func (c *commonMessageCache) AttachmentBytesUsedByUser(userID string) (int64, error) {
	rows, err := c.db.Query(c.queries.selectAttachmentsSizeByUserID, userID, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

func (c *commonMessageCache) readAttachmentBytesUsed(rows *sql.Rows) (int64, error) {
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

func (c *commonMessageCache) processMessageBatches() {
	if c.queue == nil {
		return
	}
	for messages := range c.queue.Dequeue() {
		if err := c.AddMessages(messages); err != nil {
			log.Tag(tagMessageCache).Err(err).Error("Cannot write message batch")
		}
	}
}

func (c *commonMessageCache) UpdateStats(messages int64) error {
	_, err := c.db.Exec(c.queries.updateStats, messages)
	return err
}

func (c *commonMessageCache) Stats() (messages int64, err error) {
	rows, err := c.db.Query(c.queries.selectStats)
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

func (c *commonMessageCache) DB() *sql.DB {
	return c.db
}

func (c *commonMessageCache) Close() error {
	return c.db.Close()
}

func readMessages(rows *sql.Rows) ([]*message, error) {
	defer rows.Close()
	messages := make([]*message, 0)
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

func readMessage(rows *sql.Rows) (*message, error) {
	var timestamp, expires, attachmentSize, attachmentExpires int64
	var priority int
	var id, topic, msg, title, tagsStr, click, icon, actionsStr, attachmentName, attachmentType, attachmentURL, sender, user, contentType, encoding string
	err := rows.Scan(
		&id,
		&timestamp,
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
	var actions []*action
	if actionsStr != "" {
		if err := json.Unmarshal([]byte(actionsStr), &actions); err != nil {
			return nil, err
		}
	}
	senderIP, err := netip.ParseAddr(sender)
	if err != nil {
		senderIP = netip.Addr{} // if no IP stored in database, return invalid address
	}
	var att *attachment
	if attachmentName != "" && attachmentURL != "" {
		att = &attachment{
			Name:    attachmentName,
			Type:    attachmentType,
			Size:    attachmentSize,
			Expires: attachmentExpires,
			URL:     attachmentURL,
		}
	}
	return &message{
		ID:          id,
		Time:        timestamp,
		Expires:     expires,
		Event:       messageEvent,
		Topic:       topic,
		Message:     msg,
		Title:       title,
		Priority:    priority,
		Tags:        tags,
		Click:       click,
		Icon:        icon,
		Actions:     actions,
		Attachment:  att,
		Sender:      senderIP, // Must parse assuming database must be correct
		User:        user,
		ContentType: contentType,
		Encoding:    encoding,
	}, nil
}
