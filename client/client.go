package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	MessageEvent   = "message"
	KeepaliveEvent = "keepalive"
	OpenEvent      = "open"
)

type Client struct {
	Messages      chan *Message
	config        *Config
	subscriptions map[string]*subscription
	mu            sync.Mutex
}

type Message struct {
	ID       string
	Event    string
	Time     int64
	Topic    string
	TopicURL string
	Message  string
	Title    string
	Priority int
	Tags     []string
	Raw      string
}

type subscription struct {
	cancel context.CancelFunc
}

func New(config *Config) *Client {
	return &Client{
		Messages:      make(chan *Message),
		config:        config,
		subscriptions: make(map[string]*subscription),
	}
}

func (c *Client) Publish(topic, message string, options ...PublishOption) error {
	topicURL := c.expandTopicURL(topic)
	req, _ := http.NewRequest("POST", topicURL, strings.NewReader(message))
	for _, option := range options {
		if err := option(req); err != nil {
			return err
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response %d from server", resp.StatusCode)
	}
	return err
}

func (c *Client) Poll(topic string, options ...SubscribeOption) ([]*Message, error) {
	ctx := context.Background()
	messages := make([]*Message, 0)
	msgChan := make(chan *Message)
	errChan := make(chan error)
	topicURL := c.expandTopicURL(topic)
	go func() {
		err := performSubscribeRequest(ctx, msgChan, topicURL, options...)
		close(msgChan)
		errChan <- err
	}()
	for m := range msgChan {
		messages = append(messages, m)
	}
	return messages, <-errChan
}

func (c *Client) Subscribe(topic string, options ...SubscribeOption) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	topicURL := c.expandTopicURL(topic)
	if _, ok := c.subscriptions[topicURL]; ok {
		return topicURL
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.subscriptions[topicURL] = &subscription{cancel}
	go handleSubscribeConnLoop(ctx, c.Messages, topicURL, options...)
	return topicURL
}

func (c *Client) Unsubscribe(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	topicURL := c.expandTopicURL(topic)
	sub, ok := c.subscriptions[topicURL]
	if !ok {
		return
	}
	sub.cancel()
	return
}

func (c *Client) expandTopicURL(topic string) string {
	if strings.HasPrefix(topic, "http://") || strings.HasPrefix(topic, "https://") {
		return topic
	} else if strings.Contains(topic, "/") {
		return fmt.Sprintf("https://%s", topic)
	}
	return fmt.Sprintf("%s/%s", c.config.DefaultHost, topic)
}

func handleSubscribeConnLoop(ctx context.Context, msgChan chan *Message, topicURL string, options ...SubscribeOption) {
	for {
		if err := performSubscribeRequest(ctx, msgChan, topicURL, options...); err != nil {
			log.Printf("Connection to %s failed: %s", topicURL, err.Error())
		}
		select {
		case <-ctx.Done():
			log.Printf("Connection to %s exited", topicURL)
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func performSubscribeRequest(ctx context.Context, msgChan chan *Message, topicURL string, options ...SubscribeOption) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/json", topicURL), nil)
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
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var m *Message
		line := scanner.Text()
		if err := json.NewDecoder(strings.NewReader(line)).Decode(&m); err != nil {
			return err
		}
		m.TopicURL = topicURL
		m.Raw = line
		msgChan <- m
	}
	return nil
}
