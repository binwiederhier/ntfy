package server

import (
	"fmt"
	"math/rand"
	"sync"
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
	to := newTopic("mytopic")
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
	to := newTopic("mytopic")
	to.Subscribe(subFn, "u_another", cancelFn1)
	to.Subscribe(subFn, "u_phil", cancelFn2)

	to.CancelSubscriberUser("u_phil")
	require.False(t, canceled1.Load())
	require.True(t, canceled2.Load())
}

func TestTopic_Keepalive(t *testing.T) {
	t.Parallel()

	to := newTopic("mytopic")
	to.lastAccess = time.Now().Add(-1 * time.Hour)
	to.Keepalive()
	require.True(t, to.LastAccess().Unix() >= time.Now().Unix()-2)
	require.True(t, to.LastAccess().Unix() <= time.Now().Unix()+2)
}

func TestTopic_Subscribe_DuplicateID(t *testing.T) {
	t.Parallel()
	to := newTopic("mytopic")

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

func TestTopic_PublishMissed_OnlyToSubscribersThatMissedIt(t *testing.T) {
	to := newTopic("mytopic")
	var early, late atomic.Int32
	to.Subscribe(countingSubscriber(&early), "", func() {})
	m := model.NewDefaultMessage("mytopic", "hi")
	require.Nil(t, to.Publish(nil, m))
	waitFor(t, func() bool { return early.Load() == 1 })

	to.Subscribe(countingSubscriber(&late), "", func() {}) // Joined after the live delivery
	to.PublishMissed([]*model.Message{m})
	waitFor(t, func() bool { return late.Load() == 1 })
	to.PublishMissed([]*model.Message{m}) // Caught up already
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(1), early.Load())
	require.Equal(t, int32(1), late.Load())
}

func TestTopic_Publish_SkipsWhatWasDeliveredAlready(t *testing.T) {
	// The tail found the message first, then the fan-out from the peer arrived
	to := newTopic("mytopic")
	var count atomic.Int32
	to.Subscribe(countingSubscriber(&count), "", func() {})
	m := model.NewDefaultMessage("mytopic", "hi")
	to.PublishMissed([]*model.Message{m})
	waitFor(t, func() bool { return count.Load() == 1 })
	require.Nil(t, to.Publish(nil, m))
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(1), count.Load())
}

func countingSubscriber(count *atomic.Int32) subscriber {
	return func(_ *visitor, _ *model.Message) error {
		count.Add(1)
		return nil
	}
}

func TestTopic_PublishMissed_InWriteOrder(t *testing.T) {
	// A reconnect asks for what came after the last message the client got, so the client must
	// get the tail's messages in the order they were written
	to := newTopic("mytopic")
	var mu sync.Mutex
	received := make([]string, 0)
	to.Subscribe(func(_ *visitor, m *model.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, m.Message)
		return nil
	}, "", func() {})
	messages := make([]*model.Message, 0)
	expected := make([]string, 0)
	for i := 0; i < 100; i++ {
		messages = append(messages, model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i)))
		expected = append(expected, fmt.Sprintf("message %d", i))
	}
	require.Equal(t, 100, to.PublishMissed(messages))
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 100
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, expected, received)
}
