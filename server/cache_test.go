package server

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func testCacheMessages(t *testing.T, c cache) {
	m1 := newDefaultMessage("mytopic", "my message")
	m1.Time = 1

	m2 := newDefaultMessage("mytopic", "my other message")
	m2.Time = 2

	assert.Nil(t, c.AddMessage(m1))
	assert.Nil(t, c.AddMessage(newDefaultMessage("example", "my example message")))
	assert.Nil(t, c.AddMessage(m2))

	// Adding invalid
	assert.Equal(t, errUnexpectedMessageType, c.AddMessage(newKeepaliveMessage("mytopic"))) // These should not be added!
	assert.Equal(t, errUnexpectedMessageType, c.AddMessage(newOpenMessage("example")))      // These should not be added!

	// mytopic: count
	count, err := c.MessageCount("mytopic")
	assert.Nil(t, err)
	assert.Equal(t, 2, count)

	// mytopic: since all
	messages, _ := c.Messages("mytopic", sinceAllMessages, false)
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, "my message", messages[0].Message)
	assert.Equal(t, "mytopic", messages[0].Topic)
	assert.Equal(t, messageEvent, messages[0].Event)
	assert.Equal(t, "", messages[0].Title)
	assert.Equal(t, 0, messages[0].Priority)
	assert.Nil(t, messages[0].Tags)
	assert.Equal(t, "my other message", messages[1].Message)

	// mytopic: since none
	messages, _ = c.Messages("mytopic", sinceNoMessages, false)
	assert.Empty(t, messages)

	// mytopic: since 2
	messages, _ = c.Messages("mytopic", sinceTime(time.Unix(2, 0)), false)
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "my other message", messages[0].Message)

	// example: count
	count, err = c.MessageCount("example")
	assert.Nil(t, err)
	assert.Equal(t, 1, count)

	// example: since all
	messages, _ = c.Messages("example", sinceAllMessages, false)
	assert.Equal(t, "my example message", messages[0].Message)

	// non-existing: count
	count, err = c.MessageCount("doesnotexist")
	assert.Nil(t, err)
	assert.Equal(t, 0, count)

	// non-existing: since all
	messages, _ = c.Messages("doesnotexist", sinceAllMessages, false)
	assert.Empty(t, messages)
}

func testCacheTopics(t *testing.T, c cache) {
	assert.Nil(t, c.AddMessage(newDefaultMessage("topic1", "my example message")))
	assert.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 1")))
	assert.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 2")))
	assert.Nil(t, c.AddMessage(newDefaultMessage("topic2", "message 3")))

	topics, err := c.Topics()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(topics))
	assert.Equal(t, "topic1", topics["topic1"].ID)
	assert.Equal(t, "topic2", topics["topic2"].ID)
}

func testCachePrune(t *testing.T, c cache) {
	m1 := newDefaultMessage("mytopic", "my message")
	m1.Time = 1

	m2 := newDefaultMessage("mytopic", "my other message")
	m2.Time = 2

	m3 := newDefaultMessage("another_topic", "and another one")
	m3.Time = 1

	assert.Nil(t, c.AddMessage(m1))
	assert.Nil(t, c.AddMessage(m2))
	assert.Nil(t, c.AddMessage(m3))
	assert.Nil(t, c.Prune(time.Unix(2, 0)))

	count, err := c.MessageCount("mytopic")
	assert.Nil(t, err)
	assert.Equal(t, 1, count)

	count, err = c.MessageCount("another_topic")
	assert.Nil(t, err)
	assert.Equal(t, 0, count)

	messages, err := c.Messages("mytopic", sinceAllMessages, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "my other message", messages[0].Message)
}

func testCacheMessagesTagsPrioAndTitle(t *testing.T, c cache) {
	m := newDefaultMessage("mytopic", "some message")
	m.Tags = []string{"tag1", "tag2"}
	m.Priority = 5
	m.Title = "some title"
	assert.Nil(t, c.AddMessage(m))

	messages, _ := c.Messages("mytopic", sinceAllMessages, false)
	assert.Equal(t, []string{"tag1", "tag2"}, messages[0].Tags)
	assert.Equal(t, 5, messages[0].Priority)
	assert.Equal(t, "some title", messages[0].Title)
}

func testCacheMessagesScheduled(t *testing.T, c cache) {
	m1 := newDefaultMessage("mytopic", "message 1")
	m2 := newDefaultMessage("mytopic", "message 2")
	m2.Time = time.Now().Add(time.Hour).Unix()
	m3 := newDefaultMessage("mytopic", "message 3")
	m3.Time = time.Now().Add(time.Minute).Unix() // earlier than m2!
	m4 := newDefaultMessage("mytopic2", "message 4")
	m4.Time = time.Now().Add(time.Minute).Unix()
	assert.Nil(t, c.AddMessage(m1))
	assert.Nil(t, c.AddMessage(m2))
	assert.Nil(t, c.AddMessage(m3))

	messages, _ := c.Messages("mytopic", sinceAllMessages, false) // exclude scheduled
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "message 1", messages[0].Message)

	messages, _ = c.Messages("mytopic", sinceAllMessages, true) // include scheduled
	assert.Equal(t, 3, len(messages))
	assert.Equal(t, "message 1", messages[0].Message)
	assert.Equal(t, "message 3", messages[1].Message) // Order!
	assert.Equal(t, "message 2", messages[2].Message)

	messages, _ = c.MessagesDue()
	assert.Empty(t, messages)
}
