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
)

// Messages cache
const (
	createMessagesTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			time INT NOT NULL,
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
			sender TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages (mid);
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		COMMIT;
	`
	insertMessageQuery = `
		INSERT INTO messages (mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding, published)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	pruneMessagesQuery           = `DELETE FROM messages WHERE time < ? AND published = 1`
	selectRowIDFromMessageID     = `SELECT id FROM messages WHERE mid = ?` // Do not include topic, see #336 and TestServer_PollSinceID_MultipleTopics
	selectMessagesSinceTimeQuery = `
		SELECT mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding
		FROM messages 
		WHERE topic = ? AND time >= ? AND published = 1
		ORDER BY time, id
	`
	selectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding
		FROM messages 
		WHERE topic = ? AND time >= ?
		ORDER BY time, id
	`
	selectMessagesSinceIDQuery = `
		SELECT mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding
		FROM messages 
		WHERE topic = ? AND id > ? AND published = 1 
		ORDER BY time, id
	`
	selectMessagesSinceIDIncludeScheduledQuery = `
		SELECT mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding
		FROM messages 
		WHERE topic = ? AND (id > ? OR published = 0)
		ORDER BY time, id
	`
	selectMessagesDueQuery = `
		SELECT mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding
		FROM messages 
		WHERE time <= ? AND published = 0
		ORDER BY time, id
	`
	updateMessagePublishedQuery     = `UPDATE messages SET published = 1 WHERE mid = ?`
	selectMessagesCountQuery        = `SELECT COUNT(*) FROM messages`
	selectMessageCountPerTopicQuery = `SELECT topic, COUNT(*) FROM messages GROUP BY topic`
	selectTopicsQuery               = `SELECT topic FROM messages GROUP BY topic`
	selectAttachmentsSizeQuery      = `SELECT IFNULL(SUM(attachment_size), 0) FROM messages WHERE sender = ? AND attachment_expires >= ?`
)

// Schema management queries
const (
	currentSchemaVersion          = 9
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
)

type messageCache struct {
	db    *sql.DB
	queue *util.BatchingQueue[*message]
	nop   bool
}

// newSqliteCache creates a SQLite file-backed cache
func newSqliteCache(filename, startupQueries string, batchSize int, batchTimeout time.Duration, nop bool) (*messageCache, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupCacheDB(db, startupQueries); err != nil {
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
	return newSqliteCache(createMemoryFilename(), "", 0, 0, false)
}

// newNopCache creates an in-memory cache that discards all messages;
// it is always empty and can be used if caching is entirely disabled
func newNopCache() (*messageCache, error) {
	return newSqliteCache(createMemoryFilename(), "", 0, 0, true)
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
	start := time.Now()
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range ms {
		if m.Event != messageEvent {
			return errUnexpectedMessageType
		}
		published := m.Time <= time.Now().Unix()
		tags := strings.Join(m.Tags, ",")
		var attachmentName, attachmentType, attachmentURL string
		var attachmentSize, attachmentExpires int64
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
		_, err := tx.Exec(
			insertMessageQuery,
			m.ID,
			m.Time,
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
			m.Sender.String(),
			m.Encoding,
			published,
		)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		log.Error("Cache: Writing %d message(s) failed (took %v)", len(ms), time.Since(start))
		return err
	}
	log.Debug("Cache: Wrote %d message(s) in %v", len(ms), time.Since(start))
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

func (c *messageCache) Prune(olderThan time.Time) error {
	start := time.Now()
	if _, err := c.db.Exec(pruneMessagesQuery, olderThan.Unix()); err != nil {
		log.Warn("Cache: Pruning failed (after %v): %s", time.Since(start), err.Error())
	}
	log.Debug("Cache: Pruning successful (took %v)", time.Since(start))
	return nil
}

func (c *messageCache) AttachmentBytesUsed(sender string) (int64, error) {
	rows, err := c.db.Query(selectAttachmentsSizeQuery, sender, time.Now().Unix())
	if err != nil {
		return 0, err
	}
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
			log.Error("Cache: %s", err.Error())
		}
	}
}

func readMessages(rows *sql.Rows) ([]*message, error) {
	defer rows.Close()
	messages := make([]*message, 0)
	for rows.Next() {
		var timestamp, attachmentSize, attachmentExpires int64
		var priority int
		var id, topic, msg, title, tagsStr, click, icon, actionsStr, attachmentName, attachmentType, attachmentURL, sender, encoding string
		err := rows.Scan(
			&id,
			&timestamp,
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
			senderIP = netip.IPv4Unspecified() // if no IP stored in database, 0.0.0.0
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
		messages = append(messages, &message{
			ID:         id,
			Time:       timestamp,
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
			Encoding:   encoding,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func setupCacheDB(db *sql.DB, startupQueries string) error {
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
	} else if schemaVersion == 0 {
		return migrateFrom0(db)
	} else if schemaVersion == 1 {
		return migrateFrom1(db)
	} else if schemaVersion == 2 {
		return migrateFrom2(db)
	} else if schemaVersion == 3 {
		return migrateFrom3(db)
	} else if schemaVersion == 4 {
		return migrateFrom4(db)
	} else if schemaVersion == 5 {
		return migrateFrom5(db)
	} else if schemaVersion == 6 {
		return migrateFrom6(db)
	} else if schemaVersion == 7 {
		return migrateFrom7(db)
	} else if schemaVersion == 8 {
		return migrateFrom8(db)
	}
	return fmt.Errorf("unexpected schema version found: %d", schemaVersion)
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

func migrateFrom0(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 0 to 1")
	if _, err := db.Exec(migrate0To1AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(createSchemaVersionTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(insertSchemaVersion, 1); err != nil {
		return err
	}
	return migrateFrom1(db)
}

func migrateFrom1(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 1 to 2")
	if _, err := db.Exec(migrate1To2AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 2); err != nil {
		return err
	}
	return migrateFrom2(db)
}

func migrateFrom2(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 2 to 3")
	if _, err := db.Exec(migrate2To3AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 3); err != nil {
		return err
	}
	return migrateFrom3(db)
}

func migrateFrom3(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 3 to 4")
	if _, err := db.Exec(migrate3To4AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 4); err != nil {
		return err
	}
	return migrateFrom4(db)
}

func migrateFrom4(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 4 to 5")
	if _, err := db.Exec(migrate4To5AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 5); err != nil {
		return err
	}
	return migrateFrom5(db)
}

func migrateFrom5(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 5 to 6")
	if _, err := db.Exec(migrate5To6AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 6); err != nil {
		return err
	}
	return migrateFrom6(db)
}

func migrateFrom6(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 6 to 7")
	if _, err := db.Exec(migrate6To7AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 7); err != nil {
		return err
	}
	return migrateFrom7(db)
}

func migrateFrom7(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 7 to 8")
	if _, err := db.Exec(migrate7To8AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 8); err != nil {
		return err
	}
	return migrateFrom8(db)
}

func migrateFrom8(db *sql.DB) error {
	log.Info("Migrating cache database schema: from 8 to 9")
	if _, err := db.Exec(migrate8To9AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 9); err != nil {
		return err
	}
	return nil // Update this when a new version is added
}
