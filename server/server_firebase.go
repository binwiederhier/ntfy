package server

import (
	"context"
	"encoding/json"
	"errors"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"fmt"
	"google.golang.org/api/option"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"strings"
)

const (
	fcmMessageLimit         = 4000
	fcmApnsBodyMessageLimit = 100
)

var (
	errFirebaseQuotaExceeded     = errors.New("quota exceeded for Firebase messages to topic")
	errFirebaseTemporarilyBanned = errors.New("visitor temporarily banned from using Firebase")
)

// firebaseClient is a generic client that formats and sends messages to Firebase.
// The actual Firebase implementation is implemented in firebaseSenderImpl, to make it testable.
type firebaseClient struct {
	sender firebaseSender
	auther user.Auther
}

func newFirebaseClient(sender firebaseSender, auther user.Auther) *firebaseClient {
	return &firebaseClient{
		sender: sender,
		auther: auther,
	}
}

func (c *firebaseClient) Send(v *visitor, m *message) error {
	if err := v.FirebaseAllowed(); err != nil {
		return errFirebaseTemporarilyBanned
	}
	fbm, err := toFirebaseMessage(m, c.auther)
	if err != nil {
		return err
	}
	if log.Tag(tagFirebase).IsTrace() {
		logvm(v, m).
			Tag(tagFirebase).
			Field("firebase_message", util.MaybeMarshalJSON(fbm)).
			Trace("Firebase message")
	}
	err = c.sender.Send(fbm)
	if err == errFirebaseQuotaExceeded {
		logvm(v, m).
			Tag(tagFirebase).
			Warn("Firebase quota exceeded (likely for topic), temporarily denying Firebase access to visitor")
		v.FirebaseTemporarilyDeny()
	}
	return err
}

// firebaseSender is an interface that represents a client that can send to Firebase Cloud Messaging.
// In tests, this can be implemented with a mock.
type firebaseSender interface {
	// Send sends a message to Firebase, or returns an error. It returns errFirebaseQuotaExceeded
	// if a rate limit has reached.
	Send(m *messaging.Message) error
}

// firebaseSenderImpl is a firebaseSender that actually talks to Firebase
type firebaseSenderImpl struct {
	client *messaging.Client
}

func newFirebaseSender(credentialsFile string) (*firebaseSenderImpl, error) {
	fb, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	client, err := fb.Messaging(context.Background())
	if err != nil {
		return nil, err
	}
	return &firebaseSenderImpl{
		client: client,
	}, nil
}

func (c *firebaseSenderImpl) Send(m *messaging.Message) error {
	_, err := c.client.Send(context.Background(), m)
	if err != nil && messaging.IsQuotaExceeded(err) {
		return errFirebaseQuotaExceeded
	}
	return err
}

// toFirebaseMessage converts a message to a Firebase message.
//
// Normal messages ("message"):
//   - For Android, we can receive data messages from Firebase and process them as code, so we just send all fields
//     in the "data" attribute. In the Android app, we then turn those into a notification and display it.
//   - On iOS, we are not allowed to receive data-only messages, so we build messages with an "alert" (with title and
//     message), and still send the rest of the data along in the "aps" attribute. We can then locally modify the
//     message in the Notification Service Extension.
//
// Keepalive messages ("keepalive"):
//   - On Android, we subscribe to the "~control" topic, which is used to restart the foreground service (if it died,
//     e.g. after an app update). We send these keepalive messages regularly (see Config.FirebaseKeepaliveInterval).
//   - On iOS, we subscribe to the "~poll" topic, which is used to poll all topics regularly. This is because iOS
//     does not allow any background or scheduled activity at all.
//
// Poll request messages ("poll_request"):
//   - Normal messages are turned into poll request messages if anonymous users are not allowed to read the message.
//     On Android, this will trigger the app to poll the topic and thereby displaying new messages.
//   - If UpstreamBaseURL is set, messages are forwarded as poll requests to an upstream server and then forwarded
//     to Firebase here. This is mainly for iOS to support self-hosted servers.
func toFirebaseMessage(m *message, auther user.Auther) (*messaging.Message, error) {
	var data map[string]string // Mostly matches https://ntfy.sh/docs/subscribe/api/#json-message-format
	var apnsConfig *messaging.APNSConfig
	switch m.Event {
	case keepaliveEvent, openEvent:
		data = map[string]string{
			"id":    m.ID,
			"time":  fmt.Sprintf("%d", m.Time),
			"event": m.Event,
			"topic": m.Topic,
		}
		apnsConfig = createAPNSBackgroundConfig(data)
	case pollRequestEvent:
		data = map[string]string{
			"id":      m.ID,
			"time":    fmt.Sprintf("%d", m.Time),
			"event":   m.Event,
			"topic":   m.Topic,
			"message": m.Message,
			"poll_id": m.PollID,
		}
		apnsConfig = createAPNSAlertConfig(m, data)
	case messageEvent:
		allowForward := true
		if auther != nil {
			allowForward = auther.Authorize(nil, m.Topic, user.PermissionRead) == nil
		}
		if allowForward {
			data = map[string]string{
				"id":       m.ID,
				"time":     fmt.Sprintf("%d", m.Time),
				"event":    m.Event,
				"topic":    m.Topic,
				"priority": fmt.Sprintf("%d", m.Priority),
				"tags":     strings.Join(m.Tags, ","),
				"click":    m.Click,
				"icon":     m.Icon,
				"title":    m.Title,
				"message":  m.Message,
				"encoding": m.Encoding,
			}
			if len(m.Actions) > 0 {
				actions, err := json.Marshal(m.Actions)
				if err != nil {
					return nil, err
				}
				data["actions"] = string(actions)
			}
			if m.Attachment != nil {
				data["attachment_name"] = m.Attachment.Name
				data["attachment_type"] = m.Attachment.Type
				data["attachment_size"] = fmt.Sprintf("%d", m.Attachment.Size)
				data["attachment_expires"] = fmt.Sprintf("%d", m.Attachment.Expires)
				data["attachment_url"] = m.Attachment.URL
			}
			apnsConfig = createAPNSAlertConfig(m, data)
		} else {
			// If anonymous read for a topic is not allowed, we cannot send the message along
			// via Firebase. Instead, we send a "poll_request" message, asking the client to poll.
			data = map[string]string{
				"id":    m.ID,
				"time":  fmt.Sprintf("%d", m.Time),
				"event": pollRequestEvent,
				"topic": m.Topic,
			}
			// TODO Handle APNS?
		}
	}
	var androidConfig *messaging.AndroidConfig
	if m.Priority >= 4 {
		androidConfig = &messaging.AndroidConfig{
			Priority: "high",
		}
	}
	return maybeTruncateFCMMessage(&messaging.Message{
		Topic:   m.Topic,
		Data:    data,
		Android: androidConfig,
		APNS:    apnsConfig,
	}), nil
}

// maybeTruncateFCMMessage performs best-effort truncation of FCM messages.
// The docs say the limit is 4000 characters, but during testing it wasn't quite clear
// what fields matter; so we're just capping the serialized JSON to 4000 bytes.
func maybeTruncateFCMMessage(m *messaging.Message) *messaging.Message {
	s, err := json.Marshal(m)
	if err != nil {
		return m
	}
	if len(s) > fcmMessageLimit {
		over := len(s) - fcmMessageLimit + 16 // = len("truncated":"1",), sigh ...
		message, ok := m.Data["message"]
		if ok && len(message) > over {
			m.Data["truncated"] = "1"
			m.Data["message"] = message[:len(message)-over]
		}
	}
	return m
}

// createAPNSAlertConfig creates an APNS config for iOS notifications that show up as an alert (only relevant for iOS).
// We must set the Alert struct ("alert"), and we need to set MutableContent ("mutable-content"), so the Notification Service
// Extension in iOS can modify the message.
func createAPNSAlertConfig(m *message, data map[string]string) *messaging.APNSConfig {
	apnsData := make(map[string]any)
	for k, v := range data {
		apnsData[k] = v
	}
	return &messaging.APNSConfig{
		Payload: &messaging.APNSPayload{
			CustomData: apnsData,
			Aps: &messaging.Aps{
				MutableContent: true,
				Alert: &messaging.ApsAlert{
					Title: m.Title,
					Body:  maybeTruncateAPNSBodyMessage(m.Message),
				},
			},
		},
	}
}

// createAPNSBackgroundConfig creates an APNS config for a silent background message (only relevant for iOS). Apple only
// allows us to send 2-3 of these notifications per hour, and delivery not guaranteed. We use this only for the ~poll
// topic, which triggers the iOS app to poll all topics for changes.
//
// See https://developer.apple.com/documentation/usernotifications/setting_up_a_remote_notification_server/pushing_background_updates_to_your_app
func createAPNSBackgroundConfig(data map[string]string) *messaging.APNSConfig {
	apnsData := make(map[string]any)
	for k, v := range data {
		apnsData[k] = v
	}
	return &messaging.APNSConfig{
		Headers: map[string]string{
			"apns-push-type": "background",
			"apns-priority":  "5",
		},
		Payload: &messaging.APNSPayload{
			Aps: &messaging.Aps{
				ContentAvailable: true,
			},
			CustomData: apnsData,
		},
	}
}

// maybeTruncateAPNSBodyMessage truncates the body for APNS.
//
// The "body" of the push notification can contain the entire message, which would count doubly for the overall length
// of the APNS payload. I set a limit of 100 characters before truncating the notification "body" with ellipsis.
// The message would not be changed (unless truncated for being too long). Note: if the payload is too large (>4KB),
// APNS will simply reject / discard the notification, meaning it will never arrive on the iOS device.
func maybeTruncateAPNSBodyMessage(s string) string {
	if len(s) >= fcmApnsBodyMessageLimit {
		over := len(s) - fcmApnsBodyMessageLimit + 3 // len("...")
		return s[:len(s)-over] + "..."
	}
	return s
}
