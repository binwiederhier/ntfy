package server

import (
	"database/sql"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCache_Messages(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
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
	})
}

func TestCache_MessagesScheduled(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
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
	})
}

func TestCache_Topics(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
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
	})
}

func TestCache_MessagesTagsPrioAndTitle(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
		m := newDefaultMessage("mytopic", "some message")
		m.Tags = []string{"tag1", "tag2"}
		m.Priority = 5
		m.Title = "some title"
		require.Nil(t, c.AddMessage(m))

		messages, _ := c.Messages("mytopic", sinceAllMessages, false)
		require.Equal(t, []string{"tag1", "tag2"}, messages[0].Tags)
		require.Equal(t, 5, messages[0].Priority)
		require.Equal(t, "some title", messages[0].Title)
	})
}

func TestCache_MessagesSinceID(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
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
	})
}

func TestCache_Prune(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
		now := time.Now().Unix()

		m1 := newDefaultMessage("mytopic", "my message")
		m1.Time = now - 10
		m1.Expires = now - 5

		m2 := newDefaultMessage("mytopic", "my other message")
		m2.Time = now - 5
		m2.Expires = now + 5 // In the future

		m3 := newDefaultMessage("another_topic", "and another one")
		m3.Time = now - 12
		m3.Expires = now - 2

		require.Nil(t, c.AddMessage(m1))
		require.Nil(t, c.AddMessage(m2))
		require.Nil(t, c.AddMessage(m3))

		counts, err := c.MessageCounts()
		require.Nil(t, err)
		require.Equal(t, 2, counts["mytopic"])
		require.Equal(t, 1, counts["another_topic"])

		expiredMessageIDs, err := c.MessagesExpired()
		require.Nil(t, err)
		require.Nil(t, c.DeleteMessages(expiredMessageIDs...))

		counts, err = c.MessageCounts()
		require.Nil(t, err)
		require.Equal(t, 1, counts["mytopic"])
		require.Equal(t, 0, counts["another_topic"])

		messages, err := c.Messages("mytopic", sinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "my other message", messages[0].Message)
	})
}

func TestCache_Attachments(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
		expires1 := time.Now().Add(-4 * time.Hour).Unix() // Expired
		m := newDefaultMessage("mytopic", "flower for you")
		m.ID = "m1"
		m.Sender = netip.MustParseAddr("1.2.3.4")
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
		m.Sender = netip.MustParseAddr("1.2.3.4")
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
		m.User = "u_BAsbaAa"
		m.Sender = netip.MustParseAddr("5.6.7.8")
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
		require.Equal(t, "1.2.3.4", messages[0].Sender.String())

		require.Equal(t, "sending you a car", messages[1].Message)
		require.Equal(t, "car.jpg", messages[1].Attachment.Name)
		require.Equal(t, "image/jpeg", messages[1].Attachment.Type)
		require.Equal(t, int64(10000), messages[1].Attachment.Size)
		require.Equal(t, expires2, messages[1].Attachment.Expires)
		require.Equal(t, "https://ntfy.sh/file/aCaRURL.jpg", messages[1].Attachment.URL)
		require.Equal(t, "1.2.3.4", messages[1].Sender.String())

		size, err := c.AttachmentBytesUsedBySender("1.2.3.4")
		require.Nil(t, err)
		require.Equal(t, int64(10000), size)

		size, err = c.AttachmentBytesUsedBySender("5.6.7.8")
		require.Nil(t, err)
		require.Equal(t, int64(0), size) // Accounted to the user, not the IP!

		size, err = c.AttachmentBytesUsedByUser("u_BAsbaAa")
		require.Nil(t, err)
		require.Equal(t, int64(20000), size)
	})
}

func TestCache_AttachmentsExpired(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
		m := newDefaultMessage("mytopic", "flower for you")
		m.ID = "m1"
		m.Expires = time.Now().Add(time.Hour).Unix()
		require.Nil(t, c.AddMessage(m))

		m = newDefaultMessage("mytopic", "message with attachment")
		m.ID = "m2"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &attachment{
			Name:    "car.jpg",
			Type:    "image/jpeg",
			Size:    10000,
			Expires: time.Now().Add(2 * time.Hour).Unix(),
			URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		}
		require.Nil(t, c.AddMessage(m))

		m = newDefaultMessage("mytopic", "message with external attachment")
		m.ID = "m3"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &attachment{
			Name:    "car.jpg",
			Type:    "image/jpeg",
			Expires: 0, // Unknown!
			URL:     "https://somedomain.com/car.jpg",
		}
		require.Nil(t, c.AddMessage(m))

		m = newDefaultMessage("mytopic2", "message with expired attachment")
		m.ID = "m4"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &attachment{
			Name:    "expired-car.jpg",
			Type:    "image/jpeg",
			Size:    20000,
			Expires: time.Now().Add(-1 * time.Hour).Unix(),
			URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		}
		require.Nil(t, c.AddMessage(m))

		ids, err := c.AttachmentsExpired()
		require.Nil(t, err)
		require.Equal(t, 1, len(ids))
		require.Equal(t, "m4", ids[0])
	})
}

func TestCache_Sender(t *testing.T) {
	runMessageCacheTest(t, func(t *testing.T, c MessageCache) {
		m1 := newDefaultMessage("mytopic", "mymessage")
		m1.Sender = netip.MustParseAddr("1.2.3.4")
		require.Nil(t, c.AddMessage(m1))

		m2 := newDefaultMessage("mytopic", "mymessage without sender")
		require.Nil(t, c.AddMessage(m2))

		messages, err := c.Messages("mytopic", sinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))
		require.Equal(t, messages[0].Sender, netip.MustParseAddr("1.2.3.4"))
		require.Equal(t, messages[1].Sender, netip.Addr{})
	})
}

func newSqliteTestCache(t *testing.T) *sqliteMessageCache {
	c, err := newSqliteMessageCache(newSqliteTestCacheFile(t), "", time.Hour, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newSqliteTestCacheFile(t *testing.T) string {
	return filepath.Join(t.TempDir(), "cache.db")
}

func newSqliteTestCacheFromFile(t *testing.T, filename, startupQueries string) *sqliteMessageCache {
	c, err := newSqliteMessageCache(filename, startupQueries, time.Hour, 0, 0, false)
	require.Nil(t, err)
	return c
}

func newMemTestCache(t *testing.T) MessageCache {
	c, err := newMemCache()
	require.Nil(t, err)
	return c
}

func newPgTestCache(t *testing.T) MessageCache {
	connectionString := os.Getenv("NTFY_TEST_MESSAGES_CACHE_PG_CONNECTION_STRING")
	if connectionString == "" {
		t.Skip("Skipping test, because NTFY_TEST_MESSAGES_CACHE_PG_CONNECTION_STRING not set")
	}
	db, err := sql.Open("postgres", connectionString)
	require.Nil(t, err)
	_, err = db.Exec("DROP TABLE IF EXISTS messages")
	require.Nil(t, err)
	_, err = db.Exec("DROP TABLE IF EXISTS stats")
	require.Nil(t, err)
	_, err = db.Exec("DROP TABLE IF EXISTS schemaVersion")
	require.Nil(t, err)
	require.Nil(t, db.Close())
	c, err := newPgMessageCache(connectionString, "", 0, 0)
	require.Nil(t, err)
	return c
}

func runMessageCacheTest(t *testing.T, f func(t *testing.T, c MessageCache)) {
	t.Run(t.Name()+"_sqlite", func(t *testing.T) {
		f(t, newSqliteTestCache(t))
	})
	t.Run(t.Name()+"_mem", func(t *testing.T) {
		f(t, newMemTestCache(t))
	})
	t.Run(t.Name()+"_pg", func(t *testing.T) {
		f(t, newPgTestCache(t))
	})
}
