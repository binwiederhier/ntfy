package server

import (
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/require"
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
	require.Nil(t, err)

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
	require.Nil(t, err)

	// Insert a bunch of messages
	for i := 0; i < 10; i++ {
		_, err = db.Exec(`INSERT INTO messages (id, time, topic, message) VALUES (?, ?, ?, ?)`,
			fmt.Sprintf("abcd%d", i), time.Now().Unix(), "mytopic", fmt.Sprintf("some message %d", i))
		require.Nil(t, err)
	}
	require.Nil(t, db.Close())

	// Create cache to trigger migration
	c := newSqliteTestCacheFromFile(t, filename)
	checkSchemaVersion(t, c.db)

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))
	require.Equal(t, "some message 5", messages[5].Message)
	require.Equal(t, "", messages[5].Title)
	require.Nil(t, messages[5].Tags)
	require.Equal(t, 0, messages[5].Priority)
}

func TestSqliteCache_Migration_From1(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)

	// Create "version 1" schema
	_, err = db.Exec(`
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
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);		
		INSERT INTO schemaVersion (id, version) VALUES (1, 1);
	`)
	require.Nil(t, err)

	// Insert a bunch of messages
	for i := 0; i < 10; i++ {
		_, err = db.Exec(`INSERT INTO messages (id, time, topic, message, title, priority, tags) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("abcd%d", i), time.Now().Unix(), "mytopic", fmt.Sprintf("some message %d", i), "", 0, "")
		require.Nil(t, err)
	}
	require.Nil(t, db.Close())

	// Create cache to trigger migration
	c := newSqliteTestCacheFromFile(t, filename)
	checkSchemaVersion(t, c.db)

	// Add delayed message
	delayedMessage := newDefaultMessage("mytopic", "some delayed message")
	delayedMessage.Time = time.Now().Add(time.Minute).Unix()
	require.Nil(t, c.AddMessage(delayedMessage))

	// 10, not 11!
	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))

	// 11!
	messages, err = c.Messages("mytopic", sinceAllMessages, true)
	require.Nil(t, err)
	require.Equal(t, 11, len(messages))
}

func checkSchemaVersion(t *testing.T, db *sql.DB) {
	rows, err := db.Query(`SELECT version FROM schemaVersion`)
	require.Nil(t, err)
	require.True(t, rows.Next())

	var schemaVersion int
	require.Nil(t, rows.Scan(&schemaVersion))
	require.Equal(t, currentSchemaVersion, schemaVersion)
	require.Nil(t, rows.Close())
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
