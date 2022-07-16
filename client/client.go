// Package client provides a ntfy client to publish and subscribe to topics
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event type constants
const (
	MessageEvent     = "message"
	KeepaliveEvent   = "keepalive"
	OpenEvent        = "open"
	PollRequestEvent = "poll_request"
)

const (
	maxResponseBytes = 4096
)

// Client is the ntfy client that can be used to publish and subscribe to ntfy topics
type Client struct {
	Messages      chan *Message
	config        *Config
	subscriptions map[string]*subscription
	mu            sync.Mutex
}

// Message is a struct that represents a ntfy message
type Message struct { // TODO combine with server.message
	ID         string
	Event      string
	Time       int64
	Topic      string
	Message    string
	Title      string
	Priority   int
	Tags       []string
	Click      string
	Icon       *Icon
	Attachment *Attachment

	// Additional fields
	TopicURL       string
	SubscriptionID string
	Raw            string
}

// Attachment represents a message attachment
type Attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	URL     string `json:"url"`
	Owner   string `json:"-"` // IP address of uploader, used for rate limiting
}

// Icon represents a message icon
type Icon struct {
	Url  string `json:"url"`
	Type string `json:"type,omitempty"`
	Size int64  `json:"size,omitempty"`
}

type subscription struct {
	ID       string
	topicURL string
	cancel   context.CancelFunc
}

// New creates a new Client using a given Config
func New(config *Config) *Client {
	return &Client{
		Messages:      make(chan *Message, 50), // Allow reading a few messages
		config:        config,
		subscriptions: make(map[string]*subscription),
	}
}

// Publish sends a message to a specific topic, optionally using options.
// See PublishReader for details.
func (c *Client) Publish(topic, message string, options ...PublishOption) (*Message, error) {
	return c.PublishReader(topic, strings.NewReader(message), options...)
}

// PublishReader sends a message to a specific topic, optionally using options.
//
// A topic can be either a full URL (e.g. https://myhost.lan/mytopic), a short URL which is then prepended https://
// (e.g. myhost.lan -> https://myhost.lan), or a short name which is expanded using the default host in the
// config (e.g. mytopic -> https://ntfy.sh/mytopic).
//
// To pass title, priority and tags, check out WithTitle, WithPriority, WithTagsList, WithDelay, WithNoCache,
// WithNoFirebase, and the generic WithHeader.
func (c *Client) PublishReader(topic string, body io.Reader, options ...PublishOption) (*Message, error) {
	topicURL := c.expandTopicURL(topic)
	req, _ := http.NewRequest("POST", topicURL, body)
	for _, option := range options {
		if err := option(req); err != nil {
			return nil, err
		}
	}
	log.Debug("%s Publishing message with headers %s", util.ShortTopicURL(topicURL), req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(strings.TrimSpace(string(b)))
	}
	m, err := toMessage(string(b), topicURL, "")
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Poll queries a topic for all (or a limited set) of messages. Unlike Subscribe, this method only polls for
// messages and does not subscribe to messages that arrive after this call.
//
// A topic can be either a full URL (e.g. https://myhost.lan/mytopic), a short URL which is then prepended https://
// (e.g. myhost.lan -> https://myhost.lan), or a short name which is expanded using the default host in the
// config (e.g. mytopic -> https://ntfy.sh/mytopic).
//
// By default, all messages will be returned, but you can change this behavior using a SubscribeOption.
// See WithSince, WithSinceAll, WithSinceUnixTime, WithScheduled, and the generic WithQueryParam.
func (c *Client) Poll(topic string, options ...SubscribeOption) ([]*Message, error) {
	ctx := context.Background()
	messages := make([]*Message, 0)
	msgChan := make(chan *Message)
	errChan := make(chan error)
	topicURL := c.expandTopicURL(topic)
	log.Debug("%s Polling from topic", util.ShortTopicURL(topicURL))
	options = append(options, WithPoll())
	go func() {
		err := performSubscribeRequest(ctx, msgChan, topicURL, "", options...)
		close(msgChan)
		errChan <- err
	}()
	for m := range msgChan {
		messages = append(messages, m)
	}
	return messages, <-errChan
}

// Subscribe subscribes to a topic to listen for newly incoming messages. The method starts a connection in the
// background and returns new messages via the Messages channel.
//
// A topic can be either a full URL (e.g. https://myhost.lan/mytopic), a short URL which is then prepended https://
// (e.g. myhost.lan -> https://myhost.lan), or a short name which is expanded using the default host in the
// config (e.g. mytopic -> https://ntfy.sh/mytopic).
//
// By default, only new messages will be returned, but you can change this behavior using a SubscribeOption.
// See WithSince, WithSinceAll, WithSinceUnixTime, WithScheduled, and the generic WithQueryParam.
//
// The method returns a unique subscriptionID that can be used in Unsubscribe.
//
// Example:
//   c := client.New(client.NewConfig())
//   subscriptionID := c.Subscribe("mytopic")
//   for m := range c.Messages {
//     fmt.Printf("New message: %s", m.Message)
//   }
func (c *Client) Subscribe(topic string, options ...SubscribeOption) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	subscriptionID := util.RandomString(10)
	topicURL := c.expandTopicURL(topic)
	log.Debug("%s Subscribing to topic", util.ShortTopicURL(topicURL))
	ctx, cancel := context.WithCancel(context.Background())
	c.subscriptions[subscriptionID] = &subscription{
		ID:       subscriptionID,
		topicURL: topicURL,
		cancel:   cancel,
	}
	go handleSubscribeConnLoop(ctx, c.Messages, topicURL, subscriptionID, options...)
	return subscriptionID
}

// Unsubscribe unsubscribes from a topic that has been previously subscribed to using the unique
// subscriptionID returned in Subscribe.
func (c *Client) Unsubscribe(subscriptionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sub, ok := c.subscriptions[subscriptionID]
	if !ok {
		return
	}
	delete(c.subscriptions, subscriptionID)
	sub.cancel()
}

// UnsubscribeAll unsubscribes from a topic that has been previously subscribed with Subscribe.
// If there are multiple subscriptions matching the topic, all of them are unsubscribed from.
//
// A topic can be either a full URL (e.g. https://myhost.lan/mytopic), a short URL which is then prepended https://
// (e.g. myhost.lan -> https://myhost.lan), or a short name which is expanded using the default host in the
// config (e.g. mytopic -> https://ntfy.sh/mytopic).
func (c *Client) UnsubscribeAll(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	topicURL := c.expandTopicURL(topic)
	for _, sub := range c.subscriptions {
		if sub.topicURL == topicURL {
			delete(c.subscriptions, sub.ID)
			sub.cancel()
		}
	}
}

func (c *Client) expandTopicURL(topic string) string {
	if strings.HasPrefix(topic, "http://") || strings.HasPrefix(topic, "https://") {
		return topic
	} else if strings.Contains(topic, "/") {
		return fmt.Sprintf("https://%s", topic)
	}
	return fmt.Sprintf("%s/%s", c.config.DefaultHost, topic)
}

func handleSubscribeConnLoop(ctx context.Context, msgChan chan *Message, topicURL, subcriptionID string, options ...SubscribeOption) {
	for {
		// TODO The retry logic is crude and may lose messages. It should record the last message like the
		//      Android client, use since=, and do incremental backoff too
		if err := performSubscribeRequest(ctx, msgChan, topicURL, subcriptionID, options...); err != nil {
			log.Warn("%s Connection failed: %s", util.ShortTopicURL(topicURL), err.Error())
		}
		select {
		case <-ctx.Done():
			log.Info("%s Connection exited", util.ShortTopicURL(topicURL))
			return
		case <-time.After(10 * time.Second): // TODO Add incremental backoff
		}
	}
}

func performSubscribeRequest(ctx context.Context, msgChan chan *Message, topicURL string, subscriptionID string, options ...SubscribeOption) error {
	streamURL := fmt.Sprintf("%s/json", topicURL)
	log.Debug("%s Listening to %s", util.ShortTopicURL(topicURL), streamURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}
	for _, option := range options {
		if err := option(req); err != nil {
			return err
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return err
		}
		return errors.New(strings.TrimSpace(string(b)))
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		messageJSON := scanner.Text()
		m, err := toMessage(messageJSON, topicURL, subscriptionID)
		if err != nil {
			return err
		}
		log.Trace("%s Message received: %s", util.ShortTopicURL(topicURL), messageJSON)
		if m.Event == MessageEvent {
			msgChan <- m
		}
	}
	return nil
}

func toMessage(s, topicURL, subscriptionID string) (*Message, error) {
	var m *Message
	if err := json.NewDecoder(strings.NewReader(s)).Decode(&m); err != nil {
		return nil, err
	}
	m.TopicURL = topicURL
	m.SubscriptionID = subscriptionID
	m.Raw = s
	return m, nil
}
