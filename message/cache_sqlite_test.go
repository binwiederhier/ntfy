package message_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func TestSqliteStore_Migration_From0(t *testing.T) {
	filename := newSqliteTestStoreFile(t)
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

	// Create store to trigger migration
	s := newSqliteTestStoreFromFile(t, filename, "")
	checkSqliteSchemaVersion(t, filename)

	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))
	require.Equal(t, "some message 5", messages[5].Message)
	require.Equal(t, "", messages[5].Title)
	require.Nil(t, messages[5].Tags)
	require.Equal(t, 0, messages[5].Priority)
}

func TestSqliteStore_Migration_From1(t *testing.T) {
	filename := newSqliteTestStoreFile(t)
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

	// Create store to trigger migration
	s := newSqliteTestStoreFromFile(t, filename, "")
	checkSqliteSchemaVersion(t, filename)

	// Add delayed message
	delayedMessage := model.NewDefaultMessage("mytopic", "some delayed message")
	delayedMessage.Time = time.Now().Add(time.Minute).Unix()
	require.Nil(t, s.AddMessage(delayedMessage))

	// 10, not 11!
	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))

	// 11!
	messages, err = s.Messages("mytopic", model.SinceAllMessages, true)
	require.Nil(t, err)
	require.Equal(t, 11, len(messages))

	// Check that index "idx_topic" exists
	verifyDB, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)
	defer verifyDB.Close()
	rows, err := verifyDB.Query(`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_topic'`)
	require.Nil(t, err)
	require.True(t, rows.Next())
	var indexName string
	require.Nil(t, rows.Scan(&indexName))
	require.Equal(t, "idx_topic", indexName)
	require.Nil(t, rows.Close())
}

func TestSqliteStore_Migration_From9(t *testing.T) {
	// This primarily tests the awkward migration that introduces the "expires" column.
	// The migration logic has to update the column, using the existing "cache-duration" value.

	filename := newSqliteTestStoreFile(t)
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)

	// Create "version 9" schema
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
			0,         // attachment_expires
			"",        // attachment_url
			"9.9.9.9", // sender
			"",        // encoding
			1,         // published
		)
		require.Nil(t, err)
	}
	require.Nil(t, db.Close())

	// Create store to trigger migration
	cacheDuration := 17 * time.Hour
	s, err := message.NewSQLiteStore(filename, "", cacheDuration, 0, 0, false)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	checkSqliteSchemaVersion(t, filename)

	// Check version
	verifyDB, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)
	defer verifyDB.Close()
	rows, err := verifyDB.Query(`SELECT version FROM schemaVersion WHERE id = 1`)
	require.Nil(t, err)
	require.True(t, rows.Next())
	var version int
	require.Nil(t, rows.Scan(&version))
	require.Equal(t, 15, version)
	require.Nil(t, rows.Close())

	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 10, len(messages))
	for _, m := range messages {
		require.True(t, m.Expires > time.Now().Add(cacheDuration-5*time.Second).Unix())
		require.True(t, m.Expires < time.Now().Add(cacheDuration+5*time.Second).Unix())
	}
}

func TestSqliteStore_StartupQueries_WAL(t *testing.T) {
	filename := newSqliteTestStoreFile(t)
	startupQueries := `pragma journal_mode = WAL;
pragma synchronous = normal;
pragma temp_store = memory;`
	s, err := message.NewSQLiteStore(filename, startupQueries, time.Hour, 0, 0, false)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.FileExists(t, filename+"-wal")
	require.FileExists(t, filename+"-shm")
}

func TestSqliteStore_StartupQueries_None(t *testing.T) {
	filename := newSqliteTestStoreFile(t)
	s, err := message.NewSQLiteStore(filename, "", time.Hour, 0, 0, false)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.NoFileExists(t, filename+"-wal")
	require.NoFileExists(t, filename+"-shm")
}

func TestSqliteStore_StartupQueries_Fail(t *testing.T) {
	filename := newSqliteTestStoreFile(t)
	_, err := message.NewSQLiteStore(filename, `xx error`, time.Hour, 0, 0, false)
	require.Error(t, err)
}

func TestNopStore(t *testing.T) {
	s, err := message.NewNopStore()
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "my message")))

	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Empty(t, messages)

	topics, err := s.Topics()
	require.Nil(t, err)
	require.Empty(t, topics)
}

func newSqliteTestStoreFile(t *testing.T) string {
	return filepath.Join(t.TempDir(), "cache.db")
}

func newSqliteTestStoreFromFile(t *testing.T, filename, startupQueries string) *message.Cache {
	s, err := message.NewSQLiteStore(filename, startupQueries, time.Hour, 0, 0, false)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func checkSqliteSchemaVersion(t *testing.T, filename string) {
	db, err := sql.Open("sqlite3", filename)
	require.Nil(t, err)
	defer db.Close()
	rows, err := db.Query(`SELECT version FROM schemaVersion`)
	require.Nil(t, err)
	require.True(t, rows.Next())
	var schemaVersion int
	require.Nil(t, rows.Scan(&schemaVersion))
	require.Equal(t, 15, schemaVersion)
	require.Nil(t, rows.Close())
}
