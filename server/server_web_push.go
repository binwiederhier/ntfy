package server

import (
	"encoding/json"
	"fmt"
	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/log"
	"net/http"
)

func (s *Server) handleTopicWebPushSubscribe(w http.ResponseWriter, r *http.Request, v *visitor) error {
	sub, err := readJSONWithLimit[webPushSubscribePayload](r.Body, jsonBodyBytesLimit, false)
	if err != nil || sub.BrowserSubscription.Endpoint == "" || sub.BrowserSubscription.Keys.P256dh == "" || sub.BrowserSubscription.Keys.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}

	topic, err := fromContext[*topic](r, contextTopic)
	if err != nil {
		return err
	}
	if err = s.webPush.AddSubscription(topic.ID, v.MaybeUserID(), *sub); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleTopicWebPushUnsubscribe(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	payload, err := readJSONWithLimit[webPushUnsubscribePayload](r.Body, jsonBodyBytesLimit, false)
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
	subscriptions, err := s.webPush.SubscriptionsForTopic(m.Topic)
	if err != nil {
		logvm(v, m).Err(err).Warn("Unable to publish web push messages")
		return
	}

	for i, xi := range subscriptions {
		go func(i int, sub webPushSubscription) {
			ctx := log.Context{"endpoint": sub.BrowserSubscription.Endpoint, "username": sub.UserID, "topic": m.Topic, "message_id": m.ID}

			payload := &webPushPayload{
				SubscriptionID: fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic),
				Message:        *m,
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
				// Deliverability on iOS isn't great with lower urgency values,
				// and thus we can't really map lower ntfy priorities to lower urgency values
				Urgency: webpush.UrgencyHigh,
			})

			if err != nil {
				logvm(v, m).Err(err).Fields(ctx).Debug("Unable to publish web push message")
				if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
					logvm(v, m).Err(err).Fields(ctx).Warn("Unable to expire subscription")
				}
				return
			}

			// May want to handle at least 429 differently, but for now treat all errors the same
			if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
				logvm(v, m).Fields(ctx).Field("response", resp).Debug("Unable to publish web push message")
				if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
					logvm(v, m).Err(err).Fields(ctx).Warn("Unable to expire subscription")
				}
				return
			}
		}(i, xi)
	}
}
