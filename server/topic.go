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

	// recentMargin is added to the cache batch timeout and the fan-out linger to size a topic's
	// recent-message window (see topic.recent), covering replica lag
	recentMargin = time.Second
)

// topic represents a channel to which subscribers can subscribe, and publishers
// can publish a message
type topic struct {
	ID           string
	subscribers  map[int]*topicSubscriber
	rateVisitor  *visitor
	lastAccess   time.Time
	recent       []*model.Message // Messages published in the last recentWindow, oldest first (see RecentAfter)
	recentWindow time.Duration
	mu           sync.RWMutex
}

type topicSubscriber struct {
	userID     string // User ID associated with this subscription, may be empty
	subscriber subscriber
	cancel     func()
}

// subscriber is a function that is called for every new message on a topic
type subscriber func(v *visitor, msg *model.Message) error

// newTopic creates a new topic
func newTopic(id string, recentWindow time.Duration) *topic {
	return &topic{
		ID:           id,
		subscribers:  make(map[int]*topicSubscriber),
		lastAccess:   time.Now(),
		recentWindow: recentWindow,
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
func (t *topic) Publish(v *visitor, m *model.Message) error {
	// Remembering the message and taking the subscriber snapshot happen under one lock, so a
	// subscriber is either in the snapshot (and gets the message live) or subscribed after the
	// message was remembered (and gets it from its replay), never both
	subscribers := t.rememberAndSnapshot(m)
	go func() {
		// Sending out the messages does not hold the lock, so a slow subscriber cannot block it
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

// rememberAndSnapshot keeps the message in the topic's recent window, pruning what has aged out
// of it, and returns the subscribers to deliver it to. Only messages that are going to be stored
// belong in the window: it stands in for the database while the row is in flight, never for a
// message that will not have a row (cache: no, or a server without a cache).
func (t *topic) rememberAndSnapshot(m *model.Message) map[int]*topicSubscriber {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.recentWindow > 0 && m.Expires > 0 {
		t.pruneRecentNoLock()
		t.recent = append(t.recent, m)
	}
	return t.subscribersCopyNoLock()
}

// PruneRecent drops what has aged out of the recent window. Publishing prunes as it goes, but a
// topic that goes quiet would otherwise hold its last messages until the topic itself is expunged,
// which across every topic on a busy server adds up; the manager sweeps them.
func (t *topic) PruneRecent() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneRecentNoLock()
}

// pruneRecentNoLock is PruneRecent for callers already holding t.mu. Message times are whole
// seconds, so a message stays up to a second longer than the window; erring that way is fine.
func (t *topic) pruneRecentNoLock() {
	cutoff := time.Now().Add(-t.recentWindow).Unix()
	kept := 0
	for kept < len(t.recent) && t.recent[kept].Time < cutoff {
		kept++
	}
	if kept == len(t.recent) {
		t.recent = nil // Release the backing array, not just the entries
		return
	}
	t.recent = t.recent[kept:]
}

// subscribersCopy returns a shallow copy of the subscribers, so delivery never holds the lock
func (t *topic) subscribersCopy() map[int]*topicSubscriber {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.subscribersCopyNoLock()
}

// subscribersCopyNoLock is subscribersCopy for callers already holding t.mu
func (t *topic) subscribersCopyNoLock() map[int]*topicSubscriber {
	subscribers := make(map[int]*topicSubscriber, len(t.subscribers))
	for k, sub := range t.subscribers {
		subscribers[k] = &topicSubscriber{
			userID:     sub.userID,
			subscriber: sub.subscriber,
			cancel:     sub.cancel,
		}
	}
	return subscribers
}

// RecentAfter returns the recently published messages a reconnecting subscriber may not find in
// the database yet: a message is pushed live before its row is written (it sits in a cache write
// batch for up to cache-batch-timeout, here or on the node that accepted it), so a replay from
// the database alone has a hole exactly as wide as that batch. Every message passes through
// Publish on every node moments after it was accepted, so the topic keeps the last recentWindow
// of them. If the marker id is in the window the messages after it are returned, otherwise those
// published at or after since.
func (t *topic) RecentAfter(markerID string, since int64) []*model.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now().Unix()
	from := 0
	for i, m := range t.recent {
		if m.ID == markerID {
			from, since = i+1, 0
			break
		}
	}
	var out []*model.Message
	for _, m := range t.recent[from:] {
		if m.Time >= since && m.Expires > now { // Expired here means expired in the database too
			out = append(out, m)
		}
	}
	return out
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
