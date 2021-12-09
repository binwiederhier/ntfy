package server

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMemCache_Messages(t *testing.T) {
	testCacheMessages(t, newMemCache())
}

func TestMemCache_Topics(t *testing.T) {
	testCacheTopics(t, newMemCache())
}

func TestMemCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newMemCache())
}

func TestMemCache_Prune(t *testing.T) {
	testCachePrune(t, newMemCache())
}

func TestMemCache_NopCache(t *testing.T) {
	c := newNopCache()
	assert.Nil(t, c.AddMessage(newDefaultMessage("mytopic", "my message")))

	messages, err := c.Messages("mytopic", sinceAllMessages)
	assert.Nil(t, err)
	assert.Empty(t, messages)

	topics, err := c.Topics()
	assert.Nil(t, err)
	assert.Empty(t, topics)
}
