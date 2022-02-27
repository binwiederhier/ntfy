package server

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMemCache_Messages(t *testing.T) {
	testCacheMessages(t, newMemTestCache(t))
}

func TestMemCache_MessagesScheduled(t *testing.T) {
	testCacheMessagesScheduled(t, newMemTestCache(t))
}

func TestMemCache_Topics(t *testing.T) {
	testCacheTopics(t, newMemTestCache(t))
}

func TestMemCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newMemTestCache(t))
}

func TestMemCache_MessagesSinceID(t *testing.T) {
	testCacheMessagesSinceID(t, newMemTestCache(t))
}

func TestMemCache_Prune(t *testing.T) {
	testCachePrune(t, newMemTestCache(t))
}

func TestMemCache_Attachments(t *testing.T) {
	testCacheAttachments(t, newMemTestCache(t))
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

func newMemTestCache(t *testing.T) cache {
	c, err := newMemCache()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
