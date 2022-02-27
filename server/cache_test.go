package server

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testCacheMessages(t *testing.T, c cache) {
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
	count, err := c.MessageCount("mytopic")
	require.Nil(t, err)
	require.Equal(t, 2, count)

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
	count, err = c.MessageCount("example")
	require.Nil(t, err)
	require.Equal(t, 1, count)

	// example: since all
	messages, _ = c.Messages("example", sinceAllMessages, false)
	require.Equal(t, "my example message", messages[0].Message)

	// non-existing: count
	count, err = c.MessageCount("doesnotexist")
	require.Nil(t, err)
	require.Equal(t, 0, count)

	// non-existing: since all
	messages, _ = c.Messages("doesnotexist", sinceAllMessages, false)
	require.Empty(t, messages)
}

func testCacheTopics(t *testing.T, c cache) {
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

func testCachePrune(t *testing.T, c cache) {
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

	count, err := c.MessageCount("mytopic")
	require.Nil(t, err)
	require.Equal(t, 1, count)

	count, err = c.MessageCount("another_topic")
	require.Nil(t, err)
	require.Equal(t, 0, count)

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "my other message", messages[0].Message)
}

func testCacheMessagesTagsPrioAndTitle(t *testing.T, c cache) {
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

func testCacheMessagesScheduled(t *testing.T, c cache) {
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

func testCacheMessagesSinceID(t *testing.T, c cache) {
	m1 := newDefaultMessage("mytopic", "message 1")
	m1.Time = 100
	m2 := newDefaultMessage("mytopic", "message 2")
	m2.Time = 200
	m3 := newDefaultMessage("mytopic", "message 3")
	m3.Time = 300
	m4 := newDefaultMessage("mytopic", "message 4")
	m4.Time = 400
	m5 := newDefaultMessage("mytopic", "message 5")
	m5.Time = time.Now().Add(time.Minute).Unix() // Scheduled, in the future, later than m6
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
	require.Equal(t, 4, len(messages))
	require.Equal(t, "message 3", messages[0].Message)
	require.Equal(t, "message 4", messages[1].Message)
	require.Equal(t, "message 6", messages[2].Message) // Not scheduled m5!
	require.Equal(t, "message 7", messages[3].Message)

	// Case 2: Since ID exists, include scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m2.ID), true)
	require.Equal(t, 5, len(messages))
	require.Equal(t, "message 3", messages[0].Message)
	require.Equal(t, "message 4", messages[1].Message)
	require.Equal(t, "message 6", messages[2].Message)
	require.Equal(t, "message 7", messages[3].Message)
	require.Equal(t, "message 5", messages[4].Message) // Order!

	// Case 3: Since ID does not exist (-> Return all messages), include scheduled
	messages, _ = c.Messages("mytopic", newSinceID("doesntexist"), true)
	require.Equal(t, 7, len(messages))
	require.Equal(t, "message 1", messages[0].Message)
	require.Equal(t, "message 2", messages[1].Message)
	require.Equal(t, "message 3", messages[2].Message)
	require.Equal(t, "message 4", messages[3].Message)
	require.Equal(t, "message 6", messages[4].Message)
	require.Equal(t, "message 7", messages[5].Message)
	require.Equal(t, "message 5", messages[6].Message) // Order!

	// Case 4: Since ID exists and is last message (-> Return no messages), exclude scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m7.ID), false)
	require.Equal(t, 0, len(messages))

	// Case 5: Since ID exists and is last message (-> Return no messages), include scheduled
	messages, _ = c.Messages("mytopic", newSinceID(m7.ID), true)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "message 5", messages[0].Message)

	// FIXME This test still fails because the behavior of the code is incorrect.
	// TODO Add more delayed messages
}

func testCacheAttachments(t *testing.T, c cache) {
	expires1 := time.Now().Add(-4 * time.Hour).Unix()
	m := newDefaultMessage("mytopic", "flower for you")
	m.ID = "m1"
	m.Attachment = &attachment{
		Name:    "flower.jpg",
		Type:    "image/jpeg",
		Size:    5000,
		Expires: expires1,
		URL:     "https://ntfy.sh/file/AbDeFgJhal.jpg",
		Owner:   "1.2.3.4",
	}
	require.Nil(t, c.AddMessage(m))

	expires2 := time.Now().Add(2 * time.Hour).Unix() // Future
	m = newDefaultMessage("mytopic", "sending you a car")
	m.ID = "m2"
	m.Attachment = &attachment{
		Name:    "car.jpg",
		Type:    "image/jpeg",
		Size:    10000,
		Expires: expires2,
		URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		Owner:   "1.2.3.4",
	}
	require.Nil(t, c.AddMessage(m))

	expires3 := time.Now().Add(1 * time.Hour).Unix() // Future
	m = newDefaultMessage("another-topic", "sending you another car")
	m.ID = "m3"
	m.Attachment = &attachment{
		Name:    "another-car.jpg",
		Type:    "image/jpeg",
		Size:    20000,
		Expires: expires3,
		URL:     "https://ntfy.sh/file/zakaDHFW.jpg",
		Owner:   "1.2.3.4",
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
	require.Equal(t, "1.2.3.4", messages[0].Attachment.Owner)

	require.Equal(t, "sending you a car", messages[1].Message)
	require.Equal(t, "car.jpg", messages[1].Attachment.Name)
	require.Equal(t, "image/jpeg", messages[1].Attachment.Type)
	require.Equal(t, int64(10000), messages[1].Attachment.Size)
	require.Equal(t, expires2, messages[1].Attachment.Expires)
	require.Equal(t, "https://ntfy.sh/file/aCaRURL.jpg", messages[1].Attachment.URL)
	require.Equal(t, "1.2.3.4", messages[1].Attachment.Owner)

	size, err := c.AttachmentsSize("1.2.3.4")
	require.Nil(t, err)
	require.Equal(t, int64(30000), size)

	size, err = c.AttachmentsSize("5.6.7.8")
	require.Nil(t, err)
	require.Equal(t, int64(0), size)

	ids, err := c.AttachmentsExpired()
	require.Nil(t, err)
	require.Equal(t, []string{"m1"}, ids)
}
