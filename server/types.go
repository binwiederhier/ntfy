package server

import (
	"heckel.io/ntfy/util"
	"net/http"
	"time"
)

// List of possible events
const (
	openEvent        = "open"
	keepaliveEvent   = "keepalive"
	messageEvent     = "message"
	pollRequestEvent = "poll_request"
)

const (
	messageIDLength = 10
)

// message represents a message published to a topic
type message struct {
	ID         string      `json:"id"`    // Random message ID
	Time       int64       `json:"time"`  // Unix time in seconds
	Event      string      `json:"event"` // One of the above
	Topic      string      `json:"topic"`
	Priority   int         `json:"priority,omitempty"`
	Tags       []string    `json:"tags,omitempty"`
	Click      string      `json:"click,omitempty"`
	Attachment *attachment `json:"attachment,omitempty"`
	Title      string      `json:"title,omitempty"`
	Message    string      `json:"message,omitempty"`
	Encoding   string      `json:"encoding,omitempty"` // empty for raw UTF-8, or "base64" for encoded bytes
}

type attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	URL     string `json:"url"`
	Owner   string `json:"-"` // IP address of uploader, used for rate limiting
}

// messageEncoder is a function that knows how to encode a message
type messageEncoder func(msg *message) (string, error)

// newMessage creates a new message with the current timestamp
func newMessage(event, topic, msg string) *message {
	return &message{
		ID:       util.RandomString(messageIDLength),
		Time:     time.Now().Unix(),
		Event:    event,
		Topic:    topic,
		Priority: 0,
		Tags:     nil,
		Title:    "",
		Message:  msg,
	}
}

// newOpenMessage is a convenience method to create an open message
func newOpenMessage(topic string) *message {
	return newMessage(openEvent, topic, "")
}

// newKeepaliveMessage is a convenience method to create a keepalive message
func newKeepaliveMessage(topic string) *message {
	return newMessage(keepaliveEvent, topic, "")
}

// newDefaultMessage is a convenience method to create a notification message
func newDefaultMessage(topic, msg string) *message {
	return newMessage(messageEvent, topic, msg)
}

type sinceTime time.Time

func (t sinceTime) IsAll() bool {
	return t == sinceAllMessages
}

func (t sinceTime) IsNone() bool {
	return t == sinceNoMessages
}

func (t sinceTime) Time() time.Time {
	return time.Time(t)
}

var (
	sinceAllMessages = sinceTime(time.Unix(0, 0))
	sinceNoMessages  = sinceTime(time.Unix(1, 0))
)

type queryFilter struct {
	Message  string
	Title    string
	Tags     []string
	Priority []int
}

func parseQueryFilters(r *http.Request) (*queryFilter, error) {
	messageFilter := readParam(r, "x-message", "message", "m")
	titleFilter := readParam(r, "x-title", "title", "t")
	tagsFilter := util.SplitNoEmpty(readParam(r, "x-tags", "tags", "tag", "ta"), ",")
	priorityFilter := make([]int, 0)
	for _, p := range util.SplitNoEmpty(readParam(r, "x-priority", "priority", "prio", "p"), ",") {
		priority, err := util.ParsePriority(p)
		if err != nil {
			return nil, err
		}
		priorityFilter = append(priorityFilter, priority)
	}
	return &queryFilter{
		Message:  messageFilter,
		Title:    titleFilter,
		Tags:     tagsFilter,
		Priority: priorityFilter,
	}, nil
}

func (q *queryFilter) Pass(msg *message) bool {
	if msg.Event != messageEvent {
		return true // filters only apply to messages
	}
	if q.Message != "" && msg.Message != q.Message {
		return false
	}
	if q.Title != "" && msg.Title != q.Title {
		return false
	}
	messagePriority := msg.Priority
	if messagePriority == 0 {
		messagePriority = 3 // For query filters, default priority (3) is the same as "not set" (0)
	}
	if len(q.Priority) > 0 && !util.InIntList(q.Priority, messagePriority) {
		return false
	}
	if len(q.Tags) > 0 && !util.InStringListAll(msg.Tags, q.Tags) {
		return false
	}
	return true
}
