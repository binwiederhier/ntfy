package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
)

var (
	errUnexpectedMessageType = errors.New("unexpected message type")
	errMessageNotFound       = errors.New("message not found")
)

// Messages cache
const (
	createMessagesTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
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
			attachment_deleted INT NOT NULL,
			sender TEXT NOT NULL,
			user TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages (mid);
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
		COMMIT;
	`
	insertMessageQuery = `
		INSERT INTO messages (mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user, encoding, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	deleteMessageQuery                = `DELETE FROM messages WHERE mid = ?`
	updateMessagesForTopicExpiryQuery = `UPDATE messages SET expires = ? WHERE topic = ?`
	selectRowIDFromMessageID          = `SELECT id FROM messages WHERE mid = ?` // Do not include topic, see #336 and TestServer_PollSinceID_MultipleTopics
	selectMessagesByIDQuery           = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE mid = ?
	`
	selectMessagesSinceTimeQuery = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE topic = ? AND time >= ? AND published = 1
		ORDER BY time, id
	`
	selectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE topic = ? AND time >= ?
		ORDER BY time, id
	`
	selectMessagesSinceIDQuery = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE topic = ? AND id > ? AND published = 1 
		ORDER BY time, id
	`
	selectMessagesSinceIDIncludeScheduledQuery = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE topic = ? AND (id > ? OR published = 0)
		ORDER BY time, id
	`
	selectMessagesDueQuery = `
		SELECT mid, time, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user, encoding
		FROM messages 
		WHERE time <= ? AND published = 0
		ORDER BY time, id
	`
	selectMessagesExpiredQuery      = `SELECT mid FROM messages WHERE expires <= ? AND published = 1`
	updateMessagePublishedQuery     = `UPDATE messages SET published = 1 WHERE mid = ?`
	selectMessagesCountQuery        = `SELECT COUNT(*) FROM messages`
	selectMessageCountPerTopicQuery = `SELECT topic, COUNT(*) FROM messages GROUP BY topic`
	selectTopicsQuery               = `SELECT topic FROM messages GROUP BY topic`

	updateAttachmentDeleted            = `UPDATE messages SET attachment_deleted = 1 WHERE mid = ?`
	selectAttachmentsExpiredQuery      = `SELECT mid FROM messages WHERE attachment_expires > 0 AND attachment_expires <= ? AND attachment_deleted = 0`
	selectAttachmentsSizeBySenderQuery = `SELECT IFNULL(SUM(attachment_size), 0) FROM messages WHERE user = '' AND sender = ? AND attachment_expires >= ?`
	selectAttachmentsSizeByUserIDQuery = `SELECT IFNULL(SUM(attachment_size), 0) FROM messages WHERE user = ? AND attachment_expires >= ?`
)

// Schema management queries
const (
	currentSchemaVersion          = 10
	createSchemaVersionTableQuery = `
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
	`
	insertSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	updateSchemaVersion      = `UPDATE schemaVersion SET version = ? WHERE id = 1`
	selectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`

	// 0 -> 1
	migrate0To1AlterMessagesTableQuery = `
		BEGIN;
		ALTER TABLE messages ADD COLUMN title TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN priority INT NOT NULL DEFAULT(0);
		ALTER TABLE messages ADD COLUMN tags TEXT NOT NULL DEFAULT('');
		COMMIT;
	`

	// 1 -> 2
	migrate1To2AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN published INT NOT NULL DEFAULT(1);
	`

	// 2 -> 3
	migrate2To3AlterMessagesTableQuery = `
		BEGIN;
		ALTER TABLE messages ADD COLUMN click TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_name TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_type TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_size INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_expires INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_owner TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_url TEXT NOT NULL DEFAULT('');
		COMMIT;
	`
	// 3 -> 4
	migrate3To4AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN encoding TEXT NOT NULL DEFAULT('');
	`

	// 4 -> 5
	migrate4To5AlterMessagesTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			time INT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size INT NOT NULL,
			attachment_expires INT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_owner TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages_new (mid);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages_new (topic);
		INSERT 
			INTO messages_new (
				mid, time, topic, message, title, priority, tags, click, attachment_name, attachment_type, 
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published)
			SELECT
				id, time, topic, message, title, priority, tags, click, attachment_name, attachment_type, 
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published
			FROM messages;
		DROP TABLE messages;
		ALTER TABLE messages_new RENAME TO messages;
		COMMIT;
	`

	// 5 -> 6
	migrate5To6AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN actions TEXT NOT NULL DEFAULT('');
	`

	// 6 -> 7
	migrate6To7AlterMessagesTableQuery = `
		ALTER TABLE messages RENAME COLUMN attachment_owner TO sender;
	`

	// 7 -> 8
	migrate7To8AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN icon TEXT NOT NULL DEFAULT('');
	`

	// 8 -> 9
	migrate8To9AlterMessagesTableQuery = `
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);	
	`

	// 9 -> 10
	migrate9To10AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN user TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_deleted INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN expires INT NOT NULL DEFAULT('0');
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
	`
	migrate9To10UpdateMessageExpiryQuery = `UPDATE messages SET expires = time + ?`
)

var (
	migrations = map[int]func(db *sql.DB, cacheDuration time.Duration) error{
		0: migrateFrom0,
		1: migrateFrom1,
		2: migrateFrom2,
		3: migrateFrom3,
		4: migrateFrom4,
		5: migrateFrom5,
		6: migrateFrom6,
		7: migrateFrom7,
		8: migrateFrom8,
		9: migrateFrom9,
	}
)

type messageCache struct {
	db    *sql.DB
	queue *util.BatchingQueue[*message]
	nop   bool
}

// newSqliteCache creates a SQLite file-backed cache
func newSqliteCache(filename, startupQueries string, cacheDuration time.Duration, batchSize int, batchTimeout time.Duration, nop bool) (*messageCache, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupDB(db, startupQueries, cacheDuration); err != nil {
		return nil, err
	}
	var queue *util.BatchingQueue[*message]
	if batchSize > 0 || batchTimeout > 0 {
		queue = util.NewBatchingQueue[*message](batchSize, batchTimeout)
	}
	cache := &messageCache{
		db:    db,
		queue: queue,
		nop:   nop,
	}
	go cache.processMessageBatches()
	return cache, nil
}

// newMemCache creates an in-memory cache
func newMemCache() (*messageCache, error) {
	return newSqliteCache(createMemoryFilename(), "", 0, 0, 0, false)
}

// newNopCache creates an in-memory cache that discards all messages;
// it is always empty and can be used if caching is entirely disabled
func newNopCache() (*messageCache, error) {
	return newSqliteCache(createMemoryFilename(), "", 0, 0, 0, true)
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

// AddMessage stores a message to the message cache synchronously, or queues it to be stored at a later date asyncronously.
// The message is queued only if "batchSize" or "batchTimeout" are passed to the constructor.
func (c *messageCache) AddMessage(m *message) error {
	if c.queue != nil {
		c.queue.Enqueue(m)
		return nil
	}
	return c.addMessages([]*message{m})
}

// addMessages synchronously stores a match of messages. If the database is locked, the transaction waits until
// SQLite's busy_timeout is exceeded before erroring out.
func (c *messageCache) addMessages(ms []*message) error {
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
	stmt, err := tx.Prepare(insertMessageQuery)
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
		var attachmentSize, attachmentExpires, attachmentDeleted int64
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
			attachmentDeleted, // Always zero
			sender,
			m.User,
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

func (c *messageCache) Messages(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	if since.IsNone() {
		return make([]*message, 0), nil
	} else if since.IsID() {
		return c.messagesSinceID(topic, since, scheduled)
	}
	return c.messagesSinceTime(topic, since, scheduled)
}

func (c *messageCache) messagesSinceTime(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	var rows *sql.Rows
	var err error
	if scheduled {
		rows, err = c.db.Query(selectMessagesSinceTimeIncludeScheduledQuery, topic, since.Time().Unix())
	} else {
		rows, err = c.db.Query(selectMessagesSinceTimeQuery, topic, since.Time().Unix())
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *messageCache) messagesSinceID(topic string, since sinceMarker, scheduled bool) ([]*message, error) {
	idrows, err := c.db.Query(selectRowIDFromMessageID, since.ID())
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
		rows, err = c.db.Query(selectMessagesSinceIDIncludeScheduledQuery, topic, rowID)
	} else {
		rows, err = c.db.Query(selectMessagesSinceIDQuery, topic, rowID)
	}
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *messageCache) MessagesDue() ([]*message, error) {
	rows, err := c.db.Query(selectMessagesDueQuery, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

// MessagesExpired returns a list of IDs for messages that have expires (should be deleted)
func (c *messageCache) MessagesExpired() ([]string, error) {
	rows, err := c.db.Query(selectMessagesExpiredQuery, time.Now().Unix())
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

func (c *messageCache) Message(id string) (*message, error) {
	rows, err := c.db.Query(selectMessagesByIDQuery, id)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		return nil, errMessageNotFound
	}
	defer rows.Close()
	return readMessage(rows)
}

func (c *messageCache) MarkPublished(m *message) error {
	_, err := c.db.Exec(updateMessagePublishedQuery, m.ID)
	return err
}

func (c *messageCache) MessageCounts() (map[string]int, error) {
	rows, err := c.db.Query(selectMessageCountPerTopicQuery)
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

func (c *messageCache) Topics() (map[string]*topic, error) {
	rows, err := c.db.Query(selectTopicsQuery)
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

func (c *messageCache) DeleteMessages(ids ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(deleteMessageQuery, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *messageCache) ExpireMessages(topics ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range topics {
		if _, err := tx.Exec(updateMessagesForTopicExpiryQuery, time.Now().Unix(), t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *messageCache) AttachmentsExpired() ([]string, error) {
	rows, err := c.db.Query(selectAttachmentsExpiredQuery, time.Now().Unix())
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

func (c *messageCache) MarkAttachmentsDeleted(ids ...string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(updateAttachmentDeleted, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *messageCache) AttachmentBytesUsedBySender(sender string) (int64, error) {
	rows, err := c.db.Query(selectAttachmentsSizeBySenderQuery, sender, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

func (c *messageCache) AttachmentBytesUsedByUser(userID string) (int64, error) {
	rows, err := c.db.Query(selectAttachmentsSizeByUserIDQuery, userID, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return c.readAttachmentBytesUsed(rows)
}

func (c *messageCache) readAttachmentBytesUsed(rows *sql.Rows) (int64, error) {
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

func (c *messageCache) processMessageBatches() {
	if c.queue == nil {
		return
	}
	for messages := range c.queue.Dequeue() {
		if err := c.addMessages(messages); err != nil {
			log.Tag(tagMessageCache).Err(err).Error("Cannot write message batch")
		}
	}
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
	var id, topic, msg, title, tagsStr, click, icon, actionsStr, attachmentName, attachmentType, attachmentURL, sender, user, encoding string
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
		ID:         id,
		Time:       timestamp,
		Expires:    expires,
		Event:      messageEvent,
		Topic:      topic,
		Message:    msg,
		Title:      title,
		Priority:   priority,
		Tags:       tags,
		Click:      click,
		Icon:       icon,
		Actions:    actions,
		Attachment: att,
		Sender:     senderIP, // Must parse assuming database must be correct
		User:       user,
		Encoding:   encoding,
	}, nil
}

func (c *messageCache) Close() error {
	return c.db.Close()
}

func setupDB(db *sql.DB, startupQueries string, cacheDuration time.Duration) error {
	// Run startup queries
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}

	// If 'messages' table does not exist, this must be a new database
	rowsMC, err := db.Query(selectMessagesCountQuery)
	if err != nil {
		return setupNewCacheDB(db)
	}
	rowsMC.Close()

	// If 'messages' table exists, check 'schemaVersion' table
	schemaVersion := 0
	rowsSV, err := db.Query(selectSchemaVersionQuery)
	if err == nil {
		defer rowsSV.Close()
		if !rowsSV.Next() {
			return errors.New("cannot determine schema version: cache file may be corrupt")
		}
		if err := rowsSV.Scan(&schemaVersion); err != nil {
			return err
		}
		rowsSV.Close()
	}

	// Do migrations
	if schemaVersion == currentSchemaVersion {
		return nil
	} else if schemaVersion > currentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, currentSchemaVersion)
	}
	for i := schemaVersion; i < currentSchemaVersion; i++ {
		fn, ok := migrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(db, cacheDuration); err != nil {
			return err
		}
	}
	return nil
}

func setupNewCacheDB(db *sql.DB) error {
	if _, err := db.Exec(createMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(createSchemaVersionTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(insertSchemaVersion, currentSchemaVersion); err != nil {
		return err
	}
	return nil
}

func migrateFrom0(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 0 to 1")
	if _, err := db.Exec(migrate0To1AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(createSchemaVersionTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(insertSchemaVersion, 1); err != nil {
		return err
	}
	return nil
}

func migrateFrom1(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 1 to 2")
	if _, err := db.Exec(migrate1To2AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 2); err != nil {
		return err
	}
	return nil
}

func migrateFrom2(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 2 to 3")
	if _, err := db.Exec(migrate2To3AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 3); err != nil {
		return err
	}
	return nil
}

func migrateFrom3(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 3 to 4")
	if _, err := db.Exec(migrate3To4AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 4); err != nil {
		return err
	}
	return nil
}

func migrateFrom4(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 4 to 5")
	if _, err := db.Exec(migrate4To5AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 5); err != nil {
		return err
	}
	return nil
}

func migrateFrom5(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 5 to 6")
	if _, err := db.Exec(migrate5To6AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 6); err != nil {
		return err
	}
	return nil
}

func migrateFrom6(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 6 to 7")
	if _, err := db.Exec(migrate6To7AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 7); err != nil {
		return err
	}
	return nil
}

func migrateFrom7(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 7 to 8")
	if _, err := db.Exec(migrate7To8AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 8); err != nil {
		return err
	}
	return nil
}

func migrateFrom8(db *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 8 to 9")
	if _, err := db.Exec(migrate8To9AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 9); err != nil {
		return err
	}
	return nil
}

func migrateFrom9(db *sql.DB, cacheDuration time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 9 to 10")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migrate9To10AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := tx.Exec(migrate9To10UpdateMessageExpiryQuery, int64(cacheDuration.Seconds())); err != nil {
		return err
	}
	if _, err := tx.Exec(updateSchemaVersion, 10); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil // Update this when a new version is added
}
