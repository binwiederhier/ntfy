package server

import (
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
