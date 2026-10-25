package limit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The topic table: who a topic bills to (the UnifiedPush rate visitor), when it was last
// subscribed to and published on, shared by every node on the store and kept across restarts
// (see docs/10-cluster.md, section 11).

func TestService_Topics_RecordAndGet(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		_, err := s.Topic("mytopic")
		require.ErrorIs(t, err, ErrTopicNotFound)
		require.Nil(t, s.RecordTopicPublish("mytopic", "ip:1.2.3.4"))
		clock.advance(time.Minute)
		require.Nil(t, s.RecordTopicSubscriber("mytopic", "ip:5.6.7.8"))
		info, err := s.Topic("mytopic")
		require.Nil(t, err)
		require.Equal(t, "ip:1.2.3.4", info.LastPublisherKey)
		require.Equal(t, "ip:5.6.7.8", info.LastSubscriberKey)
		require.Equal(t, info.CreatedAt.Add(time.Minute), info.LastSubscribedAt)
		require.Equal(t, "", info.RateVisitorKey)
		require.Equal(t, info.LastSubscribedAt, info.LastActivity())
	})
}

func TestService_Topics_RateVisitor_SetReplaceAndExpire(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		_, _, err := s.TopicRateVisitor("up123456789012")
		require.ErrorIs(t, err, ErrTopicNotFound)
		require.Nil(t, s.SetTopicRateVisitor("up123456789012", "ip:1.2.3.4", ""))
		key, uid, err := s.TopicRateVisitor("up123456789012")
		require.Nil(t, err)
		require.Equal(t, "ip:1.2.3.4", key)
		require.Equal(t, "", uid)

		// A newer subscriber takes over
		require.Nil(t, s.SetTopicRateVisitor("up123456789012", "user:u_abc", "u_abc"))
		key, uid, err = s.TopicRateVisitor("up123456789012")
		require.Nil(t, err)
		require.Equal(t, "user:u_abc", key)
		require.Equal(t, "u_abc", uid)

		// The assignment expires when the subscriber has not been seen for topicRateVisitorTTL; a
		// keepalive of that subscriber (RecordTopicSubscriber with its key) extends it, one of
		// another subscriber does not
		clock.advance(topicRateVisitorTTL - time.Hour)
		require.Nil(t, s.RecordTopicSubscriber("up123456789012", "user:u_abc"))
		clock.advance(2 * time.Hour)
		key, _, err = s.TopicRateVisitor("up123456789012")
		require.Nil(t, err)
		require.Equal(t, "user:u_abc", key)
		clock.advance(topicRateVisitorTTL - time.Hour)
		require.Nil(t, s.RecordTopicSubscriber("up123456789012", "ip:9.9.9.9"))
		clock.advance(2 * time.Hour)
		_, _, err = s.TopicRateVisitor("up123456789012")
		require.ErrorIs(t, err, ErrTopicNotFound)
		info, err := s.Topic("up123456789012")
		require.Nil(t, err)
		require.Equal(t, "user:u_abc", info.RateVisitorKey) // Still recorded, just stale
	})
}

func TestService_Topics_SharedAcrossServices(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		a := newPersistentTestService(t, newStore(), clock)
		b := newPersistentTestService(t, newStore(), clock)
		require.Nil(t, a.SetTopicRateVisitor("up123456789012", "ip:1.2.3.4", ""))
		key, _, err := b.TopicRateVisitor("up123456789012")
		require.Nil(t, err)
		require.Equal(t, "ip:1.2.3.4", key)
	})
}

func TestService_Topics_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, newStore func() *Store) {
		clock := &testClock{t: time.Now()}
		s := newPersistentTestService(t, newStore(), clock)
		require.Nil(t, s.RecordTopicPublish("oldtopic", "ip:1.2.3.4"))
		clock.advance(topicRetention + time.Hour)
		require.Nil(t, s.RecordTopicPublish("newtopic", "ip:1.2.3.4"))
		require.Nil(t, s.PruneTopics())
		_, err := s.Topic("oldtopic")
		require.ErrorIs(t, err, ErrTopicNotFound)
		_, err = s.Topic("newtopic")
		require.Nil(t, err)
	})
}

func TestService_Topics_WithoutStore(t *testing.T) {
	s, _ := newTestServiceWithClock(t)
	require.False(t, s.Persistent())
	require.Nil(t, s.RecordTopicPublish("mytopic", "ip:1.2.3.4")) // Nothing to record to; not an error
	_, err := s.Topic("mytopic")
	require.ErrorIs(t, err, ErrTopicNotFound)
}
