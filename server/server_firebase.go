package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"google.golang.org/api/option"
	"heckel.io/ntfy/auth"
)

const (
	fcmMessageLimit         = 4000
	fcmApnsBodyMessageLimit = 100
)

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

func createFirebaseSubscriber(credentialsFile string, auther auth.Auther) (subscriber, error) {
	fb, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	msg, err := fb.Messaging(context.Background())
	if err != nil {
		return nil, err
	}
	return func(m *message) error {
		fbm, err := toFirebaseMessage(m, auther)
		if err != nil {
			return err
		}
		_, err = msg.Send(context.Background(), fbm)
		return err
	}, nil
}

func toFirebaseMessage(m *message, auther auth.Auther) (*messaging.Message, error) {
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
		// Silent notification; only 2-3 per hour are allowed; delivery not guaranteed
		// See https://developer.apple.com/documentation/usernotifications/setting_up_a_remote_notification_server/pushing_background_updates_to_your_app
		apnsData := make(map[string]interface{})
		for k, v := range data {
			apnsData[k] = v
		}
		apnsConfig = &messaging.APNSConfig{
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
	case messageEvent:
		allowForward := true
		if auther != nil {
			allowForward = auther.Authorize(nil, m.Topic, auth.PermissionRead) == nil
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
			apnsData := make(map[string]interface{})
			for k, v := range data {
				apnsData[k] = v
			}
			apnsConfig = &messaging.APNSConfig{
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
		} else {
			// If anonymous read for a topic is not allowed, we cannot send the message along
			// via Firebase. Instead, we send a "poll_request" message, asking the client to poll.
			data = map[string]string{
				"id":    m.ID,
				"time":  fmt.Sprintf("%d", m.Time),
				"event": pollRequestEvent,
				"topic": m.Topic,
			}
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
