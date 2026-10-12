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
	// are dropped by the manager's next pass over the topics (see Stale), so they may live longer.
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
	userID     string // User ID associated with this subscription, may be empty
	subscriber subscriber
	cancel     func()
	since      time.Time     // When it subscribed; what went out before, it missed
	missedDone chan struct{} // Closed when the tail's last batch for this subscriber is sent; the next one waits for it
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
		userID:     userID, // May be empty
		subscriber: s,
		cancel:     cancel,
		since:      time.Now(),
	}
	t.lastAccess = time.Now()
	return subscriberID
}

func (t *topic) Stale() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.expireDeliveredNoLock()
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

// Publish asynchronously publishes to all subscribers, unless the message went out already
func (t *topic) Publish(v *visitor, m *model.Message) error {
	go func() {
		// We want to lock the topic as short as possible, so we make a shallow copy of the
		// subscribers map here. Actually sending out the messages then doesn't have to lock.
		subscribers, ok := t.markDelivered(m)
		if !ok {
			logvm(v, m).Tag(tagPublish).Trace("Delivered already, not forwarding")
			return
		}
		t.send(v, m, subscribers)
		t.Keepalive()
	}()
	return nil
}

// PublishMissed asynchronously publishes messages the tail read from the cache to the
// subscribers that did not get them: all of them if a message never went out here (its fan-out
// was lost), or those that subscribed after it went out (they were reconnecting and their read
// missed it). It returns how many sends it made.
func (t *topic) PublishMissed(messages []*model.Message) int {
	t.mu.Lock()
	missed := make(map[*topicSubscriber][]*model.Message)
	for _, m := range messages {
		at, ok := t.delivered[m.ID]
		for _, s := range t.subscribers {
			if !ok || !s.since.Before(at) {
				missed[s] = append(missed[s], m)
			}
		}
		t.rememberDeliveredNoLock(m.ID)
	}
	sends := 0
	for s, subscriberMessages := range missed {
		sends += len(subscriberMessages)
		previous, done := s.missedDone, make(chan struct{})
		s.missedDone = done
		// One Go routine per subscriber, sending in write order after the previous batch: a reconnect
		// asks for what came after the last message the client got, so no later one may overtake
		go func(s subscriber, subscriberMessages []*model.Message) {
			defer close(done)
			if previous != nil {
				<-previous
			}
			for _, m := range subscriberMessages {
				if err := s(nil, m); err != nil { // No visitor: the tail is not a request
					log.Tag(tagPublish).With(m).Err(err).Warn("Error forwarding missed message to subscriber")
					return // The connection is broken, so the rest would fail too
				}
			}
		}(s.subscriber, subscriberMessages)
	}
	t.mu.Unlock()
	return sends
}

func (t *topic) send(v *visitor, m *model.Message, subscribers []subscriber) {
	if len(subscribers) == 0 {
		logvm(v, m).Tag(tagPublish).Trace("No stream or WebSocket subscribers, not forwarding")
		return
	}
	logvm(v, m).Tag(tagPublish).Debug("Forwarding to %d subscriber(s)", len(subscribers))
	for _, s := range subscribers {
		// We call the subscriber functions in their own Go routines because they are blocking, and
		// we don't want individual slow subscribers to be able to block others.
		go func(s subscriber) {
			if err := s(v, m); err != nil {
				logvm(v, m).Tag(tagPublish).Err(err).Warn("Error forwarding to subscriber")
			}
		}(s)
	}
}

// markDelivered records that the message goes out now, and returns who it goes out to; false if
// it went out already. Both under one lock, so a subscriber either gets it or subscribed after.
func (t *topic) markDelivered(m *model.Message) ([]subscriber, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.delivered[m.ID]; ok {
		return nil, false
	}
	t.rememberDeliveredNoLock(m.ID)
	subscribers := make([]subscriber, 0, len(t.subscribers))
	for _, s := range t.subscribers {
		subscribers = append(subscribers, s.subscriber)
	}
	return subscribers, true
}

// rememberDeliveredNoLock records that the message went out now; t.mu must be held
func (t *topic) rememberDeliveredNoLock(id string) {
	t.delivered[id] = time.Now()
}

// expireDeliveredNoLock forgets deliveries older than topicDeliveredRetain; t.mu must be held
func (t *topic) expireDeliveredNoLock() {
	cutoff := time.Now().Add(-topicDeliveredRetain)
	for id, at := range t.delivered {
		if at.Before(cutoff) {
			delete(t.delivered, id)
		}
	}
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
