//go:build !nofirebase

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/stretchr/testify/require"
)

type testAuther struct {
	Allow bool
}

var _ user.Auther = (*testAuther)(nil)

func (t testAuther) Authenticate(_, _ string) (*user.User, error) {
	return nil, errors.New("not used")
}

func (t testAuther) Authorize(_ *user.User, _ string, _ user.Permission) error {
	if t.Allow {
		return nil
	}
	return errors.New("unauthorized")
}

type testFirebaseSender struct {
	allowed  int
	messages []*messaging.Message
	onSend   func(m *messaging.Message) // Called before a message is accepted, if set
	mu       sync.Mutex
}

func newTestFirebaseSender(allowed int) *testFirebaseSender {
	return &testFirebaseSender{
		allowed:  allowed,
		messages: make([]*messaging.Message, 0),
	}
}

func (s *testFirebaseSender) Send(m *messaging.Message) error {
	if s.onSend != nil {
		s.onSend(m)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages)+1 > s.allowed {
		return errFirebaseQuotaExceeded
	}
	s.messages = append(s.messages, m)
	return nil
}

func (s *testFirebaseSender) Messages() []*messaging.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(make([]*messaging.Message, 0), s.messages...)
}

func TestToFirebaseMessage_Keepalive(t *testing.T) {
	m := model.NewKeepaliveMessage("mytopic")
	fbm, err := toFirebaseMessage(m, nil)
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Nil(t, fbm.Android)
	require.Equal(t, &messaging.APNSConfig{
		Headers: map[string]string{
			"apns-push-type": "background",
			"apns-priority":  "5",
		},
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				ContentAvailable: true,
			},
			CustomData: map[string]any{
				"id":    m.ID,
				"time":  fmt.Sprintf("%d", m.Time),
				"event": m.Event,
				"topic": m.Topic,
			},
		},
	}, fbm.APNS)
	require.Equal(t, map[string]string{
		"id":    m.ID,
		"time":  fmt.Sprintf("%d", m.Time),
		"event": m.Event,
		"topic": m.Topic,
	}, fbm.Data)
}

func TestToFirebaseMessage_Open(t *testing.T) {
	m := model.NewOpenMessage("mytopic")
	fbm, err := toFirebaseMessage(m, nil)
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Nil(t, fbm.Android)
	require.Equal(t, &messaging.APNSConfig{
		Headers: map[string]string{
			"apns-push-type": "background",
			"apns-priority":  "5",
		},
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				ContentAvailable: true,
			},
			CustomData: map[string]any{
				"id":    m.ID,
				"time":  fmt.Sprintf("%d", m.Time),
				"event": m.Event,
				"topic": m.Topic,
			},
		},
	}, fbm.APNS)
	require.Equal(t, map[string]string{
		"id":    m.ID,
		"time":  fmt.Sprintf("%d", m.Time),
		"event": m.Event,
		"topic": m.Topic,
	}, fbm.Data)
}

func TestToFirebaseMessage_Message_Normal_Allowed(t *testing.T) {
	m := model.NewDefaultMessage("mytopic", "this is a message")
	m.Priority = 4
	m.Tags = []string{"tag 1", "tag2"}
	m.Click = "https://google.com"
	m.Icon = "https://ntfy.sh/static/img/ntfy.png"
	m.Title = "some title"
	m.Actions = []*model.Action{
		{
			ID:     "123",
			Action: "view",
			Label:  "Open page",
			Clear:  true,
			URL:    "https://ntfy.sh",
		},
		{
			ID:     "456",
			Action: "http",
			Label:  "Close door",
			URL:    "https://door.com/close",
			Method: "PUT",
			Headers: map[string]string{
				"really": "yes",
			},
		},
	}
	m.Attachment = &model.Attachment{
		Name:    "some file.jpg",
		Type:    "image/jpeg",
		Size:    12345,
		Expires: 98765543,
		URL:     "https://example.com/file.jpg",
	}
	fbm, err := toFirebaseMessage(m, &testAuther{Allow: true})
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Equal(t, &messaging.AndroidConfig{
		Priority: "high",
	}, fbm.Android)
	require.Equal(t, &messaging.APNSConfig{
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				MutableContent: true,
				Alert: &messaging.ApsAlert{
					Title: "some title",
					Body:  "this is a message",
				},
			},
			CustomData: map[string]any{
				"id":                 m.ID,
				"time":               fmt.Sprintf("%d", m.Time),
				"event":              "message",
				"topic":              "mytopic",
				"sequence_id":        "",
				"priority":           "4",
				"tags":               strings.Join(m.Tags, ","),
				"click":              "https://google.com",
				"icon":               "https://ntfy.sh/static/img/ntfy.png",
				"title":              "some title",
				"message":            "this is a message",
				"actions":            `[{"id":"123","action":"view","label":"Open page","clear":true,"url":"https://ntfy.sh"},{"id":"456","action":"http","label":"Close door","clear":false,"url":"https://door.com/close","method":"PUT","headers":{"really":"yes"}}]`,
				"content_type":       "",
				"encoding":           "",
				"attachment_name":    "some file.jpg",
				"attachment_type":    "image/jpeg",
				"attachment_size":    "12345",
				"attachment_expires": "98765543",
				"attachment_url":     "https://example.com/file.jpg",
			},
		},
	}, fbm.APNS)
	require.Equal(t, map[string]string{
		"id":                 m.ID,
		"time":               fmt.Sprintf("%d", m.Time),
		"event":              "message",
		"topic":              "mytopic",
		"sequence_id":        "",
		"priority":           "4",
		"tags":               strings.Join(m.Tags, ","),
		"click":              "https://google.com",
		"icon":               "https://ntfy.sh/static/img/ntfy.png",
		"title":              "some title",
		"message":            "this is a message",
		"actions":            `[{"id":"123","action":"view","label":"Open page","clear":true,"url":"https://ntfy.sh"},{"id":"456","action":"http","label":"Close door","clear":false,"url":"https://door.com/close","method":"PUT","headers":{"really":"yes"}}]`,
		"content_type":       "",
		"encoding":           "",
		"attachment_name":    "some file.jpg",
		"attachment_type":    "image/jpeg",
		"attachment_size":    "12345",
		"attachment_expires": "98765543",
		"attachment_url":     "https://example.com/file.jpg",
	}, fbm.Data)
}

func TestToFirebaseMessage_Message_Normal_Not_Allowed(t *testing.T) {
	m := model.NewDefaultMessage("mytopic", "this is a message")
	m.Priority = 5
	fbm, err := toFirebaseMessage(m, &testAuther{Allow: false}) // Not allowed!
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Equal(t, &messaging.AndroidConfig{
		Priority: "high",
	}, fbm.Android)
	require.Equal(t, "New message", fbm.Data["message"])
	require.Equal(t, "5", fbm.Data["priority"])
	require.Equal(t, map[string]string{
		"id":           m.ID,
		"time":         fmt.Sprintf("%d", m.Time),
		"event":        "poll_request",
		"topic":        "mytopic",
		"sequence_id":  "",
		"message":      "New message",
		"title":        "",
		"tags":         "",
		"click":        "",
		"icon":         "",
		"priority":     "5",
		"encoding":     "",
		"content_type": "",
		"poll_id":      m.ID,
	}, fbm.Data)
	require.Equal(t, "", fbm.APNS.Payload.Aps.Alert.Title)
	require.Equal(t, "New message", fbm.APNS.Payload.Aps.Alert.Body)
}

func TestToFirebaseMessage_PollRequest(t *testing.T) {
	m := model.NewPollRequestMessage("mytopic", "fOv6k1QbCzo6")
	fbm, err := toFirebaseMessage(m, nil)
	require.Nil(t, err)
	require.Equal(t, "mytopic", fbm.Topic)
	require.Nil(t, fbm.Android)
	require.Equal(t, &messaging.APNSConfig{
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				MutableContent: true,
				Alert: &messaging.ApsAlert{
					Title: "",
					Body:  "New message",
				},
			},
			CustomData: map[string]any{
				"id":      m.ID,
				"time":    fmt.Sprintf("%d", m.Time),
				"event":   "poll_request",
				"topic":   "mytopic",
				"message": "New message",
				"poll_id": "fOv6k1QbCzo6",
			},
		},
	}, fbm.APNS)
	require.Equal(t, map[string]string{
		"id":      m.ID,
		"time":    fmt.Sprintf("%d", m.Time),
		"event":   "poll_request",
		"topic":   "mytopic",
		"message": "New message",
		"poll_id": "fOv6k1QbCzo6",
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

func TestToFirebaseSender_Abuse(t *testing.T) {
	sender := &testFirebaseSender{allowed: 2}
	client := newFirebaseClient(sender, &testAuther{})
	visitor := newVisitor(newVisitorEnv(newTestConfig(t, ""), limit.New(&limit.Config{}), newMemTestCache(t), nil), netip.MustParseAddr("1.2.3.4"), nil)

	require.Nil(t, client.Send(visitor, &model.Message{Topic: "mytopic"}, nil))
	require.Equal(t, 1, len(sender.Messages()))

	require.Nil(t, client.Send(visitor, &model.Message{Topic: "mytopic"}, nil))
	require.Equal(t, 2, len(sender.Messages()))

	require.Equal(t, errFirebaseQuotaExceeded, client.Send(visitor, &model.Message{Topic: "mytopic"}, nil))
	require.Equal(t, 2, len(sender.Messages()))

	sender.messages = make([]*messaging.Message, 0) // Reset to test that time limit is working
	require.Equal(t, errFirebaseTemporarilyBanned, client.Send(visitor, &model.Message{Topic: "mytopic"}, nil))
	require.Equal(t, 0, len(sender.Messages()))
}

func TestServer_PublishWithFirebase_PollRequestSentAfterMessageIsWritten(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// A poll_request makes the phone poll for the message right away, so it must not be sent
		// before the batch writer stored the message, or the poll comes back empty
		c := newTestConfig(t, databaseURL)
		c.CacheBatchSize = 100
		c.CacheBatchTimeout = time.Second
		s := newTestServer(t, c)
		var written atomic.Bool
		sender := newTestFirebaseSender(10)
		sender.onSend = func(m *messaging.Message) {
			_, err := s.messageCache.Message(m.Data["id"])
			written.Store(err == nil)
		}
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: false}) // Protected topic -> poll_request

		response := request(t, s, "PUT", "/mytopic", "my protected message", nil)
		require.Equal(t, 200, response.Code)
		waitFor(t, func() bool {
			return len(sender.Messages()) == 1
		})
		require.Equal(t, model.PollRequestEvent, sender.Messages()[0].Data["event"])
		require.True(t, written.Load())
	})
}

func TestServer_PublishWithFirebase_MessageNotDelayedByBatchWrite(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		// A normal message carries its content, so it is sent right away, without waiting for the batch writer
		c := newTestConfig(t, databaseURL)
		c.CacheBatchSize = 100
		c.CacheBatchTimeout = 3 * time.Second
		s := newTestServer(t, c)
		sender := newTestFirebaseSender(10)
		s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true})

		response := request(t, s, "PUT", "/mytopic", "my message", nil)
		require.Equal(t, 200, response.Code)
		waitForWithMaxWait(t, time.Second, func() bool {
			return len(sender.Messages()) == 1
		})
		require.Equal(t, model.MessageEvent, sender.Messages()[0].Data["event"])
	})
}
