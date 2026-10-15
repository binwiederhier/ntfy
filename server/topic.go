package server

import (
	"math/rand"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	// topicExpungeAfter defines how long a topic is active before it is removed from memory.
	// This must be larger than matrixRejectPushKeyForUnifiedPushTopicWithoutRateVisitorAfter to give
	// time for more requests to come in, so that we can send a {"rejected":["<pushkey>"]} response back.
	topicExpungeAfter = 16 * time.Hour

	// topicDeliveredRetain is how long a topic at least remembers that a message went out to its
	// subscribers, so the tail (see PublishMissed) does not send it again. It must cover the write
	// delay plus how long the tail waits for a row committed out of order (tailHoleTimeout). Entries
	// are dropped by the manager's next pass over the topics (see ExpireDelivered), so they may live longer.
	topicDeliveredRetain = 2 * time.Minute
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID          string
	subscribers map[int]*topicSubscriber
	rateVisitor *visitor
	lastAccess  time.Time
	delivered   map[string]time.Time // When recent messages went out to the subscribers, by message ID
	mu          sync.RWMutex
}

type topicSubscriber struct {
	userID       string // User ID associated with this subscription, may be empty
	subscriber   subscriber
	cancel       func()
	subscribedAt time.Time     // When it subscribed; what went out before, it missed
	sent         chan struct{} // Closed when the last batch of sends to this subscriber is done; the next one waits for it
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(v *visitor, msg *model.Message) error

// newTopic creates a new topic
func newTopic(id string) *topic {
	return &topic{
		ID:          id,
		subscribers: make(map[int]*topicSubscriber),
		lastAccess:  time.Now(),
		delivered:   make(map[string]time.Time),
	}
}

// Subscribe subscribes to this topic
func (t *topic) Subscribe(s subscriber, userID string, cancel func()) (subscriberID int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := 0; i < 5; i++ { // Best effort retry
		subscriberID = rand.Int()
		_, exists := t.subscribers[subscriberID]
		if !exists {
			break
		}
	}
	t.subscribers[subscriberID] = &topicSubscriber{
		userID:       userID, // May be empty
		subscriber:   s,
		cancel:       cancel,
		subscribedAt: time.Now(),
	}
	t.lastAccess = time.Now()
	return subscriberID
}

// DeliveredAt returns when the message went out to this topic's subscribers, if it did recently
func (t *topic) DeliveredAt(id string) (time.Time, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	at, ok := t.delivered[id]
	return at, ok
}

// ExpireDelivered forgets deliveries older than topicDeliveredRetain; the manager calls it on
// every pass over the topics
func (t *topic) ExpireDelivered() {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-topicDeliveredRetain)
	for id, at := range t.delivered {
		if at.Before(cutoff) {
			delete(t.delivered, id)
		}
	}
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

// Publish sends a live message to the subscribers that have not had it (see deliver), without
// waiting for the sends
func (t *topic) Publish(v *visitor, m *model.Message) error {
	if sends := t.deliver(v, []*model.Message{m}); sends > 0 {
		logvm(v, m).Tag(tagPublish).Debug("Forwarding to %d subscriber(s)", sends)
	} else {
		logvm(v, m).Tag(tagPublish).Trace("No subscribers that have not had it, not forwarding")
	}
	t.Keepalive()
	return nil
}

// PublishMissed sends messages the tail read from the cache to the subscribers that have not had
// them (see deliver), without waiting for the sends. It returns how many sends it started.
func (t *topic) PublishMissed(messages []*model.Message) int {
	return t.deliver(nil, messages) // No visitor: the tail is not a request
}

// deliver sends each message to the subscribers that have not had it: all of them if it never
// went out here, or those that subscribed after it went out (they were reconnecting and their
// read missed it, or the tail was faster than the fan-out). It records each as gone out now, and
// picks the subscribers under the same lock, so a subscriber either gets a message or subscribed
// after. Each subscriber's messages go out from one Go routine, in order, after its earlier ones:
// a reconnect asks for what came after the last message the client got, so none may overtake.
func (t *topic) deliver(v *visitor, messages []*model.Message) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	pending := make(map[*topicSubscriber][]*model.Message)
	// Pick the subscribers that have not had each message, and record it as gone out to all of them now
	for _, m := range messages {
		at, ok := t.delivered[m.ID]
		for _, s := range t.subscribers {
			if !ok || !s.subscribedAt.Before(at) {
				pending[s] = append(pending[s], m)
			}
		}
		t.delivered[m.ID] = now
	}
	sends := 0
	// Send each subscriber its messages, chained after its earlier sends so they stay in order
	for s, subscriberMessages := range pending {
		sends += len(subscriberMessages)
		previous, done := s.sent, make(chan struct{})
		s.sent = done
		// In their own Go routines because sends block, and a slow subscriber must not hold up others
		go func(s subscriber, subscriberMessages []*model.Message) {
			defer close(done)
			if previous != nil {
				<-previous
			}
			for _, m := range subscriberMessages {
				if err := s(v, m); err != nil {
					ev := log.Tag(tagPublish).With(m)
					if v != nil {
						ev = logvm(v, m).Tag(tagPublish)
					}
					ev.Err(err).Warn("Error forwarding to subscriber")
					return // The connection is broken, so the rest would fail too
				}
			}
		}(s.subscriber, subscriberMessages)
	}
	return sends
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

// SubscribersCount returns the number of subscribers currently attached to this topic
func (t *topic) SubscribersCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subscribers)
}

// CancelAllSubscribers calls the cancel function of every subscriber, closing their connections
func (t *topic) CancelAllSubscribers() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, s := range t.subscribers {
		s.cancel()
	}
}

// CancelSubscribersExceptUser calls the cancel function for all subscribers, forcing
func (t *topic) CancelSubscribersExceptUser(exceptUserID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.subscribers {
		if s.userID != exceptUserID {
			t.cancelUserSubscriber(s)
		}
	}
}

// CancelSubscriberUser kills the subscriber with the given user ID
func (t *topic) CancelSubscriberUser(userID string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, s := range t.subscribers {
		if s.userID == userID {
			t.cancelUserSubscriber(s)
			return
		}
	}
}

func (t *topic) cancelUserSubscriber(s *topicSubscriber) {
	log.
		Tag(tagSubscribe).
		With(t).
		Fields(log.Context{
			"user_id": s.userID,
		}).
		Debug("Canceling subscriber with user ID %s", s.userID)
	s.cancel()
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
