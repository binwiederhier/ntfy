package tray

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"heckel.io/ntfy/v2/client"
)

func TestListenerStartSubscribesConfiguredTopicsAndDispatchesNotifications(t *testing.T) {
	password := "mypass"
	token := "tk_topic"
	conf := client.NewConfig()
	conf.DefaultToken = "tk_default"
	conf.Subscribe = []client.Subscribe{
		{
			Topic: "alerts",
			If: map[string]string{
				"priority": "high",
			},
		},
		{
			Topic:    "secret",
			User:     stringPtr("philipp"),
			Password: &password,
			Token:    &token,
		},
	}

	subscriber := newRecordingSubscriber()
	notifier := newRecordingNotifier()
	listener := NewListener(subscriber, notifier)

	if err := listener.Start(conf); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := len(subscriber.subscriptions), 2; got != want {
		t.Fatalf("subscriptions len = %d, want %d", got, want)
	}
	if got, want := subscriber.subscriptions[0].topic, "alerts"; got != want {
		t.Fatalf("first topic = %q, want %q", got, want)
	}
	if got, want := subscriber.subscriptions[1].topic, "secret"; got != want {
		t.Fatalf("second topic = %q, want %q", got, want)
	}
	alertsReq := requestWithOptions(t, subscriber.subscriptions[0].options)
	if got, want := alertsReq.URL.Query().Get("priority"), "high"; got != want {
		t.Fatalf("alerts priority filter = %q, want %q", got, want)
	}
	if got, want := alertsReq.Header.Get("Authorization"), "Bearer tk_default"; got != want {
		t.Fatalf("alerts authorization = %q, want %q", got, want)
	}
	secretReq := requestWithOptions(t, subscriber.subscriptions[1].options)
	if got, want := secretReq.Header.Get("Authorization"), "Bearer tk_topic"; got != want {
		t.Fatalf("secret authorization = %q, want %q", got, want)
	}

	subscriber.messages <- &client.Message{
		ID:             "m1",
		Topic:          "alerts",
		Title:          "Disk full",
		Message:        "Only 2GB left",
		SubscriptionID: "sub-1",
	}

	notification := notifier.next(t)
	if got, want := len(notifier.notifications), 1; got != want {
		t.Fatalf("notifications len = %d, want %d", got, want)
	}
	assertEqual(t, notification, Notification{
		Title:   "Disk full",
		Message: "Only 2GB left",
		Topic:   "alerts",
	})
}

func TestListenerStartFailsWithoutConfiguredSubscriptions(t *testing.T) {
	listener := NewListener(newRecordingSubscriber(), newRecordingNotifier())

	err := listener.Start(client.NewConfig())

	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if got, want := err.Error(), "no subscriptions configured in client config"; got != want {
		t.Fatalf("Start() error = %q, want %q", got, want)
	}
}

func TestListenerStopUnsubscribesAllSubscriptions(t *testing.T) {
	conf := client.NewConfig()
	conf.Subscribe = []client.Subscribe{{Topic: "alerts"}, {Topic: "ops"}}
	subscriber := newRecordingSubscriber()
	notifier := newRecordingNotifier()
	listener := NewListener(subscriber, notifier)

	if err := listener.Start(conf); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	listener.Stop()
	subscriber.messages <- &client.Message{
		ID:             "m1",
		Topic:          "alerts",
		Message:        "after stop",
		SubscriptionID: "sub-1",
	}

	assertStringSliceEqual(t, subscriber.unsubscribed, []string{"sub-1", "sub-2"})
	if got, want := len(notifier.notifications), 0; got != want {
		t.Fatalf("notifications len = %d, want %d", got, want)
	}
}

func TestListenerStopsWhenMessagesChannelCloses(t *testing.T) {
	conf := client.NewConfig()
	conf.Subscribe = []client.Subscribe{{Topic: "alerts"}}
	subscriber := newRecordingSubscriber()
	notifier := newRecordingNotifier()
	listener := NewListener(subscriber, notifier)

	if err := listener.Start(conf); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	oldNotifications := listener.notifications
	close(subscriber.messages)

	select {
	case <-listener.done:
	case <-time.After(time.Second):
		t.Fatal("listener did not stop after messages channel closed")
	}

	assertStringSliceEqual(t, subscriber.unsubscribed, []string{"sub-1"})
	oldNotifications <- Notification{Title: "stale", Message: "stale", Topic: "alerts"}
	select {
	case notification := <-notifier.notifyCh:
		t.Fatalf("stale notification was dispatched after channel close: %#v", notification)
	case <-time.After(50 * time.Millisecond):
	}
	subscriber.messages = make(chan *client.Message, 10)
	if err := listener.Start(conf); err != nil {
		t.Fatalf("Start() after channel close error = %v", err)
	}
	if got, want := len(subscriber.subscriptions), 2; got != want {
		t.Fatalf("subscriptions len after restart = %d, want %d", got, want)
	}
	listener.Stop()
}

func TestListenerStartStopsExistingSubscriptionsWhenLaterSubscribeFails(t *testing.T) {
	conf := client.NewConfig()
	conf.Subscribe = []client.Subscribe{{Topic: "alerts"}, {Topic: "ops"}}
	client := newRecordingSubscriber()
	client.errByTopic["ops"] = errors.New("subscription failed")
	listener := NewListener(client, newRecordingNotifier())

	err := listener.Start(conf)

	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if got, want := err.Error(), "subscribe ops: subscription failed"; got != want {
		t.Fatalf("Start() error = %q, want %q", got, want)
	}
	assertStringSliceEqual(t, client.unsubscribed, []string{"sub-1"})
}

func TestListenerStopDoesNotWaitForSlowNotifier(t *testing.T) {
	conf := client.NewConfig()
	conf.Subscribe = []client.Subscribe{{Topic: "alerts"}}
	subscriber := newRecordingSubscriber()
	notifier := newBlockingNotifier()
	listener := NewListener(subscriber, notifier)

	if err := listener.Start(conf); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	subscriber.messages <- &client.Message{
		ID:             "m1",
		Topic:          "alerts",
		Message:        "blocked",
		SubscriptionID: "sub-1",
	}
	notifier.waitStarted(t)

	stopped := make(chan struct{})
	go func() {
		listener.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop() waited for blocked notifier")
	}
	notifier.release()
}

type recordingSubscriber struct {
	messages      chan *client.Message
	subscriptions []recordedSubscription
	unsubscribed  []string
	errByTopic    map[string]error
}

type recordedSubscription struct {
	topic   string
	options []client.SubscribeOption
}

func newRecordingSubscriber() *recordingSubscriber {
	return &recordingSubscriber{
		messages:   make(chan *client.Message, 10),
		errByTopic: make(map[string]error),
	}
}

func (r *recordingSubscriber) Subscribe(topic string, options ...client.SubscribeOption) (string, error) {
	if err := r.errByTopic[topic]; err != nil {
		return "", err
	}
	id := "sub-" + string(rune('1'+len(r.subscriptions)))
	r.subscriptions = append(r.subscriptions, recordedSubscription{topic: topic, options: options})
	return id, nil
}

func (r *recordingSubscriber) Unsubscribe(subscriptionID string) {
	r.unsubscribed = append(r.unsubscribed, subscriptionID)
}

func (r *recordingSubscriber) Messages() <-chan *client.Message {
	return r.messages
}

type recordingNotifier struct {
	notifications []Notification
	notifyCh      chan Notification
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{
		notifyCh: make(chan Notification, 10),
	}
}

func (r *recordingNotifier) Notify(notification Notification) error {
	r.notifications = append(r.notifications, notification)
	r.notifyCh <- notification
	return nil
}

func (r *recordingNotifier) next(t *testing.T) Notification {
	t.Helper()
	select {
	case notification := <-r.notifyCh:
		return notification
	case <-time.After(time.Second):
		t.Fatal("notification was not dispatched")
		return Notification{}
	}
}

type blockingNotifier struct {
	started   chan struct{}
	releaseCh chan struct{}
	once      sync.Once
}

func newBlockingNotifier() *blockingNotifier {
	return &blockingNotifier{
		started:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (b *blockingNotifier) Notify(notification Notification) error {
	b.once.Do(func() {
		close(b.started)
	})
	<-b.releaseCh
	return nil
}

func (b *blockingNotifier) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-b.started:
	case <-time.After(time.Second):
		t.Fatal("notifier was not called")
	}
}

func (b *blockingNotifier) release() {
	close(b.releaseCh)
}

func stringPtr(value string) *string {
	return &value
}

func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func requestWithOptions(t *testing.T, options []client.SubscribeOption) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://ntfy.sh/topic/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if err := option(req); err != nil {
			t.Fatalf("option() error = %v", err)
		}
	}
	return req
}
