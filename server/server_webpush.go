package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

const (
	webPushTopicSubscribeLimit = 50
)

var (
	webPushAllowedEndpointsPatterns = []string{
		"https://*.google.com/",
		"https://*.googleapis.com/",
		"https://*.mozilla.com/",
		"https://*.mozaws.net/",
		"https://*.windows.com/",
		"https://*.microsoft.com/",
		"https://*.apple.com/",
	}
	webPushAllowedEndpointsRegex *regexp.Regexp
)

func init() {
	for i, pattern := range webPushAllowedEndpointsPatterns {
		webPushAllowedEndpointsPatterns[i] = strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "\\."), "*", ".+")
	}
	allPatterns := fmt.Sprintf("^(%s)", strings.Join(webPushAllowedEndpointsPatterns, "|"))
	webPushAllowedEndpointsRegex = regexp.MustCompile(allPatterns)
}

func (s *Server) handleWebPushUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiWebPushUpdateSubscriptionRequest](r.Body, httpBodyBytesLimit, false)
	if err != nil || req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	} else if !webPushAllowedEndpointsRegex.MatchString(req.Endpoint) {
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
	if err := s.webPush.UpsertSubscription(req.Endpoint, req.Auth, req.P256dh, v.MaybeUserID(), v.IP(), req.Topics); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleWebPushDelete(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	req, err := readJSONWithLimit[apiWebPushUpdateSubscriptionRequest](r.Body, httpBodyBytesLimit, false)
	if err != nil || req.Endpoint == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}
	if err := s.webPush.RemoveSubscriptionsByEndpoint(req.Endpoint); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) publishToWebPushEndpoints(v *visitor, m *message) {
	subscriptions, err := s.webPush.SubscriptionsForTopic(m.Topic)
	if err != nil {
		logvm(v, m).Err(err).With(v, m).Warn("Unable to publish web push messages")
		return
	}
	log.Tag(tagWebPush).With(v, m).Debug("Publishing web push message to %d subscribers", len(subscriptions))
	payload, err := json.Marshal(newWebPushPayload(fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic), m))
	if err != nil {
		log.Tag(tagWebPush).Err(err).With(v, m).Warn("Unable to marshal expiring payload")
		return
	}
	for _, subscription := range subscriptions {
		if err := s.sendWebPushNotification(subscription, payload, v, m); err != nil {
			log.Tag(tagWebPush).Err(err).With(v, m, subscription).Warn("Unable to publish web push message")
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
		return err
	}
	warningSent := make([]*webPushSubscription, 0)
	for _, subscription := range subscriptions {
		if err := s.sendWebPushNotification(subscription, payload); err != nil {
			log.Tag(tagWebPush).Err(err).With(subscription).Warn("Unable to publish expiry imminent warning")
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

func (s *Server) sendWebPushNotification(sub *webPushSubscription, message []byte, contexters ...log.Contexter) error {
	log.Tag(tagWebPush).With(sub).With(contexters...).Debug("Sending web push message")
	payload := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Auth,
			P256dh: sub.P256dh,
		},
	}
	resp, err := webpush.SendNotification(message, payload, &webpush.Options{
		Subscriber:      s.config.WebPushEmailAddress,
		VAPIDPublicKey:  s.config.WebPushPublicKey,
		VAPIDPrivateKey: s.config.WebPushPrivateKey,
		Urgency:         webpush.UrgencyHigh, // iOS requires this to ensure delivery
		TTL:             int(s.config.CacheDuration.Seconds()),
	})
	if err != nil {
		log.Tag(tagWebPush).With(sub).With(contexters...).Err(err).Debug("Unable to publish web push message, removing endpoint")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return err
	}
	if (resp.StatusCode < 200 || resp.StatusCode > 299) && resp.StatusCode != 429 {
		log.Tag(tagWebPush).With(sub).With(contexters...).Field("response_code", resp.StatusCode).Debug("Unable to publish web push message, unexpected response")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return errHTTPInternalErrorWebPushUnableToPublish.With(sub).With(contexters...)
	}
	return nil
}
