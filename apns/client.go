package apns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

type Config struct {
	KeyFile     string
	KeyID       string
	TeamID      string
	AppBundleID string
	Sandbox     bool
}

type Client struct {
	config Config
	client *apns2.Client
}

func NewClient(config Config) (*Client, error) {
	authKey, err := token.AuthKeyFromFile(config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load APNs auth key: %w", err)
	}
	t := &token.Token{
		AuthKey: authKey,
		KeyID:   config.KeyID,
		TeamID:  config.TeamID,
	}
	var apnsClient *apns2.Client
	if config.Sandbox {
		apnsClient = apns2.NewTokenClient(t).Development()
	} else {
		apnsClient = apns2.NewTokenClient(t).Production()
	}
	return &Client{
		config: config,
		client: apnsClient,
	}, nil
}

func (c *Client) Send(deviceToken string, m *model.Message, auther user.Auther) error {
	var data map[string]string
	var isBackground bool

	switch m.Event {
	case model.KeepaliveEvent:
		return nil
	case model.PollRequestEvent:
		isBackground = true
		data = map[string]string{
			"id":          m.ID,
			"time":        fmt.Sprintf("%d", m.Time),
			"event":       m.Event,
			"topic":       m.Topic,
			"sequence_id": m.SequenceID,
		}
	case model.MessageEvent:
		msg := m
		if auther != nil {
			if err := auther.Authorize(nil, m.Topic, user.PermissionRead); err != nil {
				msg = toPollRequest(m)
				isBackground = true
			}
		}
		if isBackground {
			data = map[string]string{
				"id":          msg.ID,
				"time":        fmt.Sprintf("%d", msg.Time),
				"event":       msg.Event,
				"topic":       msg.Topic,
				"sequence_id": msg.SequenceID,
			}
		} else {
			data = map[string]string{
				"id":           msg.ID,
				"time":         fmt.Sprintf("%d", msg.Time),
				"event":        msg.Event,
				"topic":        msg.Topic,
				"sequence_id":  msg.SequenceID,
				"priority":     fmt.Sprintf("%d", msg.Priority),
				"tags":         strings.Join(msg.Tags, ","),
				"click":        msg.Click,
				"icon":         msg.Icon,
				"title":        msg.Title,
				"message":      msg.Message,
				"content_type": msg.ContentType,
				"encoding":     msg.Encoding,
			}
			if len(msg.Actions) > 0 {
				actions, err := json.Marshal(msg.Actions)
				if err == nil {
					data["actions"] = string(actions)
				}
			}
			if msg.Attachment != nil {
				data["attachment_name"] = msg.Attachment.Name
				data["attachment_type"] = msg.Attachment.Type
				data["attachment_size"] = fmt.Sprintf("%d", msg.Attachment.Size)
				data["attachment_expires"] = fmt.Sprintf("%d", msg.Attachment.Expires)
				data["attachment_url"] = msg.Attachment.URL
			}
			if msg.PollID != "" {
				data["poll_id"] = msg.PollID
			}
		}
	default:
		return nil
	}

	payload := make(map[string]any)
	apsMap := make(map[string]any)

	if isBackground {
		apsMap["content-available"] = 1
	} else {
		apsMap["mutable-content"] = 1
		alertMap := make(map[string]any)
		if m.Title != "" {
			alertMap["title"] = m.Title
		}
		alertMap["body"] = maybeTruncateAPNSBodyMessage(m.Message)
		apsMap["alert"] = alertMap
	}
	payload["aps"] = apsMap

	for k, v := range data {
		payload[k] = v
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       c.config.AppBundleID,
		Payload:     payloadBytes,
	}
	if isBackground {
		notification.PushType = apns2.PushTypeBackground
		notification.Priority = 5
	} else {
		notification.PushType = apns2.PushTypeAlert
		notification.Priority = 10
	}

	res, err := c.client.Push(notification)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("APNs push failed: status %d, reason %s", res.StatusCode, res.Reason)
	}

	return nil
}

func toPollRequest(m *model.Message) *model.Message {
	return &model.Message{
		ID:         m.ID,
		Time:       m.Time,
		Event:      model.PollRequestEvent,
		Topic:      m.Topic,
		SequenceID: m.SequenceID,
	}
}

const apnsBodyMessageLimit = 100

func maybeTruncateAPNSBodyMessage(s string) string {
	if len(s) >= apnsBodyMessageLimit {
		over := len(s) - apnsBodyMessageLimit + 3
		return s[:len(s)-over] + "..."
	}
	return s
}
