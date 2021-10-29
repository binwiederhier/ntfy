package server

import (
	"heckel.io/ntfy/util"
	"time"
)

// List of possible events
const (
	openEvent      = "open"
	keepaliveEvent = "keepalive"
	messageEvent   = "message"
)

const (
	messageIDLength = 10
)

// message represents a message published to a topic
type message struct {
	ID      string `json:"id"`    // Random message ID
	Time    int64  `json:"time"`  // Unix time in seconds
	Event   string `json:"event"` // One of the above
	Topic   string `json:"topic"`
	Message string `json:"message,omitempty"`
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
