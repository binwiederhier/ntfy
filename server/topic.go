package server

import (
	"log"
	"math/rand"
	"sync"
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID          string
	subscribers map[int]subscriber
	mu          sync.Mutex
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(v *visitor, msg *message) error

// newTopic creates a new topic
func newTopic(id string) *topic {
	return &topic{
		ID:          id,
		subscribers: make(map[int]subscriber),
	}
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = s
	return subscriberID
}

// Unsubscribe removes the subscription from the list of subscribers
func (t *topic) Unsubscribe(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.subscribers, id)
}

// Publish asynchronously publishes to all subscribers
func (t *topic) Publish(v *visitor, m *message) error {
	go func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for _, s := range t.subscribers {
			if err := s(v, m); err != nil {
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
