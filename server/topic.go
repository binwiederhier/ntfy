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
	ID                 string
	subscribers        map[int]*topicSubscriber
	lastVisitor        *visitor
	lastVisitorExpires time.Time
	mu                 sync.Mutex
}

type topicSubscriber struct {
	subscriber subscriber
	visitor    *visitor // User ID associated with this subscription, may be empty
	cancel     func()
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(v *visitor, msg *message) error

// newTopic creates a new topic
func newTopic(id string) *topic {
	return &topic{
		ID:          id,
		subscribers: make(map[int]*topicSubscriber),
	}
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber, visitor *visitor, cancel func()) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = &topicSubscriber{
		visitor:    visitor, // May be empty
		subscriber: s,
		cancel:     cancel,
	}
	return subscriberID
}

func (t *topic) Stale() bool {
	return t.getBillee() == nil
}

func (t *topic) getBillee() *visitor {
	for _, this_subscriber := range t.subscribers {
		return this_subscriber.visitor
	}
	if t.lastVisitor != nil && t.lastVisitorExpires.After(time.Now()) {
		t.lastVisitor = nil
	}
	return t.lastVisitor

}

// Unsubscribe removes the subscription from the list of subscribers
func (t *topic) Unsubscribe(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.subscribers) == 1 {
		t.lastVisitor = t.subscribers[id].visitor
		t.lastVisitorExpires = time.Now().Add(subscriberBilledValidity)
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
			logvm(v, m).Tag(tagPublish).Debug("Forwarding to %d subscriber(s)", len(subscribers))
			for _, s := range subscribers {
				// We call the subscriber functions in their own Go routines because they are blocking, and
				// we don't want individual slow subscribers to be able to block others.
				go func(s subscriber) {
					if err := s(v, m); err != nil {
						logvm(v, m).Tag(tagPublish).Err(err).Warn("Error forwarding to subscriber")
					}
				}(s.subscriber)
			}
		} else {
			logvm(v, m).Tag(tagPublish).Trace("No stream or WebSocket subscribers, not forwarding")
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

// CancelSubscribers calls the cancel function for all subscribers, forcing
func (t *topic) CancelSubscribers(exceptUserID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.subscribers {
		if s.visitor.MaybeUserID() != exceptUserID {
			// TODO: Shouldn't this log the IP for anonymous visitors? It was s.userID before my change.
			log.Tag(tagSubscribe).Field("topic", t.ID).Debug("Canceling subscriber %s", s.visitor.MaybeUserID())
			s.cancel()
		}
	}
}

// subscribersCopy returns a shallow copy of the subscribers map
func (t *topic) subscribersCopy() map[int]*topicSubscriber {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscribers := make(map[int]*topicSubscriber)
	for k, sub := range t.subscribers {
		subscribers[k] = &topicSubscriber{
			visitor:    sub.visitor,
			subscriber: sub.subscriber,
			cancel:     sub.cancel,
		}
	}
	return subscribers
}
