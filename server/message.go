package server

import "time"

// List of possible events
const (
	openEvent      = "open"
	keepaliveEvent = "keepalive"
	messageEvent = "message"
)

// message represents a message published to a topic
type message struct {
	Time    int64  `json:"time"`            // Unix time in seconds
	Event   string `json:"event,omitempty"` // One of the above
	Message string `json:"message,omitempty"`
}

// messageEncoder is a function that knows how to encode a message
type messageEncoder func(msg *message) (string, error)

// newMessage creates a new message with the current timestamp
func newMessage(event string, msg string) *message {
	return &message{
		Time:    time.Now().Unix(),
		Event:   event,
		Message: msg,
	}
}

// newOpenMessage is a convenience method to create an open message
func newOpenMessage() *message {
	return newMessage(openEvent, "")
}

// newKeepaliveMessage is a convenience method to create a keepalive message
func newKeepaliveMessage() *message {
	return newMessage(keepaliveEvent, "")
}

// newDefaultMessage is a convenience method to create a notification message
func newDefaultMessage(msg string) *message {
	return newMessage(messageEvent, msg)
}
