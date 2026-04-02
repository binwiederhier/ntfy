package model

import (
	"errors"
	"net/netip"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

// List of possible events
const (
	OpenEvent          = "open"
	KeepaliveEvent     = "keepalive"
	MessageEvent       = "message"
	MessageDeleteEvent = "message_delete"
	MessageClearEvent  = "message_clear"
	PollRequestEvent   = "poll_request"
)

// messageIDLength is the length of a randomly generated message ID
const messageIDLength = 12

// Errors for message operations
var (
	ErrUnexpectedMessageType = errors.New("unexpected message type")
	ErrMessageNotFound       = errors.New("message not found")
)

// Message represents a message published to a topic
type Message struct {
	ID          string      `json:"id"`                    // Random message ID
	SequenceID  string      `json:"sequence_id,omitempty"` // Message sequence ID for updating message contents (omitted if same as ID)
	Time        int64       `json:"time"`                  // Unix time in seconds
	Expires     int64       `json:"expires,omitempty"`     // Unix time in seconds (not required for open/keepalive)
	Event       string      `json:"event"`                 // One of the above
	Topic       string      `json:"topic"`
	Title       string      `json:"title,omitempty"`
	Message     string      `json:"message,omitempty"`
	Priority    int         `json:"priority,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Click       string      `json:"click,omitempty"`
	Icon        string      `json:"icon,omitempty"`
	Actions     []*Action   `json:"actions,omitempty"`
	Attachment  *Attachment `json:"attachment,omitempty"`
	PollID      string      `json:"poll_id,omitempty"`
	ContentType string      `json:"content_type,omitempty"` // text/plain by default (if empty), or text/markdown
	Encoding    string      `json:"encoding,omitempty"`     // Empty for raw UTF-8, or "base64" for encoded bytes
	Sender      netip.Addr  `json:"-"`                      // IP address of uploader, used for rate limiting
	User        string      `json:"-"`                      // UserID of the uploader, used to associated attachments
}

// Context returns a log context for the message
func (m *Message) Context() log.Context {
	fields := map[string]any{
		"topic":               m.Topic,
		"message_id":          m.ID,
		"message_sequence_id": m.SequenceID,
		"message_time":        m.Time,
		"message_event":       m.Event,
		"message_body_size":   len(m.Message),
	}
	if m.Sender.IsValid() {
		fields["message_sender"] = m.Sender.String()
	}
	if m.User != "" {
		fields["message_user"] = m.User
	}
	return fields
}

// SanitizeUTF8 replaces invalid UTF-8 sequences and strips NUL bytes from all user-supplied
// string fields. This is called early in the publish path so that all downstream consumers
// (Firebase, WebPush, SMTP, cache) receive clean UTF-8 strings.
func (m *Message) SanitizeUTF8() {
	m.Topic = util.SanitizeUTF8(m.Topic)
	m.Message = util.SanitizeUTF8(m.Message)
	m.Title = util.SanitizeUTF8(m.Title)
	m.Click = util.SanitizeUTF8(m.Click)
	m.Icon = util.SanitizeUTF8(m.Icon)
	m.ContentType = util.SanitizeUTF8(m.ContentType)
	for i, tag := range m.Tags {
		m.Tags[i] = util.SanitizeUTF8(tag)
	}
	if m.Attachment != nil {
		m.Attachment.Name = util.SanitizeUTF8(m.Attachment.Name)
		m.Attachment.Type = util.SanitizeUTF8(m.Attachment.Type)
		m.Attachment.URL = util.SanitizeUTF8(m.Attachment.URL)
	}
}

// ForJSON returns a copy of the message suitable for JSON output.
// It clears the SequenceID if it equals the ID to reduce redundancy.
func (m *Message) ForJSON() *Message {
	if m.SequenceID == m.ID {
		clone := *m
		clone.SequenceID = ""
		return &clone
	}
	return m
}

// Attachment represents a file attachment on a message
type Attachment struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	URL     string `json:"url"`
}

// Action represents a user-defined action on a message
type Action struct {
	ID      string            `json:"id"`
	Action  string            `json:"action"`            // "view", "broadcast", "http", or "copy"
	Label   string            `json:"label"`             // action button label
	Clear   bool              `json:"clear"`             // clear notification after successful execution
	URL     string            `json:"url,omitempty"`     // used in "view" and "http" actions
	Method  string            `json:"method,omitempty"`  // used in "http" action, default is POST (!)
	Headers map[string]string `json:"headers,omitempty"` // used in "http" action
	Body    string            `json:"body,omitempty"`    // used in "http" action
	Intent  string            `json:"intent,omitempty"`  // used in "broadcast" action
	Extras  map[string]string `json:"extras,omitempty"`  // used in "broadcast" action
	Value   string            `json:"value,omitempty"`   // used in "copy" action
}

// NewAction creates a new action with initialized maps
func NewAction() *Action {
	return &Action{
		Headers: make(map[string]string),
		Extras:  make(map[string]string),
	}
}

// GenerateMessageID creates a new random message ID
func GenerateMessageID() string {
	return util.RandomString(messageIDLength)
}

// ValidMessageID returns true if the given string is a valid message ID
func ValidMessageID(s string) bool {
	return util.ValidRandomString(s, messageIDLength)
}

// NewMessage creates a new message with the current timestamp
func NewMessage(event, topic, msg string) *Message {
	return &Message{
		ID:      GenerateMessageID(),
		Time:    time.Now().Unix(),
		Event:   event,
		Topic:   topic,
		Message: msg,
	}
}

// NewOpenMessage is a convenience method to create an open message
func NewOpenMessage(topic string) *Message {
	return NewMessage(OpenEvent, topic, "")
}

// NewKeepaliveMessage is a convenience method to create a keepalive message
func NewKeepaliveMessage(topic string) *Message {
	return NewMessage(KeepaliveEvent, topic, "")
}

// NewDefaultMessage is a convenience method to create a notification message
func NewDefaultMessage(topic, msg string) *Message {
	return NewMessage(MessageEvent, topic, msg)
}

// NewActionMessage creates a new action message (message_delete or message_clear)
func NewActionMessage(event, topic, sequenceID string) *Message {
	m := NewMessage(event, topic, "")
	m.SequenceID = sequenceID
	return m
}

// NewPollRequestMessage is a convenience method to create a poll request message
func NewPollRequestMessage(topic, pollID string) *Message {
	m := NewMessage(PollRequestEvent, topic, "New message")
	m.PollID = pollID
	return m
}

// SinceMarker represents a point in time or message ID from which to retrieve messages
type SinceMarker struct {
	time time.Time
	id   string
}

// NewSinceTime creates a new SinceMarker from a Unix timestamp
func NewSinceTime(timestamp int64) SinceMarker {
	return SinceMarker{time.Unix(timestamp, 0), ""}
}

// NewSinceID creates a new SinceMarker from a message ID
func NewSinceID(id string) SinceMarker {
	return SinceMarker{time.Unix(0, 0), id}
}

// IsAll returns true if this is the "all messages" marker
func (t SinceMarker) IsAll() bool {
	return t == SinceAllMessages
}

// IsNone returns true if this is the "no messages" marker
func (t SinceMarker) IsNone() bool {
	return t == SinceNoMessages
}

// IsLatest returns true if this is the "latest message" marker
func (t SinceMarker) IsLatest() bool {
	return t == SinceLatestMessage
}

// IsID returns true if this marker references a specific message ID
func (t SinceMarker) IsID() bool {
	return t.id != "" && t.id != SinceLatestMessage.id
}

// Time returns the time component of the marker
func (t SinceMarker) Time() time.Time {
	return t.time
}

// ID returns the message ID component of the marker
func (t SinceMarker) ID() string {
	return t.id
}

// Common SinceMarker values for subscribing to messages
var (
	SinceAllMessages   = SinceMarker{time.Unix(0, 0), ""}
	SinceNoMessages    = SinceMarker{time.Unix(1, 0), ""}
	SinceLatestMessage = SinceMarker{time.Unix(0, 0), "latest"}
)
