package server

import (
	"heckel.io/ntfy/log"
	"math/rand"
	"sync"
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID          string
	subscribers map[int]*topicSubscriber
	rateVisitor *visitor
	mu          sync.RWMutex
}

type topicSubscriber struct {
	userID     string // User ID associated with this subscription, may be empty
	subscriber subscriber
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
func (t *topic) Subscribe(s subscriber, userID string, cancel func()) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscriberID := rand.Int()
	t.subscribers[subscriberID] = &topicSubscriber{
		userID:     userID, // May be empty
		subscriber: s,
		cancel:     cancel,
	}
	return subscriberID
}

func (t *topic) Stale() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rateVisitor != nil && !t.rateVisitor.Stale() {
		return false
	}
	return len(t.subscribers) == 0
}

func (t *topic) SetRateVisitor(v *visitor) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rateVisitor = v
}

func (t *topic) RateVisitor() *visitor {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rateVisitor != nil && t.rateVisitor.Stale() {
		t.rateVisitor = nil
	}
	return t.rateVisitor
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
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subscribers)
}

// CancelSubscribers calls the cancel function for all subscribers, forcing
func (t *topic) CancelSubscribers(exceptUserID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.subscribers {
		if s.userID != exceptUserID {
			log.
				Tag(tagSubscribe).
				With(t).
				Fields(log.Context{
					"user_id": s.userID,
				}).
				Debug("Canceling subscriber %s", s.userID)
			s.cancel()
		}
	}
}

func (t *topic) Context() log.Context {
	t.mu.RLock()
	defer t.mu.RUnlock()
	fields := map[string]any{
		"topic":             t.ID,
		"topic_subscribers": len(t.subscribers),
	}
	if t.rateVisitor != nil {
		fields["topic_rate_visitor_ip"] = t.rateVisitor.IP().String()
		fields["topic_rate_visitor_user_id"] = t.rateVisitor.MaybeUserID()
	}
	return fields
}

// subscribersCopy returns a shallow copy of the subscribers map
func (t *topic) subscribersCopy() map[int]*topicSubscriber {
	t.mu.Lock()
	defer t.mu.Unlock()
	subscribers := make(map[int]*topicSubscriber)
	for k, sub := range t.subscribers {
		subscribers[k] = &topicSubscriber{
			userID:     sub.userID,
			subscriber: sub.subscriber,
			cancel:     sub.cancel,
		}
	}
	return subscribers
}
