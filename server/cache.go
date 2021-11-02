package server

import (
	"database/sql"
	"time"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	createTableQuery = `CREATE TABLE IF NOT EXISTS messages (
		id VARCHAR(20) PRIMARY KEY,
		time INT NOT NULL,
		topic VARCHAR(64) NOT NULL,
		message VARCHAR(1024) NOT NULL
	)`
	insertQuery         = `INSERT INTO messages (id, time, topic, message) VALUES (?, ?, ?, ?)`
	pruneOlderThanQuery = `DELETE FROM messages WHERE time < ?`
)

type cache struct {
	db *sql.DB
	insert *sql.Stmt
	prune *sql.Stmt
}

func newCache(filename string) (*cache, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(createTableQuery); err != nil {
		return nil, err
	}
	insert, err := db.Prepare(insertQuery)
	if err != nil {
		return nil, err
	}
	prune, err := db.Prepare(pruneOlderThanQuery)
	if err != nil {
		return nil, err
	}
	return &cache{
		db:     db,
		insert: insert,
		prune:  prune,
	}, nil
}

func (c *cache) Load() (map[string]*topic, error) {

}

func (c *cache) Add(m *message) error {
	_, err := c.insert.Exec(m.ID, m.Time, m.Topic, m.Message)
	return err
}

func (c *cache) Prune(olderThan time.Duration) error {
	_, err := c.prune.Exec(time.Now().Add(-1 * olderThan).Unix())
	return err
}
