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
	Messages(topic string, since sinceTime) ([]*message, error)
	MessageCount(topic string) (int, error)
	Topics() (map[string]*topic, error)
	Prune(keep time.Duration) error
}
