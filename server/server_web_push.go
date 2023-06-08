package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
)

// test: https://regexr.com/7eqvl
// example urls:
//
//	https://android.googleapis.com/XYZ
//	https://fcm.googleapis.com/XYZ
//	https://updates.push.services.mozilla.com/XYZ
//	https://updates-autopush.stage.mozaws.net/XYZ
//	https://updates-autopush.dev.mozaws.net/XYZ
//	https://AAA.notify.windows.com/XYZ
//	https://AAA.push.apple.com/XYZ
const (
	webPushEndpointAllowRegexStr = `^https:\/\/(android\.googleapis\.com|fcm\.googleapis\.com|updates\.push\.services\.mozilla\.com|updates-autopush\.stage\.mozaws\.net|updates-autopush\.dev\.mozaws\.net|.*\.notify\.windows\.com|.*\.push\.apple\.com)\/.*$`
	webPushTopicSubscribeLimit   = 50
)

var webPushEndpointAllowRegex = regexp.MustCompile(webPushEndpointAllowRegexStr)

func (s *Server) handleWebPushUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	payload, err := readJSONWithLimit[webPushSubscriptionPayload](r.Body, jsonBodyBytesLimit, false)
	if err != nil || payload.BrowserSubscription.Endpoint == "" || payload.BrowserSubscription.Keys.P256dh == "" || payload.BrowserSubscription.Keys.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	} else if !webPushEndpointAllowRegex.MatchString(payload.BrowserSubscription.Endpoint) {
		return errHTTPBadRequestWebPushEndpointUnknown
	} else if len(payload.Topics) > webPushTopicSubscribeLimit {
		return errHTTPBadRequestWebPushTopicCountTooHigh
	}
	topics, err := s.topicsFromIDs(payload.Topics...)
	if err != nil {
		return err
	}
	if s.userManager != nil {
		u := v.User()
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
	payload, err := json.Marshal(newWebPushPayload(fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic), m))
	if err != nil {
		log.Tag(tagWebPush).Err(err).Warn("Unable to marshal expiring payload")
		return
	}
	for _, subscription := range subscriptions {
		ctx := log.Context{"endpoint": subscription.BrowserSubscription.Endpoint, "username": subscription.UserID, "topic": m.Topic, "message_id": m.ID}
		if err := s.sendWebPushNotification(payload, subscription, &ctx); err != nil {
			log.Tag(tagWebPush).Err(err).Fields(ctx).Warn("Unable to publish web push message")
		}
	}
}

// TODO this should return error
// TODO rate limiting

func (s *Server) expireOrNotifyOldSubscriptions() {
	subscriptions, err := s.webPush.ExpireAndGetExpiringSubscriptions(s.config.WebPushExpiryWarningDuration, s.config.WebPushExpiryDuration)
	if err != nil {
		log.Tag(tagWebPush).Err(err).Warn("Unable to publish expiry imminent warning")
		return
	} else if len(subscriptions) == 0 {
		return
	}
	payload, err := json.Marshal(newWebPushSubscriptionExpiringPayload())
	if err != nil {
		log.Tag(tagWebPush).Err(err).Warn("Unable to marshal expiring payload")
		return
	}
	go func() {
		for _, subscription := range subscriptions {
			ctx := log.Context{"endpoint": subscription.BrowserSubscription.Endpoint}
			if err := s.sendWebPushNotification(payload, &subscription, &ctx); err != nil {
				log.Tag(tagWebPush).Err(err).Fields(ctx).Warn("Unable to publish expiry imminent warning")
			}
		}
	}()
	log.Tag(tagWebPush).Debug("Expiring old subscriptions and published %d expiry imminent warnings", len(subscriptions))
}

func (s *Server) sendWebPushNotification(message []byte, sub *webPushSubscription, ctx *log.Context) error {
	resp, err := webpush.SendNotification(message, &sub.BrowserSubscription, &webpush.Options{
		Subscriber:      s.config.WebPushEmailAddress,
		VAPIDPublicKey:  s.config.WebPushPublicKey,
		VAPIDPrivateKey: s.config.WebPushPrivateKey,
		Urgency:         webpush.UrgencyHigh, // iOS requires this to ensure delivery
	})
	if err != nil {
		log.Tag(tagWebPush).Err(err).Fields(*ctx).Debug("Unable to publish web push message, removing endpoint")
		if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
			return err
		}
		return err
	}
	if (resp.StatusCode < 200 || resp.StatusCode > 299) && resp.StatusCode != 429 {
		log.Tag(tagWebPush).Fields(*ctx).Field("response_code", resp.StatusCode).Debug("Unable to publish web push message, unexpected response")
		if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
			return err
		}
		return errHTTPInternalErrorWebPushUnableToPublish.Fields(*ctx)
	}
	return nil
}
