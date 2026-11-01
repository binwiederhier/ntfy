package server

import (
	"fmt"
	"math"
	"net/http"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

type contextKey int

const (
	contextRateVisitor contextKey = iota + 2586
	contextTopic
	contextMatrixPushKey
	contextVisitorIP   // Client IP extracted in maybeAuthenticate; reused by the abuse ban-feed (see ban.Service.Record)
	contextAdminSocket // Set (to true) on requests that arrived over the admin socket, see isAdminSocketRequest
)

// Rate limit headers of draft-ietf-httpapi-ratelimit-headers-11, sent on publishes
const (
	headerRateLimitPolicy = "RateLimit-Policy"
	headerRateLimit       = "RateLimit"
)

func (s *Server) limitRequests(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if util.ContainsIP(s.config.VisitorRequestExemptPrefixes, v.ip) {
			return next(w, r, v)
		} else if d := v.RequestAllowed(); !d.OK {
			return errHTTPTooManyRequestsLimitRequests.RetryAfter(d.RetryAfter)
		}
		return next(w, r, v)
	}
}

// limitRequestsWithTopic limits requests with a topic and stores the rate-limiting-subscriber and topic into request.Context
func (s *Server) limitRequestsWithTopic(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		t, err := s.topicFromPath(v, r.URL.Path)
		if err != nil {
			return err
		}
		vrate := v
		if rateVisitor := t.RateVisitor(); rateVisitor != nil {
			vrate = rateVisitor
		}
		r = withContext(r, map[contextKey]any{
			contextRateVisitor: vrate,
			contextTopic:       t,
		})
		if util.ContainsIP(s.config.VisitorRequestExemptPrefixes, v.ip) {
			return next(w, r, v)
		}
		d := vrate.RequestAllowed()
		setRateLimitHeaders(w, limit.KindRequests, d)
		if !d.OK {
			return errHTTPTooManyRequestsLimitRequests.RetryAfter(d.RetryAfter)
		}
		return next(w, r, v)
	}
}

// setRateLimitHeaders adds the decision's limit as a policy named after the kind, e.g.
// RateLimit-Policy: "messages";q=250;w=86400 and RateLimit: "messages";r=249;t=3600. Each policy
// is its own field line, which the draft allows; a decision without a quota adds nothing.
func setRateLimitHeaders(w http.ResponseWriter, kind limit.Kind, d limit.Decision) {
	if d.Quota <= 0 {
		return
	}
	policy := fmt.Sprintf(`"%s";q=%d`, kind, d.Quota)
	if d.Window > 0 {
		policy += fmt.Sprintf(";w=%d", int64(math.Ceil(d.Window.Seconds())))
	}
	current := fmt.Sprintf(`"%s";r=%d`, kind, d.Remaining)
	if d.Reset > 0 {
		current += fmt.Sprintf(";t=%d", int64(math.Ceil(d.Reset.Seconds()))) // Rounded up like Retry-After, so the two agree
	}
	w.Header().Add(headerRateLimitPolicy, policy)
	w.Header().Add(headerRateLimit, current)
}

func (s *Server) ensureWebEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.config.WebRoot == "" {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureWebPushEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.config.WebRoot == "" || s.config.WebPushPublicKey == "" {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureUserManager(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.userManager == nil {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureUser(next handleFunc) handleFunc {
	return s.ensureUserManager(func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if v.User() == nil {
			return errHTTPUnauthorized
		}
		return next(w, r, v)
	})
}

func (s *Server) ensureAdmin(next handleFunc) handleFunc {
	return s.ensureUserManager(func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if !v.User().IsAdmin() {
			return errHTTPUnauthorized
		}
		return next(w, r, v)
	})
}

func (s *Server) ensureCallsEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.config.TwilioAccount == "" || s.userManager == nil {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureEmailsEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.mailer == nil || s.userManager == nil {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensurePaymentsEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.config.StripeSecretKey == "" || s.stripe == nil {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureStripeCustomer(next handleFunc) handleFunc {
	return s.ensureUser(func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if v.User().Billing.StripeCustomerID == "" {
			return errHTTPBadRequestNotAPaidUser
		}
		return next(w, r, v)
	})
}

func (s *Server) withAccountSync(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		err := next(w, r, v)
		if err == nil {
			s.publishSyncEventAsync(v)
		}
		return err
	}
}

func (s *Server) authorizeTopicWrite(next handleFunc) handleFunc {
	return s.authorizeTopic(next, user.PermissionWrite)
}

func (s *Server) authorizeTopicRead(next handleFunc) handleFunc {
	return s.authorizeTopic(next, user.PermissionRead)
}

func (s *Server) authorizeTopic(next handleFunc, perm user.Permission) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.userManager == nil {
			return next(w, r, v)
		}
		topics, _, err := s.topicsFromPath(v, r.URL.Path)
		if err != nil {
			return err
		}
		u := v.User()
		for _, t := range topics {
			if err := s.userManager.Authorize(u, t.ID, perm); err != nil {
				logvr(v, r).With(t).Err(err).Debug("Access to topic %s not authorized", t.ID)
				return errHTTPForbidden.With(t)
			}
		}
		return next(w, r, v)
	}
}
