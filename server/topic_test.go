package server

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
)

func TestTopic_CancelSubscribersExceptUser(t *testing.T) {
	subFn := func(v *visitor, msg *model.Message) error {
		return nil
	}
	canceled1 := atomic.Bool{}
	cancelFn1 := func() {
		canceled1.Store(true)
	}
	canceled2 := atomic.Bool{}
	cancelFn2 := func() {
		canceled2.Store(true)
	}
	to := newTopic("mytopic", time.Minute)
	to.Subscribe(subFn, "", cancelFn1)
	to.Subscribe(subFn, "u_phil", cancelFn2)

	to.CancelSubscribersExceptUser("u_phil")
	require.True(t, canceled1.Load())
	require.False(t, canceled2.Load())
}

func TestTopic_CancelSubscribersUser(t *testing.T) {
	t.Parallel()

	subFn := func(v *visitor, msg *model.Message) error {
		return nil
	}
	canceled1 := atomic.Bool{}
	cancelFn1 := func() {
		canceled1.Store(true)
	}
	canceled2 := atomic.Bool{}
	cancelFn2 := func() {
		canceled2.Store(true)
	}
	to := newTopic("mytopic", time.Minute)
	to.Subscribe(subFn, "u_another", cancelFn1)
	to.Subscribe(subFn, "u_phil", cancelFn2)

	to.CancelSubscriberUser("u_phil")
	require.False(t, canceled1.Load())
	require.True(t, canceled2.Load())
}

func TestTopic_Keepalive(t *testing.T) {
	t.Parallel()

	to := newTopic("mytopic", time.Minute)
	to.lastAccess = time.Now().Add(-1 * time.Hour)
	to.Keepalive()
	require.True(t, to.LastAccess().Unix() >= time.Now().Unix()-2)
	require.True(t, to.LastAccess().Unix() <= time.Now().Unix()+2)
}

func TestTopic_Subscribe_DuplicateID(t *testing.T) {
	t.Parallel()
	to := newTopic("mytopic", time.Minute)

	//lint:ignore SA1019 Fix random seed to force same number generation
	rand.Seed(1)
	a := rand.Int()
	to.subscribers[a] = &topicSubscriber{
		userID:     "a",
		subscriber: nil,
		cancel:     func() {},
	}

	subFn := func(v *visitor, msg *model.Message) error {
		return nil
	}

	//lint:ignore SA1019 Force rand.Int to generate the same id once more
	rand.Seed(1)
	id := to.Subscribe(subFn, "b", func() {})
	res := to.subscribers[id]

	require.NotEqual(t, id, a)
	require.Equal(t, "b", res.userID, "b")
}

func TestTopic_RecentAfter(t *testing.T) {
	// The window is what a reconnect can read back before the rows are written: after the marker
	// when the marker is in it, by time otherwise, and nothing older than the window
	topic := newTopic("mytopic", time.Minute)
	old := model.NewDefaultMessage("mytopic", "aged out")
	old.Time = time.Now().Add(-2 * time.Minute).Unix()
	a, b, c := model.NewDefaultMessage("mytopic", "a"), model.NewDefaultMessage("mytopic", "b"), model.NewDefaultMessage("mytopic", "c")
	for _, m := range []*model.Message{old, a, b, c} {
		m.Expires = m.Time + 3600 // Cached, so it is remembered
		require.Nil(t, topic.Publish(nil, m))
	}
	ids := func(messages []*model.Message) []string {
		out := make([]string, 0, len(messages))
		for _, m := range messages {
			out = append(out, m.ID)
		}
		return out
	}
	require.Equal(t, []string{c.ID}, ids(topic.RecentAfter(b.ID, 0)))                  // After the marker
	require.Equal(t, []string{a.ID, b.ID, c.ID}, ids(topic.RecentAfter("unknown", 0))) // By time, and old is gone
	require.Empty(t, topic.RecentAfter("unknown", time.Now().Add(time.Minute).Unix()))
}
