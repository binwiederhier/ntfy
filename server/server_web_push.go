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
	req, err := readJSONWithLimit[apiWebPushUpdateSubscriptionRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil || req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	} else if !webPushEndpointAllowRegex.MatchString(req.Endpoint) {
		return errHTTPBadRequestWebPushEndpointUnknown
	} else if len(req.Topics) > webPushTopicSubscribeLimit {
		return errHTTPBadRequestWebPushTopicCountTooHigh
	}
	topics, err := s.topicsFromIDs(req.Topics...)
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
	if err := s.webPush.UpsertSubscription(req.Endpoint, req.Auth, req.P256dh, v.MaybeUserID(), req.Topics); err != nil {
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
		ctx := log.Context{"endpoint": subscription.Endpoint, "username": subscription.UserID, "topic": m.Topic, "message_id": m.ID}
		if err := s.sendWebPushNotification(payload, subscription, &ctx); err != nil {
			log.Tag(tagWebPush).Err(err).Fields(ctx).Warn("Unable to publish web push message")
		}
	}
}

func (s *Server) pruneAndNotifyWebPushSubscriptions() {
	if s.config.WebPushPublicKey == "" {
		return
	}
	go func() {
		if err := s.pruneAndNotifyWebPushSubscriptionsInternal(); err != nil {
			log.Tag(tagWebPush).Err(err).Warn("Unable to prune or notify web push subscriptions")
		}
	}()
}

func (s *Server) pruneAndNotifyWebPushSubscriptionsInternal() error {
	// Expire old subscriptions
	if err := s.webPush.RemoveExpiredSubscriptions(s.config.WebPushExpiryDuration); err != nil {
		return err
	}
	// Notify subscriptions that will expire soon
	subscriptions, err := s.webPush.SubscriptionsExpiring(s.config.WebPushExpiryWarningDuration)
	if err != nil {
		return err
	} else if len(subscriptions) == 0 {
		return nil
	}
	payload, err := json.Marshal(newWebPushSubscriptionExpiringPayload())
	if err != nil {
		log.Tag(tagWebPush).Err(err).Warn("Unable to marshal expiring payload")
		return err
	}
	warningSent := make([]*webPushSubscription, 0)
	for _, subscription := range subscriptions {
		ctx := log.Context{"endpoint": subscription.Endpoint}
		if err := s.sendWebPushNotification(payload, subscription, &ctx); err != nil {
			log.Tag(tagWebPush).Err(err).Fields(ctx).Warn("Unable to publish expiry imminent warning")
			continue
		}
		warningSent = append(warningSent, subscription)
	}
	if err := s.webPush.MarkExpiryWarningSent(warningSent); err != nil {
		return err
	}
	log.Tag(tagWebPush).Debug("Expired old subscriptions and published %d expiry imminent warnings", len(subscriptions))
	return nil
}

func (s *Server) sendWebPushNotification(message []byte, sub *webPushSubscription, ctx *log.Context) error {
	resp, err := webpush.SendNotification(message, sub.ToSubscription(), &webpush.Options{
		Subscriber:      s.config.WebPushEmailAddress,
		VAPIDPublicKey:  s.config.WebPushPublicKey,
		VAPIDPrivateKey: s.config.WebPushPrivateKey,
		Urgency:         webpush.UrgencyHigh, // iOS requires this to ensure delivery
		TTL:             int(s.config.CacheDuration.Seconds()),
	})
	if err != nil {
		log.Tag(tagWebPush).Err(err).Fields(*ctx).Debug("Unable to publish web push message, removing endpoint")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return err
	}
	if (resp.StatusCode < 200 || resp.StatusCode > 299) && resp.StatusCode != 429 {
		log.Tag(tagWebPush).Fields(*ctx).Field("response_code", resp.StatusCode).Debug("Unable to publish web push message, unexpected response")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return errHTTPInternalErrorWebPushUnableToPublish.Fields(*ctx)
	}
	return nil
}
