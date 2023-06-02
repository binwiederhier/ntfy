package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
)

func (s *Server) handleWebPushUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	payload, err := readJSONWithLimit[webPushSubscriptionPayload](r.Body, jsonBodyBytesLimit, false)
	if err != nil || payload.BrowserSubscription.Endpoint == "" || payload.BrowserSubscription.Keys.P256dh == "" || payload.BrowserSubscription.Keys.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}

	u := v.User()

	topics, err := s.topicsFromIDs(payload.Topics...)
	if err != nil {
		return err
	}

	if s.userManager != nil {
		for _, t := range topics {
			if err := s.userManager.Authorize(u, t.ID, user.PermissionRead); err != nil {
				logvr(v, r).With(t).Err(err).Debug("Access to topic %s not authorized", t.ID)
				return errHTTPForbidden.With(t)
			}
		}
	}

	if err := s.webPush.UpdateSubscriptions(payload.Topics, v.MaybeUserID(), payload.BrowserSubscription); err != nil {
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
