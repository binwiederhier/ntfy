package server

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"
)

type topic struct {
	id          string
	subscribers map[int]subscriber
	messages    int
	last        time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
}

type subscriber func(msg *message) error

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
	if len(t.subscribers) == 0 {
		return errors.New("no subscribers")
	}
	t.last = time.Now()
	t.messages++
	for _, s := range t.subscribers {
		if err := s(m); err != nil {
			log.Printf("error publishing message to subscriber x")
		}
	}
	return nil
}

func (t *topic) Close() {
	t.cancel()
}
