package server

import (
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestSqliteCache_Messages(t *testing.T) {
	testCacheMessages(t, newSqliteTestCache(t))
}

func TestMemCache_Messages(t *testing.T) {
	testCacheMessages(t, newMemTestCache(t))
}

func testCacheMessages(t *testing.T, c *messageCache) {
	m1 := newDefaultMessage("mytopic", "my message")
	m1.Time = 1

	m2 := newDefaultMessage("mytopic", "my other message")
	m2.Time = 2

	require.Nil(t, c.AddMessage(m1))
	require.Nil(t, c.AddMessage(newDefaultMessage("example", "my example message")))
	require.Nil(t, c.AddMessage(m2))

	// Adding invalid
	require.Equal(t, errUnexpectedMessageType, c.AddMessage(newKeepaliveMessage("mytopic"))) // These should not be added!
	require.Equal(t, errUnexpectedMessageType, c.AddMessage(newOpenMessage("example")))      // These should not be added!

	// mytopic: count
	counts, err := c.MessageCounts()
	require.Nil(t, err)
	require.Equal(t, 2, counts["mytopic"])

	// mytopic: since all
	messages, _ := c.Messages("mytopic", sinceAllMessages, false)
	require.Equal(t, 2, len(messages))
	require.Equal(t, "my message", messages[0].Message)
	require.Equal(t, "mytopic", messages[0].Topic)
	require.Equal(t, messageEvent, messages[0].Event)
	require.Equal(t, "", messages[0].Title)
	require.Equal(t, 0, messages[0].Priority)
	require.Nil(t, messages[0].Tags)
	require.Equal(t, "my other message", messages[1].Message)

	// mytopic: since none
	messages, _ = c.Messages("mytopic", sinceNoMessages, false)
	require.Empty(t, messages)

	// mytopic: since m1 (by ID)
	messages, _ = c.Messages("mytopic", newSinceID(m1.ID), false)
	require.Equal(t, 1, len(messages))
	require.Equal(t, m2.ID, messages[0].ID)
	require.Equal(t, "my other message", messages[0].Message)
	require.Equal(t, "mytopic", messages[0].Topic)

	// mytopic: since 2
	messages, _ = c.Messages("mytopic", newSinceTime(2), false)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "my other message", messages[0].Message)

	// example: count
	counts, err = c.MessageCounts()
	require.Nil(t, err)
	require.Equal(t, 1, counts["example"])

	// example: since all
	messages, _ = c.Messages("example", sinceAllMessages, false)
	require.Equal(t, "my example message", messages[0].Message)

	// non-existing: count
	counts, err = c.MessageCounts()
	require.Nil(t, err)
	require.Equal(t, 0, counts["doesnotexist"])

	// non-existing: since all
	messages, _ = c.Messages("doesnotexist", sinceAllMessages, false)
	require.Empty(t, messages)
}

func TestSqliteCache_MessagesScheduled(t *testing.T) {
	testCacheMessagesScheduled(t, newSqliteTestCache(t))
}

func TestMemCache_MessagesScheduled(t *testing.T) {
	testCacheMessagesScheduled(t, newMemTestCache(t))
}

func testCacheMessagesScheduled(t *testing.T, c *messageCache) {
	m1 := newDefaultMessage("mytopic", "message 1")
	m2 := newDefaultMessage("mytopic", "message 2")
	m2.Time = time.Now().Add(time.Hour).Unix()
	m3 := newDefaultMessage("mytopic", "message 3")
	m3.Time = time.Now().Add(time.Minute).Unix() // earlier than m2!
	m4 := newDefaultMessage("mytopic2", "message 4")
	m4.Time = time.Now().Add(time.Minute).Unix()
	require.Nil(t, c.AddMessage(m1))
	require.Nil(t, c.AddMessage(m2))
	require.Nil(t, c.AddMessage(m3))

	messages, _ := c.Messages("mytopic", sinceAllMessages, false) // exclude scheduled
	require.Equal(t, 1, len(messages))
	require.Equal(t, "message 1", messages[0].Message)

	messages, _ = c.Messages("mytopic", sinceAllMessages, true) // include scheduled
	require.Equal(t, 3, len(messages))
	require.Equal(t, "message 1", messages[0].Message)
	require.Equal(t, "message 3", messages[1].Message) // Order!
	require.Equal(t, "message 2", messages[2].Message)

	messages, _ = c.MessagesDue()
	require.Empty(t, messages)
}

func TestSqliteCache_Topics(t *testing.T) {
	testCacheTopics(t, newSqliteTestCache(t))
}

func TestMemCache_Topics(t *testing.T) {
	testCacheTopics(t, newMemTestCache(t))
}

func testCacheTopics(t *testing.T, c *messageCache) {
	require.Nil(t, c.AddMessage(newDefaultMessage("topic1", "my example message")))
	require.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 1")))
	require.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 2")))
	require.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 3")))

	topics, err := c.Topics()
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, 2, len(topics))
	require.Equal(t, "topic1", topics["topic1"].ID)
	require.Equal(t, "topic2", topics["topic2"].ID)
}

func TestSqliteCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newSqliteTestCache(t))
}

func TestMemCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newMemTestCache(t))
}

func testCacheMessagesTagsPrioAndTitle(t *testing.T, c *messageCache) {
	m := newDefaultMessage("mytopic", "some message")
	m.Tags = []string{"tag1", "tag2"}
	m.Priority = 5
	m.Title = "some title"
	require.Nil(t, c.AddMessage(m))

	messages, _ := c.Messages("mytopic", sinceAllMessages, false)
	require.Equal(t, []string{"tag1", "tag2"}, messages[0].Tags)
	require.Equal(t, 5, messages[0].Priority)
	require.Equal(t, "some title", messages[0].Title)
}

func TestSqliteCache_MessagesSinceID(t *testing.T) {
	testCacheMessagesSinceID(t, newSqliteTestCache(t))
}

func TestMemCache_MessagesSinceID(t *testing.T) {
	testCacheMessagesSinceID(t, newMemTestCache(t))
}

func testCacheMessagesSinceID(t *testing.T, c *messageCache) {
	m1 := newDefaultMessage("mytopic", "message 1")
	m1.Time = 100
	m2 := newDefaultMessage("mytopic", "message 2")
	m2.Time = 200
	m3 := newDefaultMessage("mytopic", "message 3")
	m3.Time = time.Now().Add(time.Hour).Unix() // Scheduled, in the future, later than m7 and m5
	m4 := newDefaultMessage("mytopic", "message 4")
	m4.Time = 400
	m5 := newDefaultMessage("mytopic", "message 5")
	m5.Time = time.Now().Add(time.Minute).Unix() // Scheduled, in the future, later than m7
	m6 := newDefaultMessage("mytopic", "message 6")
	m6.Time = 600
	m7 := newDefaultMessage("mytopic", "message 7")
	m7.Time = 700

	require.Nil(t, c.AddMessage(m1))
	require.Nil(t, c.AddMessage(m2))
	require.Nil(t, c.AddMessage(m3))
	require.Nil(t, c.AddMessage(m4))
	require.Nil(t, c.AddMessage(m5))
	require.Nil(t, c.AddMessage(m6))
	require.Nil(t, c.AddMessage(m7))

	// Case 1: Since ID exists, exclude scheduled
	messages, _ := c.Messages("mytopic", newSinceID(m2.ID), false)
	require.Equal(t, 3, len(messages))
	require.Equal(t, "message 4", messages[0].Message)
	require.Equal(t, "message 6", messages[1].Message) // Not scheduled m3/m5!
	require.Equal(t, "message 7", messages[2].Message)

	// Case 2: Since ID exists, include scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m2.ID), true)
	require.Equal(t, 5, len(messages))
	require.Equal(t, "message 4", messages[0].Message)
	require.Equal(t, "message 6", messages[1].Message)
	require.Equal(t, "message 7", messages[2].Message)
	require.Equal(t, "message 5", messages[3].Message) // Order!
	require.Equal(t, "message 3", messages[4].Message) // Order!

	// Case 3: Since ID does not exist (-> Return all messages), include scheduled
	messages, _ = c.Messages("mytopic", newSinceID("doesntexist"), true)
	require.Equal(t, 7, len(messages))
	require.Equal(t, "message 1", messages[0].Message)
	require.Equal(t, "message 2", messages[1].Message)
	require.Equal(t, "message 4", messages[2].Message)
	require.Equal(t, "message 6", messages[3].Message)
	require.Equal(t, "message 7", messages[4].Message)
	require.Equal(t, "message 5", messages[5].Message) // Order!
	require.Equal(t, "message 3", messages[6].Message) // Order!

	// Case 4: Since ID exists and is last message (-> Return no messages), exclude scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m7.ID), false)
	require.Equal(t, 0, len(messages))

	// Case 5: Since ID exists and is last message (-> Return no messages), include scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m7.ID), true)
	require.Equal(t, 2, len(messages))
	require.Equal(t, "message 5", messages[0].Message)
	require.Equal(t, "message 3", messages[1].Message)
}

func TestSqliteCache_Prune(t *testing.T) {
	testCachePrune(t, newSqliteTestCache(t))
}

func TestMemCache_Prune(t *testing.T) {
	testCachePrune(t, newMemTestCache(t))
}

func testCachePrune(t *testing.T, c *messageCache) {
	m1 := newDefaultMessage("mytopic", "my message")
	m1.Time = 1

	m2 := newDefaultMessage("mytopic", "my other message")
	m2.Time = 2

	m3 := newDefaultMessage("another_topic", "and another one")
	m3.Time = 1

	require.Nil(t, c.AddMessage(m1))
	require.Nil(t, c.AddMessage(m2))
	require.Nil(t, c.AddMessage(m3))
	require.Nil(t, c.Prune(time.Unix(2, 0)))

	counts, err := c.MessageCounts()
	require.Nil(t, err)
	require.Equal(t, 1, counts["mytopic"])

	counts, err = c.MessageCounts()
	require.Nil(t, err)
	require.Equal(t, 0, counts["another_topic"])

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "my other message", messages[0].Message)
}

func TestSqliteCache_Attachments(t *testing.T) {
	testCacheAttachments(t, newSqliteTestCache(t))
}

func TestMemCache_Attachments(t *testing.T) {
	testCacheAttachments(t, newMemTestCache(t))
}

func testCacheAttachments(t *testing.T, c *messageCache) {
	expires1 := time.Now().Add(-4 * time.Hour).Unix()
	m := newDefaultMessage("mytopic", "flower for you")
	m.ID = "m1"
	m.Sender = "1.2.3.4"
	m.Attachment = &attachment{
		Name:    "flower.jpg",
		Type:    "image/jpeg",
		Size:    5000,
		Expires: expires1,
		URL:     "https://ntfy.sh/file/AbDeFgJhal.jpg",
	}
	require.Nil(t, c.AddMessage(m))

	expires2 := time.Now().Add(2 * time.Hour).Unix() // Future
	m = newDefaultMessage("mytopic", "sending you a car")
	m.ID = "m2"
	m.Sender = "1.2.3.4"
	m.Attachment = &attachment{
		Name:    "car.jpg",
		Type:    "image/jpeg",
		Size:    10000,
		Expires: expires2,
		URL:     "https://ntfy.sh/file/aCaRURL.jpg",
	}
	require.Nil(t, c.AddMessage(m))

	expires3 := time.Now().Add(1 * time.Hour).Unix() // Future
	m = newDefaultMessage("another-topic", "sending you another car")
	m.ID = "m3"
	m.Sender = "1.2.3.4"
	m.Attachment = &attachment{
		Name:    "another-car.jpg",
		Type:    "image/jpeg",
		Size:    20000,
		Expires: expires3,
		URL:     "https://ntfy.sh/file/zakaDHFW.jpg",
	}
	require.Nil(t, c.AddMessage(m))

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 2, len(messages))

	require.Equal(t, "flower for you", messages[0].Message)
	require.Equal(t, "flower.jpg", messages[0].Attachment.Name)
	require.Equal(t, "image/jpeg", messages[0].Attachment.Type)
	require.Equal(t, int64(5000), messages[0].Attachment.Size)
	require.Equal(t, expires1, messages[0].Attachment.Expires)
	require.Equal(t, "https://ntfy.sh/file/AbDeFgJhal.jpg", messages[0].Attachment.URL)
	require.Equal(t, "1.2.3.4", messages[0].Sender)

	require.Equal(t, "sending you a car", messages[1].Message)
	require.Equal(t, "car.jpg", messages[1].Attachment.Name)
	require.Equal(t, "image/jpeg", messages[1].Attachment.Type)
	require.Equal(t, int64(10000), messages[1].Attachment.Size)
	require.Equal(t, expires2, messages[1].Attachment.Expires)
	require.Equal(t, "https://ntfy.sh/file/aCaRURL.jpg", messages[1].Attachment.URL)
	require.Equal(t, "1.2.3.4", messages[1].Sender)

	size, err := c.AttachmentBytesUsed("1.2.3.4")
	require.Nil(t, err)
	require.Equal(t, int64(30000), size)

	size, err = c.AttachmentBytesUsed("5.6.7.8")
	require.Nil(t, err)
	require.Equal(t, int64(0), size)
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

func TestSqliteCache_StartupQueries_WAL(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := `pragma journal_mode = WAL; 
pragma synchronous = normal; 
pragma temp_store = memory;`
	db, err := newSqliteCache(filename, startupQueries, false)
	require.Nil(t, err)
	require.Nil(t, db.AddMessage(newDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.FileExists(t, filename+"-wal")
	require.FileExists(t, filename+"-shm")
}

func TestSqliteCache_StartupQueries_None(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := ""
	db, err := newSqliteCache(filename, startupQueries, false)
	require.Nil(t, err)
	require.Nil(t, db.AddMessage(newDefaultMessage("mytopic", "some message")))
	require.FileExists(t, filename)
	require.NoFileExists(t, filename+"-wal")
	require.NoFileExists(t, filename+"-shm")
}

func TestSqliteCache_StartupQueries_Fail(t *testing.T) {
	filename := newSqliteTestCacheFile(t)
	startupQueries := `xx error`
	_, err := newSqliteCache(filename, startupQueries, false)
	require.Error(t, err)
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

func newSqliteTestCache(t *testing.T) *messageCache {
	c, err := newSqliteCache(newSqliteTestCacheFile(t), "", false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newSqliteTestCacheFile(t *testing.T) string {
	return filepath.Join(t.TempDir(), "cache.db")
}

func newSqliteTestCacheFromFile(t *testing.T, filename, startupQueries string) *messageCache {
	c, err := newSqliteCache(filename, startupQueries, false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newMemTestCache(t *testing.T) *messageCache {
	c, err := newMemCache()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
