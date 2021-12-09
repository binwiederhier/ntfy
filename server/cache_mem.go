package server

import (
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
	if m.Event != messageEvent {
		return errUnexpectedMessageType
	}
	if _, ok := s.messages[m.Topic]; !ok {
		s.messages[m.Topic] = make([]*message, 0)
	}
	s.messages[m.Topic] = append(s.messages[m.Topic], m)
	return nil
}

func (s *memCache) Messages(topic string, since sinceTime) ([]*message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.messages[topic]; !ok || since.IsNone() {
		return make([]*message, 0), nil
	}
	messages := make([]*message, 0) // copy!
	for _, m := range s.messages[topic] {
		msgTime := time.Unix(m.Time, 0)
		if msgTime == since.Time() || msgTime.After(since.Time()) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make(map[string]*topic)
	for topic := range s.messages {
		topics[topic] = newTopic(topic)
	}
	return topics, nil
}

func (s *memCache) Prune(olderThan time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic := range s.messages {
		s.pruneTopic(topic, olderThan)
	}
	return nil
}

func (s *memCache) pruneTopic(topic string, olderThan time.Time) {
	messages := make([]*message, 0)
	for _, m := range s.messages[topic] {
		if m.Time >= olderThan.Unix() {
			messages = append(messages, m)
		}
	}
	s.messages[topic] = messages
}
