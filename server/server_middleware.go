package server

import (
	"net/http"

	"heckel.io/ntfy/util"
)

type contextKey int

const (
	contextRateVisitor contextKey = iota + 2586
	contextTopic
	contextMatrixPushKey
)

func (s *Server) limitRequests(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if util.ContainsIP(s.config.VisitorRequestExemptIPAddrs, v.ip) {
			return next(w, r, v)
		} else if !v.RequestAllowed() {
			return errHTTPTooManyRequestsLimitRequests
		}
		return next(w, r, v)
	}
}

// limitRequestsWithTopic limits requests with a topic and stores the rate-limiting-subscriber and topic into request.Context
func (s *Server) limitRequestsWithTopic(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		t, err := s.topicFromPath(r.URL.Path)
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
		if util.ContainsIP(s.config.VisitorRequestExemptIPAddrs, v.ip) {
			return next(w, r, v)
		} else if !vrate.RequestAllowed() {
			return errHTTPTooManyRequestsLimitRequests
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureWebEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.config.WebRoot == "" {
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
