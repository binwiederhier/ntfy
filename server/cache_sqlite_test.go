package server

import (
	"path/filepath"
	"testing"
)

func TestSqliteCache_AddMessage(t *testing.T) {
	testCacheMessages(t, newSqliteTestCache(t))
}

func TestSqliteCache_Topics(t *testing.T) {
	testCacheTopics(t, newSqliteTestCache(t))
}

func TestSqliteCache_MessagesTagsPrioAndTitle(t *testing.T) {
	testCacheMessagesTagsPrioAndTitle(t, newSqliteTestCache(t))
}

func TestSqliteCache_Prune(t *testing.T) {
	testCachePrune(t, newSqliteTestCache(t))
}

func newSqliteTestCache(t *testing.T) cache {
	filename := filepath.Join(t.TempDir(), "cache.db")
	c, err := newSqliteCache(filename)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
