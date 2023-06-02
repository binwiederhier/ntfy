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
	}

	if !webPushEndpointAllowRegex.MatchString(payload.BrowserSubscription.Endpoint) {
		return errHTTPBadRequestWebPushEndpointUnknown
	}

	if len(payload.Topics) > webPushTopicSubscribeLimit {
		return errHTTPBadRequestWebPushTopicCountTooHigh
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

			s.sendWebPushNotification(newWebPushPayload(fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic), *m), &sub, &ctx)
		}(i, xi)
	}
}

func (s *Server) expireOrNotifyOldSubscriptions() {
	subscriptions, err := s.webPush.ExpireAndGetExpiringSubscriptions(s.config.WebPushExpiryWarningDuration, s.config.WebPushExpiryDuration)
	if err != nil {
		log.Tag(tagWebPush).Err(err).Warn("Unable to publish expiry imminent warning")

		return
	}

	for i, xi := range subscriptions {
		go func(i int, sub webPushSubscription) {
			ctx := log.Context{"endpoint": sub.BrowserSubscription.Endpoint}

			s.sendWebPushNotification(newWebPushSubscriptionExpiringPayload(), &sub, &ctx)
		}(i, xi)
	}

	log.Tag(tagWebPush).Debug("Expired old subscriptions and published %d expiry imminent warnings", len(subscriptions))
}

func (s *Server) sendWebPushNotification(payload any, sub *webPushSubscription, ctx *log.Context) {
	jsonPayload, err := json.Marshal(payload)

	if err != nil {
		log.Tag(tagWebPush).Err(err).Fields(*ctx).Debug("Unable to publish web push message")
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
		log.Tag(tagWebPush).Err(err).Fields(*ctx).Debug("Unable to publish web push message")
		if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
			log.Tag(tagWebPush).Err(err).Fields(*ctx).Warn("Unable to expire subscription")
		}
		return
	}

	// May want to handle at least 429 differently, but for now treat all errors the same
	if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
		log.Tag(tagWebPush).Fields(*ctx).Field("response", resp).Debug("Unable to publish web push message")
		if err := s.webPush.RemoveByEndpoint(sub.BrowserSubscription.Endpoint); err != nil {
			log.Tag(tagWebPush).Err(err).Fields(*ctx).Warn("Unable to expire subscription")
		}
		return
	}
}
