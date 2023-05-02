package server

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTopic_CancelSubscribers(t *testing.T) {
	t.Parallel()

	subFn := func(v *visitor, msg *message) error {
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

	to.CancelSubscribers("u_phil")
	require.True(t, canceled1.Load())
	require.False(t, canceled2.Load())
}

func TestTopic_Keepalive(t *testing.T) {
	t.Parallel()

	to := newTopic("mytopic")
	to.lastAccess = time.Now().Add(-1 * time.Hour)
	to.Keepalive()
	require.True(t, to.LastAccess().Unix() >= time.Now().Unix()-2)
	require.True(t, to.LastAccess().Unix() <= time.Now().Unix()+2)
}

func TestTopic_Subscribe_duplicateID(t *testing.T) {
	t.Parallel()

	to := newTopic("mytopic")

	// fix random seed to force same number generation
	rand.Seed(1)
	a := rand.Int()
	to.subscribers[a] = &topicSubscriber{
		userID:     "a",
		subscriber: nil,
		cancel:     func() {},
	}

	subFn := func(v *visitor, msg *message) error {
		return nil
	}

	// force rand.Int to generate the same id once more
	rand.Seed(1)
	id := to.Subscribe(subFn, "b", func() {})
	res := to.subscribers[id]

	require.False(t, id == a)
	require.True(t, res.userID == "b")
}
