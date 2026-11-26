//go:build !nowebpush

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	vis "heckel.io/ntfy/v2/visitor" // Aliased: the server has its own visitor type
	wpush "heckel.io/ntfy/v2/webpush"
)

const (
	// WebPushAvailable is a constant used to indicate that WebPush support is available.
	// It can be disabled with the 'nowebpush' build tag.
	WebPushAvailable = true

	webPushTopicSubscribeLimit = 50

	// webPushRequestTimeout bounds a single delivery, so a slow or stalled push service
	// cannot pin connections and goroutines indefinitely
	webPushRequestTimeout = 15 * time.Second

	// webPushResponseDrainLimit caps how much of a push service response body is read before
	// closing it; draining small bodies lets the transport reuse the connection
	webPushResponseDrainLimit = 4096
)

// webPushHTTPClient is shared across deliveries (connection reuse); it is a var so tests can
// swap in a client with a shorter timeout
var webPushHTTPClient = &http.Client{
	Timeout: webPushRequestTimeout,
}

// webPushAllowedEndpointsRegexes is the host-level allow-list of web push services ntfy
// will deliver to. Each regex anchors the scheme and matches the stable service host,
// followed by the authority/path boundary "/". Instance-specific labels (e.g. the
// "wns2-<region>" prefix on Windows Notification Service hosts) are wildcarded with
// a single-label pattern ([^/]+) that cannot span into the path.
// See GHSA-w9hq-5jg7-q4j7 for why wildcarding the entire host is insufficient.
var webPushAllowedEndpointsRegexes = []*regexp.Regexp{
	regexp.MustCompile(`^https://fcm\.googleapis\.com/`),
	regexp.MustCompile(`^https://jmt17\.google\.com/`),
	regexp.MustCompile(`^https://updates\.push\.services\.mozilla\.com/`),
	regexp.MustCompile(`^https://[^/]+\.mozaws\.net/`),
	regexp.MustCompile(`^https://web\.push\.apple\.com/`),
	regexp.MustCompile(`^https://[^/]+\.notify\.windows\.com/`),
}

func webPushEndpointAllowed(endpoint string) bool {
	for _, re := range webPushAllowedEndpointsRegexes {
		if re.MatchString(endpoint) {
			return true
		}
	}
	return false
}

func (s *Server) handleWebPushUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiWebPushUpdateSubscriptionRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil || req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	} else if !webPushEndpointAllowed(req.Endpoint) {
		return errHTTPBadRequestWebPushEndpointUnknown
	} else if len(req.Topics) > webPushTopicSubscribeLimit {
		return errHTTPBadRequestWebPushTopicCountTooHigh
	}
	topics, err := s.topicsFromIDs(v, req.Topics...)
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
	// Store the masked IP, so the per-subscriber cap covers a whole prefix like all other limits
	subscriberIP := vis.Mask(v.IP(), v.env.limitsConfig)
	if err := s.webPush.UpsertSubscription(req.Endpoint, req.Auth, req.P256dh, v.MaybeUserID(), subscriberIP, req.Topics); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleWebPushDelete(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	req, err := readJSONWithLimit[apiWebPushUpdateSubscriptionRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil || req.Endpoint == "" {
		return errHTTPBadRequestWebPushSubscriptionInvalid
	}
	if err := s.webPush.RemoveSubscriptionsByEndpoint(req.Endpoint); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) publishToWebPushEndpoints(v *visitor, m *model.Message) {
	subscriptions, err := s.webPush.SubscriptionsForTopic(m.Topic)
	if err != nil {
		logvm(v, m).Err(err).With(v, m).Warn("Unable to publish web push messages")
		return
	}
	log.Tag(tagWebPush).With(v, m).Debug("Publishing web push message to %d subscribers", len(subscriptions))
	payload, err := json.Marshal(newWebPushPayload(fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic), m.ForJSON()))
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
	warningSent := make([]*wpush.Subscription, 0)
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

func (s *Server) sendWebPushNotification(sub *wpush.Subscription, message []byte, contexters ...log.Contexter) error {
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
		HTTPClient:      webPushHTTPClient,
	})
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Timeouts and network errors are temporary (the push service may be slow or down), so
		// keep the subscription; removing it would silently cut off every user of that service
		log.Tag(tagWebPush).With(sub).With(contexters...).Err(err).Debug("Unable to publish web push message, keeping endpoint")
		return err
	} else if err != nil {
		log.Tag(tagWebPush).With(sub).With(contexters...).Err(err).Debug("Unable to publish web push message, removing endpoint")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return err
	}
	defer closeWebPushResponse(resp)
	if (resp.StatusCode < 200 || resp.StatusCode > 299) && resp.StatusCode != 429 {
		log.Tag(tagWebPush).With(sub).With(contexters...).Field("response_code", resp.StatusCode).Debug("Unable to publish web push message, unexpected response")
		if err := s.webPush.RemoveSubscriptionsByEndpoint(sub.Endpoint); err != nil {
			return err
		}
		return errHTTPInternalErrorWebPushUnableToPublish.With(sub).With(contexters...)
	}
	return nil
}

// closeWebPushResponse drains (bounded) and closes the response body, so the connection is
// released and can be reused instead of leaking
func closeWebPushResponse(resp *http.Response) {
	_, _ = io.CopyN(io.Discard, resp.Body, webPushResponseDrainLimit)
	_ = resp.Body.Close()
}
