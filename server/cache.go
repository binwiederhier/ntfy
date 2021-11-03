package server

import (
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"time"
)

type cache interface {
	AddMessage(m *message) error
	Messages(topic string, since time.Time) ([]*message, error)
	MessageCount(topic string) (int, error)
	Topics() (map[string]*topic, error)
	Prune(keep time.Duration) error
}
