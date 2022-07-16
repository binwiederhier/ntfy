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
	messageIDLength = 12
)

// message represents a message published to a topic
type message struct {
	ID         string      `json:"id"`    // Random message ID
	Time       int64       `json:"time"`  // Unix time in seconds
	Event      string      `json:"event"` // One of the above
	Topic      string      `json:"topic"`
	Title      string      `json:"title,omitempty"`
	Message    string      `json:"message,omitempty"`
	Priority   int         `json:"priority,omitempty"`
	Tags       []string    `json:"tags,omitempty"`
	Click      string      `json:"click,omitempty"`
	Actions    []*action   `json:"actions,omitempty"`
	Attachment *attachment `json:"attachment,omitempty"`
	Icon       *icon       `json:"icon,omitempty"`
	PollID     string      `json:"poll_id,omitempty"`
	Sender     string      `json:"-"`                  // IP address of uploader, used for rate limiting
	Encoding   string      `json:"encoding,omitempty"` // empty for raw UTF-8, or "base64" for encoded bytes
}

type attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	URL     string `json:"url"`
}

type icon struct {
	URL  string `json:"url"`
	Type string `json:"type,omitempty"`
	Size int64  `json:"size,omitempty"`
}

type action struct {
	ID      string            `json:"id"`
	Action  string            `json:"action"`            // "view", "broadcast", or "http"
	Label   string            `json:"label"`             // action button label
	Clear   bool              `json:"clear"`             // clear notification after successful execution
	URL     string            `json:"url,omitempty"`     // used in "view" and "http" actions
	Method  string            `json:"method,omitempty"`  // used in "http" action, default is POST (!)
	Headers map[string]string `json:"headers,omitempty"` // used in "http" action
	Body    string            `json:"body,omitempty"`    // used in "http" action
	Intent  string            `json:"intent,omitempty"`  // used in "broadcast" action
	Extras  map[string]string `json:"extras,omitempty"`  // used in "broadcast" action
}

func newAction() *action {
	return &action{
		Headers: make(map[string]string),
		Extras:  make(map[string]string),
	}
}

// publishMessage is used as input when publishing as JSON
type publishMessage struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags"`
	Click    string   `json:"click"`
	Actions  []action `json:"actions"`
	Attach   string   `json:"attach"`
	Icon     string   `json:"icon"`
	Filename string   `json:"filename"`
	Email    string   `json:"email"`
	Delay    string   `json:"delay"`
}

// messageEncoder is a function that knows how to encode a message
type messageEncoder func(msg *message) (string, error)

// newMessage creates a new message with the current timestamp
func newMessage(event, topic, msg string) *message {
	return &message{
		ID:      util.RandomString(messageIDLength),
		Time:    time.Now().Unix(),
		Event:   event,
		Topic:   topic,
		Message: msg,
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

// newPollRequestMessage is a convenience method to create a poll request message
func newPollRequestMessage(topic, pollID string) *message {
	m := newMessage(pollRequestEvent, topic, newMessageBody)
	m.PollID = pollID
	return m
}

func validMessageID(s string) bool {
	return util.ValidRandomString(s, messageIDLength)
}

type sinceMarker struct {
	time time.Time
	id   string
}

func newSinceTime(timestamp int64) sinceMarker {
	return sinceMarker{time.Unix(timestamp, 0), ""}
}

func newSinceID(id string) sinceMarker {
	return sinceMarker{time.Unix(0, 0), id}
}

func (t sinceMarker) IsAll() bool {
	return t == sinceAllMessages
}

func (t sinceMarker) IsNone() bool {
	return t == sinceNoMessages
}

func (t sinceMarker) IsID() bool {
	return t.id != ""
}

func (t sinceMarker) Time() time.Time {
	return t.time
}

func (t sinceMarker) ID() string {
	return t.id
}

var (
	sinceAllMessages = sinceMarker{time.Unix(0, 0), ""}
	sinceNoMessages  = sinceMarker{time.Unix(1, 0), ""}
)

type queryFilter struct {
	ID       string
	Message  string
	Title    string
	Tags     []string
	Priority []int
}

func parseQueryFilters(r *http.Request) (*queryFilter, error) {
	idFilter := readParam(r, "x-id", "id")
	messageFilter := readParam(r, "x-message", "message", "m")
	titleFilter := readParam(r, "x-title", "title", "t")
	tagsFilter := util.SplitNoEmpty(readParam(r, "x-tags", "tags", "tag", "ta"), ",")
	priorityFilter := make([]int, 0)
	for _, p := range util.SplitNoEmpty(readParam(r, "x-priority", "priority", "prio", "p"), ",") {
		priority, err := util.ParsePriority(p)
		if err != nil {
			return nil, errHTTPBadRequestPriorityInvalid
		}
		priorityFilter = append(priorityFilter, priority)
	}
	return &queryFilter{
		ID:       idFilter,
		Message:  messageFilter,
		Title:    titleFilter,
		Tags:     tagsFilter,
		Priority: priorityFilter,
	}, nil
}

func (q *queryFilter) Pass(msg *message) bool {
	if msg.Event != messageEvent {
		return true // filters only apply to messages
	} else if q.ID != "" && msg.ID != q.ID {
		return false
	} else if q.Message != "" && msg.Message != q.Message {
		return false
	} else if q.Title != "" && msg.Title != q.Title {
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
