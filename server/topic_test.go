package server

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
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
	to.Subscribe(subFn, nil, cancelFn1)
	to.Subscribe(subFn, testSubscriberUser("u_phil"), cancelFn2)

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
	to.Subscribe(subFn, testSubscriberUser("u_another"), cancelFn1)
	to.Subscribe(subFn, testSubscriberUser("u_phil"), cancelFn2)

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
		user:       testSubscriberUser("a"),
		subscriber: nil,
		cancel:     func() {},
	}

	subFn := func(v *visitor, msg *model.Message) error {
		return nil
	}

	//lint:ignore SA1019 Force rand.Int to generate the same id once more
	rand.Seed(1)
	id := to.Subscribe(subFn, testSubscriberUser("b"), func() {})
	res := to.subscribers[id]

	require.NotEqual(t, id, a)
	require.Equal(t, "b", res.userID(), "b")
}

func TestTopic_PublishMissed_OnlyToSubscribersThatMissedIt(t *testing.T) {
	to := newTopic("mytopic")
	var early, late atomic.Int32
	to.Subscribe(countingSubscriber(&early), nil, func() {})
	m := model.NewDefaultMessage("mytopic", "hi")
	require.Nil(t, to.Publish(nil, m))
	waitFor(t, func() bool { return early.Load() == 1 })

	to.Subscribe(countingSubscriber(&late), nil, func() {}) // Joined after the live delivery
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
	to.Subscribe(countingSubscriber(&count), nil, func() {})
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
	}, nil, func() {})
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

func TestTopic_PublishMissed_InWriteOrderAcrossReads(t *testing.T) {
	// A subscriber still busy with one read's messages must not get the next read's first
	to := newTopic("mytopic")
	firstEntered, releaseFirst := make(chan struct{}), make(chan struct{})
	received := make(chan string, 2)
	to.Subscribe(func(_ *visitor, m *model.Message) error {
		if m.Message == "first" {
			close(firstEntered)
			<-releaseFirst
		}
		received <- m.Message
		return nil
	}, nil, func() {})
	to.PublishMissed([]*model.Message{model.NewDefaultMessage("mytopic", "first")})
	<-firstEntered
	to.PublishMissed([]*model.Message{model.NewDefaultMessage("mytopic", "second")})
	time.Sleep(200 * time.Millisecond) // Room for "second" to overtake
	close(releaseFirst)
	require.Equal(t, "first", <-received)
	require.Equal(t, "second", <-received)
}

func TestTopic_ExpireDelivered(t *testing.T) {
	// Stale only answers; forgetting old deliveries is ExpireDelivered's job
	to := newTopic("mytopic")
	to.delivered["old"] = time.Now().Add(-topicDeliveredRetain - time.Second)
	to.delivered["new"] = time.Now()
	require.False(t, to.Stale())
	_, ok := to.DeliveredAt("old")
	require.True(t, ok)

	to.ExpireDelivered()
	_, ok = to.DeliveredAt("old")
	require.False(t, ok)
	_, ok = to.DeliveredAt("new")
	require.True(t, ok)
}

func TestTopic_Publish_InOrderPerSubscriber(t *testing.T) {
	// Live messages, like the tail's, reach each subscriber in publish order: a slow send of one
	// must not let the next overtake it
	to := newTopic("mytopic")
	firstEntered, releaseFirst := make(chan struct{}), make(chan struct{})
	received := make(chan string, 2)
	to.Subscribe(func(_ *visitor, m *model.Message) error {
		if m.Message == "first" {
			close(firstEntered)
			<-releaseFirst
		}
		received <- m.Message
		return nil
	}, nil, func() {})
	require.Nil(t, to.Publish(nil, model.NewDefaultMessage("mytopic", "first")))
	<-firstEntered
	require.Nil(t, to.Publish(nil, model.NewDefaultMessage("mytopic", "second")))
	time.Sleep(200 * time.Millisecond) // Room for "second" to overtake
	close(releaseFirst)
	require.Equal(t, "first", <-received)
	require.Equal(t, "second", <-received)
}

func TestTopic_Publish_LateFanOutReachesLaterSubscribers(t *testing.T) {
	// The tail delivered the message first; the peer's fan-out arrives later. Subscribers that
	// had it are skipped, one that subscribed in between gets it: the same rule as the tail's.
	to := newTopic("mytopic")
	var early, late atomic.Int32
	to.Subscribe(countingSubscriber(&early), nil, func() {})
	m := model.NewDefaultMessage("mytopic", "hi")
	to.PublishMissed([]*model.Message{m})
	waitFor(t, func() bool { return early.Load() == 1 })
	to.Subscribe(countingSubscriber(&late), nil, func() {})
	require.Nil(t, to.Publish(nil, m))
	waitFor(t, func() bool { return late.Load() == 1 })
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(1), early.Load())
}

func TestTopic_CancelSubscriberUser_AllOfTheUsersSubscribers(t *testing.T) {
	// A user (or anonymous, "") can hold several streams on one topic; revoking access ends all
	// of them, and nobody else's
	to := newTopic("mytopic")
	canceled := make(map[string]int)
	var mu sync.Mutex
	subscribe := func(userID string) {
		to.Subscribe(func(*visitor, *model.Message) error {
			return nil
		}, testSubscriberUser(userID), func() {
			mu.Lock()
			defer mu.Unlock()
			canceled[userID]++
		})
	}
	subscribe("u_alice")
	subscribe("u_alice")
	subscribe("")
	subscribe("")
	subscribe("u_carol")
	to.CancelSubscriberUser("u_alice")
	to.CancelSubscriberUser("") // An empty user ID cancels nobody: anonymous streams need the explicit call
	mu.Lock()
	require.Equal(t, map[string]int{"u_alice": 2}, canceled)
	mu.Unlock()

	to.CancelSubscribersWhere(func(u *user.User) bool {
		return u == nil // Anonymous
	})
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, map[string]int{"u_alice": 2, "": 2}, canceled)
}

func TestTopic_CancelSubscribersWithContextLoggingDoesNotDeadlock(t *testing.T) {
	// With log level overrides (or debug logging), canceling a subscriber logs the topic's context,
	// which takes the topic lock. A cancel must not hold that lock while it logs: holding the write
	// lock deadlocks at once, and holding a read lock deadlocks as soon as a canceled stream's
	// Unsubscribe waits for the write lock (seen in the lab, which froze the access cache reload)
	log.SetLevelOverride("tag", tagSubscribe, log.DebugLevel)
	t.Cleanup(log.ResetLevelOverrides)
	to := newTopic("mytopic")
	subscribe := func(userID string) {
		var id int
		id = to.Subscribe(func(*visitor, *model.Message) error {
			return nil
		}, testSubscriberUser(userID), func() {
			go to.Unsubscribe(id) // Like a real stream: its handler unsubscribes when it ends
			time.Sleep(50 * time.Millisecond)
		})
	}
	for _, userID := range []string{"u_owner", "u_other", "u_other", "u_revoked", "u_revoked", "", ""} {
		subscribe(userID)
	}
	done := make(chan struct{})
	go func() {
		to.CancelSubscriberUser("u_revoked")
		to.CancelSubscribersWhere(func(u *user.User) bool {
			return u == nil
		})
		to.CancelSubscribersExceptUser("u_owner")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceling subscribers deadlocked")
	}
	require.Eventually(t, func() bool {
		return to.SubscribersCount() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

// testSubscriberUser returns a user with just an ID, as a topic subscriber; nil (anonymous) for ""
func testSubscriberUser(id string) *user.User {
	if id == "" {
		return nil
	}
	return &user.User{
		ID: id,
	}
}
