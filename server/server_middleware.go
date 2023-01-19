package server

import (
	"net/http"
)

func (s *Server) ensureWebEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if !s.config.EnableWeb {
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
		if v.user == nil {
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
		if v.user.Billing.StripeCustomerID == "" {
			return errHTTPBadRequestNotAPaidUser
		}
		return next(w, r, v)
	})
}

func (s *Server) withAccountSync(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if v.user == nil {
			return next(w, r, v)
		}
		err := next(w, r, v)
		if err == nil {
			s.publishSyncEventAsync(v)
		}
		return err
	}
}
