package server

import (
	"math/rand"
	"sync"
	"time"

	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
)

const (
	// topicExpungeAfter defines how long a topic is active before it is removed from memory.
	// This must be larger than matrixRejectPushKeyForUnifiedPushTopicWithoutRateVisitorAfter to give
	// time for more requests to come in, so that we can send a {"rejected":["<pushkey>"]} response back.
	topicExpungeAfter = 16 * time.Hour
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID          string
	subscribers map[int]*topicSubscriber
	rateVisitor *visitor
	lastAccess  time.Time
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
		lastAccess:  time.Now(),
	}
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber, userID string, cancel func()) int {
	max_retries := 5
	retries := 1
	t.mu.Lock()
	defer t.mu.Unlock()

	subscriberID := rand.Int()
	// simple check for existing id in maps
	for {
		_, ok := t.subscribers[subscriberID]
		if ok && retries <= max_retries {
			subscriberID = rand.Int()
			retries++
		} else {
			break
		}
	}

	t.subscribers[subscriberID] = &topicSubscriber{
		userID:     userID, // May be empty
		subscriber: s,
		cancel:     cancel,
	}
	t.lastAccess = time.Now()
	return subscriberID
}

func (t *topic) Stale() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rateVisitor != nil && !t.rateVisitor.Stale() {
		return false
	}
	return len(t.subscribers) == 0 && time.Since(t.lastAccess) > topicExpungeAfter
}

func (t *topic) LastAccess() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastAccess
}

func (t *topic) SetRateVisitor(v *visitor) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rateVisitor = v
	t.lastAccess = time.Now()
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
		t.Keepalive()
	}()
	return nil
}

// Stats returns the number of subscribers and last access to this topic
func (t *topic) Stats() (int, time.Time) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subscribers), t.lastAccess
}

// Keepalive sets the last access time and ensures that Stale does not return true
func (t *topic) Keepalive() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastAccess = time.Now()
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
		"topic_last_access": util.FormatTime(t.lastAccess),
	}
	if t.rateVisitor != nil {
		for k, v := range t.rateVisitor.Context() {
			fields["topic_rate_"+k] = v
		}
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
