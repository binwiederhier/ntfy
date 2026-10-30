package limit

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	// topicRateVisitorTTL is how long after the topic's last sign of a subscriber (the rate
	// visitor's subscription, or any subscriber's, see RecordTopicSubscriber) the assignment
	// remains valid, as on one node (see visitorExpungeAfter in package server): an open stream
	// anywhere keeps it alive, since its node records it every few minutes.
	topicRateVisitorTTL = 24 * time.Hour

	// topicRetention is how long rows without any recorded activity are kept before PruneTopics
	// deletes them; far longer than any decision window that reads the table
	topicRetention = 30 * 24 * time.Hour
)

// topicStatement names one of the topic table's write statements (see topicQueries.query)
type topicStatement int

// The topic table's write statements
const (
	topicUpsertSubscriber topicStatement = iota
	topicUpsertPublish
	topicUpsertRateVisitor
	topicInsert
	topicPrune
)

// ErrTopicNotFound means the topic has no row (it was never recorded on any node), or there
// is no store to record to
var ErrTopicNotFound = errors.New("topic not recorded")

// TopicInfo is a topic's durable state: who it bills to (the UnifiedPush rate visitor), when it
// was created, last subscribed to and last published on, and by whom. The topic table moves
// these facts out of the in-memory topic, so any node answers the same and a topic without
// subscribers can be evicted from memory after minutes (see docs/10-cluster.md, section 11).
type TopicInfo struct {
	ID                string
	CreatedAt         time.Time
	LastSubscribedAt  time.Time
	LastSubscriberKey string
	LastPublishedAt   time.Time
	LastPublisherKey  string
	RateVisitorKey    string    // Empty if no rate visitor was ever assigned
	RateVisitorUID    string    // The rate visitor's user ID, empty if anonymous
	RateVisitorSeenAt time.Time // When the rate visitor was assigned
}

// RateVisitorLive reports whether the topic's rate visitor assignment is still valid at now:
// there is one, and the topic showed a subscriber within topicRateVisitorTTL
func (i *TopicInfo) RateVisitorLive(now time.Time) bool {
	return i.RateVisitorKey != "" && now.Sub(i.RateVisitorSeen()) <= topicRateVisitorTTL
}

// RateVisitorSeen returns when the rate visitor was last vouched for: its assignment, or the
// topic's last subscriber, whoever it was
func (i *TopicInfo) RateVisitorSeen() time.Time {
	return latest(i.RateVisitorSeenAt, i.LastSubscribedAt)
}

// LastSubscriberActivity returns when the topic last showed a sign of a subscriber: its last
// subscription or rate visitor assignment, or its creation if it never had one. Publishes do not
// count: a publisher pushing to a topic nobody reads is exactly what this is meant to catch.
func (i *TopicInfo) LastSubscriberActivity() time.Time {
	return latest(i.CreatedAt, i.LastSubscribedAt, i.RateVisitorSeenAt)
}

// Topic returns everything recorded about the topic, or ErrTopicNotFound
func (s *Service) Topic(id string) (*TopicInfo, error) {
	store := s.store.Load()
	if store == nil {
		return nil, ErrTopicNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	var createdAt, lastSubscribedAt, lastPublishedAt, rateVisitorSeenAt int64
	info := &TopicInfo{}
	row := store.db.QueryRowContext(ctx, store.queries.topics.selectTopic, id)
	err := row.Scan(&info.ID, &createdAt, &lastSubscribedAt, &info.LastSubscriberKey, &lastPublishedAt, &info.LastPublisherKey, &info.RateVisitorKey, &info.RateVisitorUID, &rateVisitorSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTopicNotFound
	} else if err != nil {
		return nil, err
	}
	info.CreatedAt = unixOrZero(createdAt)
	info.LastSubscribedAt = unixOrZero(lastSubscribedAt)
	info.LastPublishedAt = unixOrZero(lastPublishedAt)
	info.RateVisitorSeenAt = unixOrZero(rateVisitorSeenAt)
	return info, nil
}

// RecordTopicSubscriber records that the visitor key is (still) subscribed to the topic, which
// also keeps the topic's rate visitor live (see RateVisitorLive). The caller throttles calls per
// topic.
func (s *Service) RecordTopicSubscriber(id, key string) error {
	now := s.now().Unix()
	return s.execTopic(topicUpsertSubscriber, id, now, now, key)
}

// RecordTopicPublish records a successful publish to the topic, and who sent it. The caller
// throttles calls per topic.
func (s *Service) RecordTopicPublish(id, key string) error {
	now := s.now().Unix()
	return s.execTopic(topicUpsertPublish, id, now, now, key)
}

// NoteTopic records that the topic exists (its creation time), if it has no row yet; it changes
// nothing else. A publish that is refused (no subscriber) notes the topic without refreshing it.
func (s *Service) NoteTopic(id string) error {
	return s.execTopic(topicInsert, id, s.now().Unix())
}

// SetTopicRateVisitor assigns the visitor the topic bills to (the UnifiedPush subscriber); an
// existing assignment is replaced
func (s *Service) SetTopicRateVisitor(id, key, userID string) error {
	now := s.now().Unix()
	return s.execTopic(topicUpsertRateVisitor, id, now, key, userID, now)
}

// TopicRateVisitor returns the topic's rate visitor if the assignment is still live (see
// RateVisitorLive), or ErrTopicNotFound
func (s *Service) TopicRateVisitor(id string) (key string, userID string, err error) {
	info, err := s.Topic(id)
	if err != nil {
		return "", "", err
	}
	if !info.RateVisitorLive(s.now()) {
		return "", "", ErrTopicNotFound
	}
	return info.RateVisitorKey, info.RateVisitorUID, nil
}

// PruneTopics deletes the rows of topics idle for longer than topicRetention; in a cluster, one
// node (the leader) calls it
func (s *Service) PruneTopics() error {
	return s.execTopic(topicPrune, s.now().Add(-topicRetention).Unix())
}

// execTopic runs one topic statement, bounded by flushTimeout; without a store there is nothing
// to record to, which is not an error
func (s *Service) execTopic(statement topicStatement, args ...any) error {
	store := s.store.Load()
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	_, err := store.db.ExecContext(ctx, store.queries.topics.query(statement), args...)
	return err
}

func latest(times ...time.Time) time.Time {
	var t time.Time
	for _, u := range times {
		if u.After(t) {
			t = u
		}
	}
	return t
}

func unixOrZero(seconds int64) time.Time {
	if seconds == 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}
