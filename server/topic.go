package server

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	id          string
	last        time.Time
	subscribers map[int]subscriber
	mu          sync.Mutex
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(msg *message) error

// newTopic creates a new topic
func newTopic(id string, last time.Time) *topic {
	return &topic{
		id:          id,
		last:        last,
		subscribers: make(map[int]subscriber),
	}
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = s
	t.last = time.Now()
	return subscriberID
}

// Unsubscribe removes the subscription from the list of subscribers
func (t *topic) Unsubscribe(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.subscribers, id)
}

// Publish asynchronously publishes to all subscribers
func (t *topic) Publish(m *message) error {
	go func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.last = time.Now()
		for _, s := range t.subscribers {
			if err := s(m); err != nil {
				log.Printf("error publishing message to subscriber")
			}
		}
	}()
	return nil
}

// Subscribers returns the number of subscribers to this topic
func (t *topic) Subscribers() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.subscribers)
}
