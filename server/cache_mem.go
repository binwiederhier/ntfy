package server

import (
	"sort"
	"sync"
	"time"
)

type memCache struct {
	messages  map[string][]*message
	scheduled map[string]*message // Message ID -> message
	nop       bool
	mu        sync.Mutex
}

var _ cache = (*memCache)(nil)

// newMemCache creates an in-memory cache
func newMemCache() *memCache {
	return &memCache{
		messages:  make(map[string][]*message),
		scheduled: make(map[string]*message),
		nop:       false,
	}
}

// newNopCache creates an in-memory cache that discards all messages;
// it is always empty and can be used if caching is entirely disabled
func newNopCache() *memCache {
	return &memCache{
		messages:  make(map[string][]*message),
		scheduled: make(map[string]*message),
		nop:       true,
	}
}

func (c *memCache) AddMessage(m *message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nop {
		return nil
	}
	if m.Event != messageEvent {
		return errUnexpectedMessageType
	}
	if _, ok := c.messages[m.Topic]; !ok {
		c.messages[m.Topic] = make([]*message, 0)
	}
	delayed := m.Time > time.Now().Unix()
	if delayed {
		c.scheduled[m.ID] = m
	}
	c.messages[m.Topic] = append(c.messages[m.Topic], m)
	return nil
}

func (c *memCache) Messages(topic string, since sinceTime, scheduled bool) ([]*message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.messages[topic]; !ok || since.IsNone() {
		return make([]*message, 0), nil
	}
	messages := make([]*message, 0)
	for _, m := range c.messages[topic] {
		_, messageScheduled := c.scheduled[m.ID]
		include := m.Time >= since.Time().Unix() && (!messageScheduled || scheduled)
		if include {
			messages = append(messages, m)
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Time < messages[j].Time
	})
	return messages, nil
}

func (c *memCache) MessagesDue() ([]*message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	messages := make([]*message, 0)
	for _, m := range c.scheduled {
		due := time.Now().Unix() >= m.Time
		if due {
			messages = append(messages, m)
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Time < messages[j].Time
	})
	return messages, nil
}

func (c *memCache) MarkPublished(m *message) error {
	c.mu.Lock()
	delete(c.scheduled, m.ID)
	c.mu.Unlock()
	return nil
}

func (c *memCache) MessageCount(topic string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.messages[topic]; !ok {
		return 0, nil
	}
	return len(c.messages[topic]), nil
}

func (c *memCache) Topics() (map[string]*topic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	topics := make(map[string]*topic)
	for topic := range c.messages {
		topics[topic] = newTopic(topic)
	}
	return topics, nil
}

func (c *memCache) Prune(olderThan time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for topic := range c.messages {
		c.pruneTopic(topic, olderThan)
	}
	return nil
}

func (c *memCache) AttachmentsSize(owner string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var size int64
	for topic := range c.messages {
		for _, m := range c.messages[topic] {
			if m.Attachment != nil && m.Attachment.Owner == owner {
				size += m.Attachment.Size
			}
		}
	}
	return size, nil
}

func (c *memCache) AttachmentsExpired() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0)
	for topic := range c.messages {
		for _, m := range c.messages[topic] {
			if m.Attachment != nil && m.Attachment.Expires > 0 && m.Attachment.Expires < time.Now().Unix() {
				ids = append(ids, m.ID)
			}
		}
	}
	return ids, nil
}

func (c *memCache) pruneTopic(topic string, olderThan time.Time) {
	messages := make([]*message, 0)
	for _, m := range c.messages[topic] {
		if m.Time >= olderThan.Unix() {
			messages = append(messages, m)
		}
	}
	c.messages[topic] = messages
}
