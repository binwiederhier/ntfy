package server

import (
	"encoding/json"
	"fmt"
	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/log"
	"net/http"
	"strings"
)

func (s *Server) handleTopicWebPushSubscribe(w http.ResponseWriter, r *http.Request, v *visitor) error {
	var username string
	u := v.User()
	if u != nil {
		username = u.Name
	}

	var sub webPushSubscribePayload
	err := json.NewDecoder(r.Body).Decode(&sub)

	if err != nil || sub.BrowserSubscription.Endpoint == "" || sub.BrowserSubscription.Keys.P256dh == "" || sub.BrowserSubscription.Keys.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}

	topic, err := fromContext[*topic](r, contextTopic)
	if err != nil {
		return err
	}

	err = s.webPush.AddSubscription(topic.ID, username, sub)
	if err != nil {
		return err
	}

	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleTopicWebPushUnsubscribe(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	var payload webPushUnsubscribePayload

	err := json.NewDecoder(r.Body).Decode(&payload)

	if err != nil {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}

	topic, err := fromContext[*topic](r, contextTopic)
	if err != nil {
		return err
	}

	err = s.webPush.RemoveSubscription(topic.ID, payload.Endpoint)
	if err != nil {
		return err
	}

	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) publishToWebPushEndpoints(v *visitor, m *message) {
	subscriptions, err := s.webPush.GetSubscriptionsForTopic(m.Topic)
	if err != nil {
		logvm(v, m).Err(err).Warn("Unable to publish web push messages")
		return
	}

	ctx := log.Context{"topic": m.Topic, "message_id": m.ID, "total_count": len(subscriptions)}

	// Importing the emojis in the service worker would add unnecessary complexity,
	// simply do it here for web push notifications instead
	var titleWithDefault string
	var formattedTitle string

	emojis, _, err := toEmojis(m.Tags)
	if err != nil {
		logvm(v, m).Err(err).Fields(ctx).Debug("Unable to publish web push message")
		return
	}

	if m.Title == "" {
		titleWithDefault = m.Topic
	} else {
		titleWithDefault = m.Title
	}

	if len(emojis) > 0 {
		formattedTitle = fmt.Sprintf("%s %s", strings.Join(emojis[:], " "), titleWithDefault)
	} else {
		formattedTitle = titleWithDefault
	}

	for i, xi := range subscriptions {
		go func(i int, sub webPushSubscription) {
			ctx := log.Context{"endpoint": sub.BrowserSubscription.Endpoint, "username": sub.Username, "topic": m.Topic, "message_id": m.ID}

			payload := &webPushPayload{
				SubscriptionID: fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic),
				Message:        *m,
				FormattedTitle: formattedTitle,
			}
			jsonPayload, err := json.Marshal(payload)

			if err != nil {
				logvm(v, m).Err(err).Fields(ctx).Debug("Unable to publish web push message")
				return
			}

			resp, err := webpush.SendNotification(jsonPayload, &sub.BrowserSubscription, &webpush.Options{
				Subscriber:      s.config.WebPushEmailAddress,
				VAPIDPublicKey:  s.config.WebPushPublicKey,
				VAPIDPrivateKey: s.config.WebPushPrivateKey,
				// deliverability on iOS isn't great with lower urgency values,
				// and thus we can't really map lower ntfy priorities to lower urgency values
				Urgency: webpush.UrgencyHigh,
			})

			if err != nil {
				logvm(v, m).Err(err).Fields(ctx).Debug("Unable to publish web push message")

				err = s.webPush.ExpireWebPushEndpoint(sub.BrowserSubscription.Endpoint)
				if err != nil {
					logvm(v, m).Err(err).Fields(ctx).Warn("Unable to expire subscription")
				}

				return
			}

			// May want to handle at least 429 differently, but for now treat all errors the same
			if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
				logvm(v, m).Fields(ctx).Field("response", resp).Debug("Unable to publish web push message")

				err = s.webPush.ExpireWebPushEndpoint(sub.BrowserSubscription.Endpoint)
				if err != nil {
					logvm(v, m).Err(err).Fields(ctx).Warn("Unable to expire subscription")
				}

				return
			}
		}(i, xi)
	}
}
