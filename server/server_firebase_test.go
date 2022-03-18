package server

import (
	"encoding/json"
	"errors"
	"firebase.google.com/go/messaging"
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/auth"
	"strings"
	"testing"
)

type testAuther struct {
	Allow bool
}

func (t testAuther) Authenticate(_, _ string) (*auth.User, error) {
	return nil, errors.New("not used")
}

func (t testAuther) Authorize(_ *auth.User, _ string, _ auth.Permission) error {
	if t.Allow {
		return nil
	}
	return errors.New("unauthorized")
}

func TestToFirebaseMessage_Keepalive(t *testing.T) {
	m := newKeepaliveMessage("mytopic")
	fbm, err := toFirebaseMessage(m, nil)
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Nil(t, fbm.Android)
	require.Equal(t, map[string]string{
		"id":    m.ID,
		"time":  fmt.Sprintf("%d", m.Time),
		"event": m.Event,
		"topic": m.Topic,
	}, fbm.Data)
}

func TestToFirebaseMessage_Open(t *testing.T) {
	m := newOpenMessage("mytopic")
	fbm, err := toFirebaseMessage(m, nil)
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Nil(t, fbm.Android)
	require.Equal(t, map[string]string{
		"id":    m.ID,
		"time":  fmt.Sprintf("%d", m.Time),
		"event": m.Event,
		"topic": m.Topic,
	}, fbm.Data)
}

func TestToFirebaseMessage_Message_Normal_Allowed(t *testing.T) {
	m := newDefaultMessage("mytopic", "this is a message")
	m.Priority = 4
	m.Tags = []string{"tag 1", "tag2"}
	m.Click = "https://google.com"
	m.Title = "some title"
	m.Attachment = &attachment{
		Name:    "some file.jpg",
		Type:    "image/jpeg",
		Size:    12345,
		Expires: 98765543,
		URL:     "https://example.com/file.jpg",
		Owner:   "some-owner",
	}
	fbm, err := toFirebaseMessage(m, &testAuther{Allow: true})
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Equal(t, &messaging.AndroidConfig{
		Priority: "high",
	}, fbm.Android)
	require.Equal(t, map[string]string{
		"id":                 m.ID,
		"time":               fmt.Sprintf("%d", m.Time),
		"event":              "message",
		"topic":              "mytopic",
		"priority":           "4",
		"tags":               strings.Join(m.Tags, ","),
		"click":              "https://google.com",
		"title":              "some title",
		"message":            "this is a message",
		"encoding":           "",
		"attachment_name":    "some file.jpg",
		"attachment_type":    "image/jpeg",
		"attachment_size":    "12345",
		"attachment_expires": "98765543",
		"attachment_url":     "https://example.com/file.jpg",
	}, fbm.Data)
}

func TestToFirebaseMessage_Message_Normal_Not_Allowed(t *testing.T) {
	m := newDefaultMessage("mytopic", "this is a message")
	m.Priority = 5
	fbm, err := toFirebaseMessage(m, &testAuther{Allow: false}) // Not allowed!
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Equal(t, &messaging.AndroidConfig{
		Priority: "high",
	}, fbm.Android)
	require.Equal(t, "", fbm.Data["message"])
	require.Equal(t, "", fbm.Data["priority"])
	require.Equal(t, map[string]string{
		"id":    m.ID,
		"time":  fmt.Sprintf("%d", m.Time),
		"event": "poll_request",
		"topic": "mytopic",
	}, fbm.Data)
}

func TestMaybeTruncateFCMMessage(t *testing.T) {
	origMessage := strings.Repeat("this is a long string", 300)
	origFCMMessage := &messaging.Message{
		Topic: "mytopic",
		Data: map[string]string{
			"id":       "abcdefg",
			"time":     "1641324761",
			"event":    "message",
			"topic":    "mytopic",
			"priority": "0",
			"tags":     "",
			"title":    "",
			"message":  origMessage,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
	}
	origMessageLength := len(origFCMMessage.Data["message"])
	serializedOrigFCMMessage, _ := json.Marshal(origFCMMessage)
	require.Greater(t, len(serializedOrigFCMMessage), fcmMessageLimit) // Pre-condition

	truncatedFCMMessage := maybeTruncateFCMMessage(origFCMMessage)
	truncatedMessageLength := len(truncatedFCMMessage.Data["message"])
	serializedTruncatedFCMMessage, _ := json.Marshal(truncatedFCMMessage)
	require.Equal(t, fcmMessageLimit, len(serializedTruncatedFCMMessage))
	require.Equal(t, "1", truncatedFCMMessage.Data["truncated"])
	require.NotEqual(t, origMessageLength, truncatedMessageLength)
}

func TestMaybeTruncateFCMMessage_NotTooLong(t *testing.T) {
	origMessage := "not really a long string"
	origFCMMessage := &messaging.Message{
		Topic: "mytopic",
		Data: map[string]string{
			"id":       "abcdefg",
			"time":     "1641324761",
			"event":    "message",
			"topic":    "mytopic",
			"priority": "0",
			"tags":     "",
			"title":    "",
			"message":  origMessage,
		},
	}
	origMessageLength := len(origFCMMessage.Data["message"])
	serializedOrigFCMMessage, _ := json.Marshal(origFCMMessage)
	require.LessOrEqual(t, len(serializedOrigFCMMessage), fcmMessageLimit) // Pre-condition

	notTruncatedFCMMessage := maybeTruncateFCMMessage(origFCMMessage)
	notTruncatedMessageLength := len(notTruncatedFCMMessage.Data["message"])
	serializedNotTruncatedFCMMessage, _ := json.Marshal(notTruncatedFCMMessage)
	require.Equal(t, origMessageLength, notTruncatedMessageLength)
	require.Equal(t, len(serializedOrigFCMMessage), len(serializedNotTruncatedFCMMessage))
	require.Equal(t, "", notTruncatedFCMMessage.Data["truncated"])
}
