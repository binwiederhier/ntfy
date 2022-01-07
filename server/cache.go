package server

import (
	"errors"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"time"
)

var (
	errUnexpectedMessageType = errors.New("unexpected message type")
)

// cache implements a cache for messages of type "message" events,
// i.e. message structs with the Event messageEvent.
type cache interface {
	AddMessage(m *message) error
	Messages(topic string, since sinceTime, scheduled bool) ([]*message, error)
	MessagesDue() ([]*message, error)
	MessageCount(topic string) (int, error)
	Topics() (map[string]*topic, error)
	Prune(olderThan time.Time) error
	MarkPublished(m *message) error
	AttachmentsSize(owner string) (int64, error)
	AttachmentsExpired() ([]string, error)
}
