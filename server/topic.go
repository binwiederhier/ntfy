package server

import (
	"math/rand"
	"sync"
	"time"

	"heckel.io/ntfy/log"
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID          string
	subscribers map[int]topicSubscription
	lastUnsub   topicSubscription
	mu          sync.Mutex
}

// subscriber is a function that is called for every new message on a topic
type topicSubscription struct {
	f         subscriber
	v         *visitor
	unsubTime time.Time
}
type subscriber func(v *visitor, msg *message) error

// newTopic creates a new topic
func newTopic(id string) *topic {
	return &topic{
		ID:          id,
		subscribers: make(map[int]topicSubscription),
	}
}

// need a better name for bill?
// Returns nil, nil for non-UP topics
// returns visitor, nil for active UP topics
// returns nil, err for inactive UP topics
func (t *topic) getBillee() *visitor {
	//get a pseudo random visitor???
	for _, this_subscriber := range t.subscribers {
		return this_subscriber.v
	}
	// what if someone unsubscribed and DOESNT want their sub to count against them anymore, maybe the app server lost sync and will keep on sending stuff
	// I guess they suffer for unifiedPushSubscriptionDuration?

	// if lastunsub v exists, and the time since it was unsubbed is longer than our limit, it should not exist
	if t.lastUnsub.v != nil && time.Since(t.lastUnsub.unsubTime) > unifiedPushSubscriptionDuration {
		t.lastUnsub.v = nil
	}

	return t.lastUnsub.v
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber, v *visitor) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = topicSubscription{f: s, v: v}
	return subscriberID
}

// Unsubscribe removes the subscription from the list of subscribers
func (t *topic) Unsubscribe(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.subscribers) == 1 {
		t.lastUnsub = t.subscribers[id]
		t.lastUnsub.unsubTime = time.Now()
	}
	delete(t.subscribers, id)
}

// Publish asynchronously publishes to all subscribers
func (t *topic) Publish(v *visitor, m *message) error {
	go func() {
		// We want to lock the topic as short as possible, so we make a shallow copy of the
		// subscribers map here. Actually sending out the messages then doesn't have to lock.
		subscribers := t.subscribersCopy()
		if len(subscribers) > 0 {
			log.Debug("%s Forwarding to %d subscriber(s)", logMessagePrefix(v, m), len(subscribers))
			for _, s := range subscribers {
				// We call the subscriber functions in their own Go routines because they are blocking, and
				// we don't want individual slow subscribers to be able to block others.
				go func(s subscriber) {
					if err := s(v, m); err != nil {
						log.Warn("%s Error forwarding to subscriber", logMessagePrefix(v, m))
					}
				}(s.f)
			}
		} else {
			log.Trace("%s No stream or WebSocket subscribers, not forwarding", logMessagePrefix(v, m))
		}
	}()
	return nil
}

// SubscribersCount returns the number of subscribers to this topic
func (t *topic) SubscribersCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.subscribers)
}

// subscribersCopy returns a shallow copy of the subscribers map
func (t *topic) subscribersCopy() map[int]topicSubscription {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscribers := make(map[int]topicSubscription)
	for k, v := range t.subscribers {
		subscribers[k] = v
	}
	return subscribers
}
