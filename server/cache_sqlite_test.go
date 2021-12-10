package server

import (
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
	"time"
)

func TestSqliteCache_Messages(t *testing.T) {
	testCacheMessages(t, newSqliteTestCache(t))
}

func TestSqliteCache_MessagesScheduled(t *testing.T) {
	testCacheMessagesScheduled(t, newSqliteTestCache(t))
}

func TestSqliteCache_Topics(t *testing.T) {
	testCacheTopics(t, newSqliteTestCache(t))
}

func TestSqliteCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newSqliteTestCache(t))
}

func TestSqliteCache_Prune(t *testing.T) {
	testCachePrune(t, newSqliteTestCache(t))
}

func TestSqliteCache_Migration_From0(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	db, err := sql.Open("sqlite3", filename)
	assert.Nil(t, err)

	// Create "version 0" schema
	_, err = db.Exec(`
		BEGIN;
		CREATE TABLE IF NOT EXISTS messages (
			id VARCHAR(20) PRIMARY KEY,
			time INT NOT NULL,
			topic VARCHAR(64) NOT NULL,
			message VARCHAR(1024) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		COMMIT;
	`)
	assert.Nil(t, err)

	// Insert a bunch of messages
	for i := 0; i < 10; i++ {
		_, err = db.Exec(`INSERT INTO messages (id, time, topic, message) VALUES (?, ?, ?, ?)`,
			fmt.Sprintf("abcd%d", i), time.Now().Unix(), "mytopic", fmt.Sprintf("some message %d", i))
		assert.Nil(t, err)
	}

	// Create cache to trigger migration
	c := newSqliteTestCacheFromFile(t, filename)
	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	assert.Nil(t, err)
	assert.Equal(t, 10, len(messages))
	assert.Equal(t, "some message 5", messages[5].Message)
	assert.Equal(t, "", messages[5].Title)
	assert.Nil(t, messages[5].Tags)
	assert.Equal(t, 0, messages[5].Priority)

	rows, err := c.db.Query(`SELECT version  FROM schemaVersion`)
	assert.Nil(t, err)
	assert.True(t, rows.Next())

	var schemaVersion int
	assert.Nil(t, rows.Scan(&schemaVersion))
	assert.Equal(t, 2, schemaVersion)
}

func newSqliteTestCache(t *testing.T) *sqliteCache {
	c, err := newSqliteCache(newSqliteTestCacheFile(t))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newSqliteTestCacheFile(t *testing.T) string {
	return filepath.Join(t.TempDir(), "cache.db")
}

func newSqliteTestCacheFromFile(t *testing.T, filename string) *sqliteCache {
	c, err := newSqliteCache(filename)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
