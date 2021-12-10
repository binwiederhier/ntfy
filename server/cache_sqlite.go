package server

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"log"
	"strings"
	"time"
)

// Messages cache
const (
	createMessagesTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id VARCHAR(20) PRIMARY KEY,
			time INT NOT NULL,
			topic VARCHAR(64) NOT NULL,
			message VARCHAR(512) NOT NULL,
			title VARCHAR(256) NOT NULL,
			priority INT NOT NULL,
			tags VARCHAR(256) NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		COMMIT;
	`
	insertMessageQuery           = `INSERT INTO messages (id, time, topic, message, title, priority, tags, published) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	pruneMessagesQuery           = `DELETE FROM messages WHERE time < ?`
	selectMessagesSinceTimeQuery = `
		SELECT id, time, topic, message, title, priority, tags
		FROM messages 
		WHERE topic = ? AND time >= ? AND published = 1
		ORDER BY time ASC
	`
	selectMessagesSinceTimeIncludeScheduledQuery = `
		SELECT id, time, topic, message, title, priority, tags
		FROM messages 
		WHERE topic = ? AND time >= ?
		ORDER BY time ASC
	`
	selectMessagesDueQuery = `
		SELECT id, time, topic, message, title, priority, tags
		FROM messages 
		WHERE time <= ? AND published = 0
	`
	updateMessagePublishedQuery     = `UPDATE messages SET published = 1 WHERE id = ?`
	selectMessagesCountQuery        = `SELECT COUNT(*) FROM messages`
	selectMessageCountForTopicQuery = `SELECT COUNT(*) FROM messages WHERE topic = ?`
	selectTopicsQuery               = `SELECT topic FROM messages GROUP BY topic`
)

// Schema management queries
const (
	currentSchemaVersion          = 2
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
		ALTER TABLE messages ADD COLUMN title VARCHAR(256) NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN priority INT NOT NULL DEFAULT(0);
		ALTER TABLE messages ADD COLUMN tags VARCHAR(256) NOT NULL DEFAULT('');
		COMMIT;
	`

	// 1 -> 2
	migrate1To2AlterMessagesTableQuery = `
		BEGIN;
		ALTER TABLE messages ADD COLUMN published INT NOT NULL DEFAULT(1);
		COMMIT;
	`
)

type sqliteCache struct {
	db *sql.DB
}

var _ cache = (*sqliteCache)(nil)

func newSqliteCache(filename string) (*sqliteCache, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupDB(db); err != nil {
		return nil, err
	}
	return &sqliteCache{
		db: db,
	}, nil
}

func (c *sqliteCache) AddMessage(m *message) error {
	if m.Event != messageEvent {
		return errUnexpectedMessageType
	}
	published := m.Time <= time.Now().Unix()
	_, err := c.db.Exec(insertMessageQuery, m.ID, m.Time, m.Topic, m.Message, m.Title, m.Priority, strings.Join(m.Tags, ","), published)
	return err
}

func (c *sqliteCache) Messages(topic string, since sinceTime, scheduled bool) ([]*message, error) {
	if since.IsNone() {
		return make([]*message, 0), nil
	}
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

func (c *sqliteCache) MessagesDue() ([]*message, error) {
	rows, err := c.db.Query(selectMessagesDueQuery, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return readMessages(rows)
}

func (c *sqliteCache) MarkPublished(m *message) error {
	_, err := c.db.Exec(updateMessagePublishedQuery, m.ID)
	return err
}

func (c *sqliteCache) MessageCount(topic string) (int, error) {
	rows, err := c.db.Query(selectMessageCountForTopicQuery, topic)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int
	if !rows.Next() {
		return 0, errors.New("no rows found")
	}
	if err := rows.Scan(&count); err != nil {
		return 0, err
	} else if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func (c *sqliteCache) Topics() (map[string]*topic, error) {
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

func (c *sqliteCache) Prune(olderThan time.Time) error {
	_, err := c.db.Exec(pruneMessagesQuery, olderThan.Unix())
	return err
}

func readMessages(rows *sql.Rows) ([]*message, error) {
	defer rows.Close()
	messages := make([]*message, 0)
	for rows.Next() {
		var timestamp int64
		var priority int
		var id, topic, msg, title, tagsStr string
		if err := rows.Scan(&id, &timestamp, &topic, &msg, &title, &priority, &tagsStr); err != nil {
			return nil, err
		}
		var tags []string
		if tagsStr != "" {
			tags = strings.Split(tagsStr, ",")
		}
		messages = append(messages, &message{
			ID:       id,
			Time:     timestamp,
			Event:    messageEvent,
			Topic:    topic,
			Message:  msg,
			Title:    title,
			Priority: priority,
			Tags:     tags,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func setupDB(db *sql.DB) error {
	// If 'messages' table does not exist, this must be a new database
	rowsMC, err := db.Query(selectMessagesCountQuery)
	if err != nil {
		return setupNewDB(db)
	}
	defer rowsMC.Close()

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
	}

	// Do migrations
	if schemaVersion == currentSchemaVersion {
		return nil
	} else if schemaVersion == 0 {
		return migrateFrom0(db)
	} else if schemaVersion == 1 {
		return migrateFrom1(db)
	}
	return fmt.Errorf("unexpected schema version found: %d", schemaVersion)
}

func setupNewDB(db *sql.DB) error {
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
	log.Print("Migrating cache database schema: from 0 to 1")
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
	log.Print("Migrating cache database schema: from 1 to 2")
	if _, err := db.Exec(migrate1To2AlterMessagesTableQuery); err != nil {
		return err
	}
	if _, err := db.Exec(updateSchemaVersion, 2); err != nil {
		return err
	}
	return nil // Update this when a new version is added
}
