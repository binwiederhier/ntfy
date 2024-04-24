package server

import (
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

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
	c := newSqliteTestCacheFromFile(t, filename, "")
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
	c := newSqliteTestCacheFromFile(t, filename, "")
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

func TestSqliteCache_Migration_From9(t *testing.T) {
	// This primarily tests the awkward migration that introduces the "expires" column.
	// The migration logic has to update the column, using the existing "cache-duration" value.

	filename := newSqliteTestCacheFile(t)
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)

	// Create "version 8" schema
	_, err = db.Exec(`
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
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);		
		INSERT INTO schemaVersion (id, version) VALUES (1, 9);
		COMMIT;
	`)
	require.Nil(t, err)

	// Insert a bunch of messages
	insertQuery := `
		INSERT INTO messages (mid, time, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, encoding, published) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	for i := 0; i < 10; i++ {
		_, err = db.Exec(
			insertQuery,
			fmt.Sprintf("abcd%d", i),
			time.Now().Unix(),
			"mytopic",
			fmt.Sprintf("some message %d", i),
			"",        // title
			0,         // priority
			"",        // tags
			"",        // click
			"",        // icon
			"",        // actions
			"",        // attachment_name
			"",        // attachment_type
			0,         // attachment_size
			0,         // attachment_type
			"",        // attachment_url
			"9.9.9.9", // sender
			"",        // encoding
			1,         // published
		)
		require.Nil(t, err)
	}

	// Create cache to trigger migration
	cacheDuration := 17 * time.Hour
	c, err := newSqliteMessageCache(filename, "", cacheDuration, 0, 0, false)
	require.Nil(t, err)
	checkSchemaVersion(t, c.db)

	// Check version
	rows, err := db.Query(`SELECT version FROM main.schemaVersion WHERE id = 1`)
	require.Nil(t, err)
	require.True(t, rows.Next())
	var version int
	require.Nil(t, rows.Scan(&version))
	require.Equal(t, currentSchemaVersion, version)

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))
	for _, m := range messages {
		require.True(t, m.Expires > time.Now().Add(cacheDuration-5*time.Second).Unix())
		require.True(t, m.Expires < time.Now().Add(cacheDuration+5*time.Second).Unix())
	}
}

func TestSqliteCache_StartupQueries_WAL(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := `pragma journal_mode = WAL; 
pragma synchronous = normal; 
pragma temp_store = memory;`
	db, err := newSqliteMessageCache(filename, startupQueries, time.Hour, 0, 0, false)
	require.Nil(t, err)
	require.Nil(t, db.AddMessage(newDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.FileExists(t, filename+"-wal")
	require.FileExists(t, filename+"-shm")
}

func TestSqliteCache_StartupQueries_None(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := ""
	db, err := newSqliteMessageCache(filename, startupQueries, time.Hour, 0, 0, false)
	require.Nil(t, err)
	require.Nil(t, db.AddMessage(newDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.NoFileExists(t, filename+"-wal")
	require.NoFileExists(t, filename+"-shm")
}

func TestSqliteCache_StartupQueries_Fail(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := `xx error`
	_, err := newSqliteMessageCache(filename, startupQueries, time.Hour, 0, 0, false)
	require.Error(t, err)
}

func TestMemCache_NopCache(t *testing.T) {
	c, _ := newNopCache()
	assert.Nil(t, c.AddMessage(newDefaultMessage("mytopic", "my message")))

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	assert.Nil(t, err)
	assert.Empty(t, messages)

	topics, err := c.Topics()
	assert.Nil(t, err)
	assert.Empty(t, topics)
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
