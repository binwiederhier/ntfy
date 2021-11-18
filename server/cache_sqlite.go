package server

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"time"
)

const (
	createTableQuery = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id VARCHAR(20) PRIMARY KEY,
			time INT NOT NULL,
			topic VARCHAR(64) NOT NULL,
			message VARCHAR(1024) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		COMMIT;
	`
	insertMessageQuery           = `INSERT INTO messages (id, time, topic, message) VALUES (?, ?, ?, ?)`
	pruneMessagesQuery           = `DELETE FROM messages WHERE time < ?`
	selectMessagesSinceTimeQuery = `
		SELECT id, time, message 
		FROM messages 
		WHERE topic = ? AND time >= ?
		ORDER BY time ASC
	`
	selectMessageCountQuery = `SELECT count(*) FROM messages WHERE topic = ?`
	selectTopicsQuery       = `SELECT topic, MAX(time) FROM messages GROUP BY TOPIC`
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
	if _, err := db.Exec(createTableQuery); err != nil {
		return nil, err
	}
	return &sqliteCache{
		db: db,
	}, nil
}

func (c *sqliteCache) AddMessage(m *message) error {
	_, err := c.db.Exec(insertMessageQuery, m.ID, m.Time, m.Topic, m.Message)
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
		var id, msg string
		if err := rows.Scan(&id, &timestamp, &msg); err != nil {
			return nil, err
		}
		if msg == "" {
			msg = " " // Hack: never return empty messages; this should not happen
		}
		messages = append(messages, &message{
			ID:      id,
			Time:    timestamp,
			Event:   messageEvent,
			Topic:   topic,
			Message: msg,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *sqliteCache) MessageCount(topic string) (int, error) {
	rows, err := c.db.Query(selectMessageCountQuery, topic)
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

func (c *sqliteCache) Prune(keep time.Duration) error {
	_, err := c.db.Exec(pruneMessagesQuery, time.Now().Add(-1*keep).Unix())
	return err
}
