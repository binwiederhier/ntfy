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
			tags VARCHAR(256) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		COMMIT;
	`
	insertMessageQuery           = `INSERT INTO messages (id, time, topic, message, title, priority, tags) VALUES (?, ?, ?, ?, ?, ?, ?)`
	pruneMessagesQuery           = `DELETE FROM messages WHERE time < ?`
	selectMessagesSinceTimeQuery = `
		SELECT id, time, message, title, priority, tags
		FROM messages 
		WHERE topic = ? AND time >= ?
		ORDER BY time ASC
	`
	selectMessagesCountQuery        = `SELECT COUNT(*) FROM messages`
	selectMessageCountForTopicQuery = `SELECT COUNT(*) FROM messages WHERE topic = ?`
	selectTopicsQuery               = `SELECT topic, MAX(time) FROM messages GROUP BY topic`
)

// Schema management queries
const (
	currentSchemaVersion          = 1
	createSchemaVersionTableQuery = `
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
	`
	insertSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	selectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`

	// 0 -> 1
	migrate0To1AlterMessagesTableQuery = `
		BEGIN;
		ALTER TABLE messages ADD COLUMN title VARCHAR(256) NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN priority INT NOT NULL DEFAULT(0);
		ALTER TABLE messages ADD COLUMN tags VARCHAR(256) NOT NULL DEFAULT('');
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
	_, err := c.db.Exec(insertMessageQuery, m.ID, m.Time, m.Topic, m.Message, m.Title, m.Priority, strings.Join(m.Tags, ","))
	return err
}

func (c *sqliteCache) Messages(topic string, since sinceTime) ([]*message, error) {
	rows, err := c.db.Query(selectMessagesSinceTimeQuery, topic, since.Time().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]*message, 0)
	for rows.Next() {
		var timestamp int64
		var priority int
		var id, msg, title, tagsStr string
		if err := rows.Scan(&id, &timestamp, &msg, &title, &priority, &tagsStr); err != nil {
			return nil, err
		}
		if msg == "" {
			msg = " " // Hack: never return empty messages; this should not happen
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

func (s *sqliteCache) Topics() (map[string]*topic, error) {
	rows, err := s.db.Query(selectTopicsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	topics := make(map[string]*topic, 0)
	for rows.Next() {
		var id string
		var last int64
		if err := rows.Scan(&id, &last); err != nil {
			return nil, err
		}
		topics[id] = newTopic(id, time.Unix(last, 0))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return topics, nil
}

func (s *sqliteCache) Prune(keep time.Duration) error {
	_, err := s.db.Exec(pruneMessagesQuery, time.Now().Add(-1*keep).Unix())
	return err
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
		return migrateFrom0To1(db)
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

func migrateFrom0To1(db *sql.DB) error {
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
	return nil
}
