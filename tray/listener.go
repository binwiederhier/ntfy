package tray

import (
	"errors"
	"fmt"
	"sync"

	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/log"
)

const notificationQueueSize = 50

// Notification is the platform-neutral notification payload shown for an ntfy message.
type Notification struct {
	Title   string
	Message string
	Topic   string
}

// Notifier displays notifications received from subscribed topics.
type Notifier interface {
	Notify(notification Notification) error
}

// Subscriber is the subset of client.Client used by Listener.
type Subscriber interface {
	Subscribe(topic string, options ...client.SubscribeOption) (string, error)
	Unsubscribe(subscriptionID string)
	Messages() <-chan *client.Message
}

// ClientSubscriber adapts client.Client to Subscriber.
type ClientSubscriber struct {
	client *client.Client
}

// NewClientSubscriber returns a Subscriber backed by the ntfy client package.
func NewClientSubscriber(conf *client.Config) *ClientSubscriber {
	return &ClientSubscriber{client: client.New(conf)}
}

// Subscribe subscribes to a topic.
func (s *ClientSubscriber) Subscribe(topic string, options ...client.SubscribeOption) (string, error) {
	return s.client.Subscribe(topic, options...)
}

// Unsubscribe unsubscribes from a topic.
func (s *ClientSubscriber) Unsubscribe(subscriptionID string) {
	s.client.Unsubscribe(subscriptionID)
}

// Messages returns the stream of subscribed messages.
func (s *ClientSubscriber) Messages() <-chan *client.Message {
	return s.client.Messages
}

// Listener owns configured topic subscriptions and dispatches messages to a Notifier.
type Listener struct {
	subscriber Subscriber
	notifier   Notifier

	mu              sync.Mutex
	subscriptionIDs []string
	stop            chan struct{}
	stopOnce        *sync.Once
	done            chan struct{}
	notifications   chan Notification
	running         bool
}

// NewListener creates a Listener.
func NewListener(subscriber Subscriber, notifier Notifier) *Listener {
	return &Listener{
		subscriber: subscriber,
		notifier:   notifier,
	}
}

// Start subscribes to every topic configured in client.yml and starts message dispatch.
func (l *Listener) Start(conf *client.Config) error {
	if conf == nil || len(conf.Subscribe) == 0 {
		return errors.New("no subscriptions configured in client config")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return nil
	}

	subscriptionIDs := make([]string, 0, len(conf.Subscribe))
	for _, subscription := range conf.Subscribe {
		options := subscribeOptions(subscription, conf)
		subscriptionID, err := l.subscriber.Subscribe(subscription.Topic, options...)
		if err != nil {
			for _, id := range subscriptionIDs {
				l.subscriber.Unsubscribe(id)
			}
			return fmt.Errorf("subscribe %s: %w", subscription.Topic, err)
		}
		subscriptionIDs = append(subscriptionIDs, subscriptionID)
	}

	l.subscriptionIDs = subscriptionIDs
	l.stop = make(chan struct{})
	l.stopOnce = &sync.Once{}
	l.done = make(chan struct{})
	l.notifications = make(chan Notification, notificationQueueSize)
	l.running = true
	go l.runNotifier(l.stop, l.notifications)
	go l.run(l.stop, l.done)
	return nil
}

// Stop cancels all active subscriptions and waits for the dispatch loop to stop.
func (l *Listener) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	subscriptionIDs := append([]string(nil), l.subscriptionIDs...)
	stop := l.stop
	stopOnce := l.stopOnce
	done := l.done
	l.subscriptionIDs = nil
	l.notifications = nil
	l.running = false
	l.mu.Unlock()

	for _, id := range subscriptionIDs {
		l.subscriber.Unsubscribe(id)
	}
	stopOnce.Do(func() {
		close(stop)
	})
	<-done
}

func (l *Listener) run(stop chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case message, ok := <-l.subscriber.Messages():
			if !ok {
				l.markStopped(stop, l.stopOnce)
				return
			}
			if message == nil {
				continue
			}
			l.enqueueNotification(message)
		}
	}
}

func (l *Listener) enqueueNotification(message *client.Message) {
	notification := notificationFromMessage(message)
	l.mu.Lock()
	notifications := l.notifications
	l.mu.Unlock()
	if notifications == nil {
		return
	}
	select {
	case notifications <- notification:
	default:
		log.Warn("Notification queue is full, dropping notification for %s", message.ID)
	}
}

func (l *Listener) runNotifier(stop <-chan struct{}, notifications <-chan Notification) {
	for {
		select {
		case <-stop:
			return
		case notification := <-notifications:
			if err := l.notifier.Notify(notification); err != nil {
				log.Warn("Failed to show notification for %s: %s", notification.Topic, err.Error())
			}
		}
	}
}

func (l *Listener) markStopped(stop chan struct{}, stopOnce *sync.Once) {
	l.mu.Lock()
	subscriptionIDs := append([]string(nil), l.subscriptionIDs...)
	l.subscriptionIDs = nil
	l.notifications = nil
	l.running = false
	l.mu.Unlock()
	for _, id := range subscriptionIDs {
		l.subscriber.Unsubscribe(id)
	}
	stopOnce.Do(func() {
		close(stop)
	})
}

func subscribeOptions(subscription client.Subscribe, conf *client.Config) []client.SubscribeOption {
	options := make([]client.SubscribeOption, 0)
	for filter, value := range subscription.If {
		options = append(options, client.WithFilter(filter, value))
	}
	if auth := subscribeAuthOption(subscription, conf); auth != nil {
		options = append(options, auth)
	}
	return options
}

func subscribeAuthOption(subscription client.Subscribe, conf *client.Config) client.SubscribeOption {
	if (subscription.Token != nil && *subscription.Token == "") ||
		(subscription.User != nil && *subscription.User == "" && subscription.Password != nil && *subscription.Password == "") {
		return client.WithEmptyAuth()
	}
	if subscription.Token != nil && *subscription.Token != "" {
		return client.WithBearerAuth(*subscription.Token)
	}
	if subscription.User != nil && *subscription.User != "" && subscription.Password != nil {
		return client.WithBasicAuth(*subscription.User, *subscription.Password)
	}
	if conf.DefaultToken != "" {
		return client.WithBearerAuth(conf.DefaultToken)
	}
	if conf.DefaultUser != "" && conf.DefaultPassword != nil {
		return client.WithBasicAuth(conf.DefaultUser, *conf.DefaultPassword)
	}
	return nil
}

func notificationFromMessage(message *client.Message) Notification {
	title := message.Title
	if title == "" {
		title = fmt.Sprintf("ntfy: %s", message.Topic)
	}
	return Notification{
		Title:   title,
		Message: message.Message,
		Topic:   message.Topic,
	}
}
