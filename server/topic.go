package server

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	id          string
	subscribers map[int]subscriber
	messages    []*message
	last        time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(msg *message) error

// newTopic creates a new topic
func newTopic(id string) *topic {
	ctx, cancel := context.WithCancel(context.Background())
	return &topic{
		id:          id,
		subscribers: make(map[int]subscriber),
		last:        time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (t *topic) Subscribe(s subscriber) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = s
	t.last = time.Now()
	return subscriberID
}

func (t *topic) Unsubscribe(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.subscribers, id)
}

func (t *topic) Publish(m *message) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = time.Now()
	t.messages = append(t.messages, m)
	for _, s := range t.subscribers {
		if err := s(m); err != nil {
			log.Printf("error publishing message to subscriber")
		}
	}
	return nil
}

func (t *topic) Messages(since time.Time) []*message {
	t.mu.Lock()
	defer t.mu.Unlock()
	messages := make([]*message, 0) // copy!
	for _, m := range t.messages {
		msgTime := time.Unix(m.Time, 0)
		if msgTime == since || msgTime.After(since) {
			messages = append(messages, m)
		}
	}
	return messages
}

func (t *topic) Prune(keep time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, m := range t.messages {
		msgTime := time.Unix(m.Time, 0)
		if time.Since(msgTime) < keep {
			t.messages = t.messages[i:]
			return
		}
	}
	t.messages = make([]*message, 0)
}

func (t *topic) Stats() (subscribers int, messages int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.subscribers), len(t.messages)
}

func (t *topic) Close() {
	t.cancel()
}
