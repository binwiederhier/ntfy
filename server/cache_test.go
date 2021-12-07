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
	messages, _ := c.Messages("mytopic", sinceAllMessages)
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, "my message", messages[0].Message)
	assert.Equal(t, "mytopic", messages[0].Topic)
	assert.Equal(t, messageEvent, messages[0].Event)
	assert.Equal(t, "", messages[0].Title)
	assert.Equal(t, 0, messages[0].Priority)
	assert.Nil(t, messages[0].Tags)
	assert.Equal(t, "my other message", messages[1].Message)

	// mytopic: since none
	messages, _ = c.Messages("mytopic", sinceNoMessages)
	assert.Empty(t, messages)

	// mytopic: since 2
	messages, _ = c.Messages("mytopic", sinceTime(time.Unix(2, 0)))
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "my other message", messages[0].Message)

	// example: count
	count, err = c.MessageCount("example")
	assert.Nil(t, err)
	assert.Equal(t, 1, count)

	// example: since all
	messages, _ = c.Messages("example", sinceAllMessages)
	assert.Equal(t, "my example message", messages[0].Message)

	// non-existing: count
	count, err = c.MessageCount("doesnotexist")
	assert.Nil(t, err)
	assert.Equal(t, 0, count)

	// non-existing: since all
	messages, _ = c.Messages("doesnotexist", sinceAllMessages)
	assert.Empty(t, messages)
}

func testCacheMessagesTagsPrioAndTitle(t *testing.T, c cache) {
	m := newDefaultMessage("mytopic", "some message")
	m.Tags = []string{"tag1", "tag2"}
	m.Priority = 5
	m.Title = "some title"
	assert.Nil(t, c.AddMessage(m))

	messages, _ := c.Messages("mytopic", sinceAllMessages)
	assert.Equal(t, []string{"tag1", "tag2"}, messages[0].Tags)
	assert.Equal(t, 5, messages[0].Priority)
	assert.Equal(t, "some title", messages[0].Title)
}
