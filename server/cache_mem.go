package server

import (
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"sync"
	"time"
)

type memCache struct {
	messages map[string][]*message
	mu       sync.Mutex
}

var _ cache = (*memCache)(nil)

func newMemCache() *memCache {
	return &memCache{
		messages: make(map[string][]*message),
	}
}

func (s *memCache) AddMessage(m *message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[m.Topic]; !ok {
		s.messages[m.Topic] = make([]*message, 0)
	}
	s.messages[m.Topic] = append(s.messages[m.Topic], m)
	return nil
}

func (s *memCache) Messages(topic string, since time.Time) ([]*message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[topic]; !ok {
		return make([]*message, 0), nil
	}
	messages := make([]*message, 0) // copy!
	for _, m := range s.messages[topic] {
		msgTime := time.Unix(m.Time, 0)
		if msgTime == since || msgTime.After(since) {
			messages = append(messages, m)
		}
	}
	return messages, nil
}

func (s *memCache) MessageCount(topic string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[topic]; !ok {
		return 0, nil
	}
	return len(s.messages[topic]), nil
}

func (s *memCache) Topics() (map[string]*topic, error) {
	// Hack since we know when this is called there are no messages!
	return make(map[string]*topic), nil
}

func (s *memCache) Prune(keep time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, _ := range s.messages {
		s.pruneTopic(topic, keep)
	}
	return nil
}

func (s *memCache) pruneTopic(topic string, keep time.Duration) {
	for i, m := range s.messages[topic] {
		msgTime := time.Unix(m.Time, 0)
		if time.Since(msgTime) < keep {
			s.messages[topic] = s.messages[topic][i:]
			return
		}
	}
	s.messages[topic] = make([]*message, 0) // all messages expired
}
