package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"heckel.io/ntfy/v2/user"
)

// maybeAuthenticate reads the "Authorization" header and will try to authenticate the user
// if it is set.
//
//   - If auth-file is not configured, immediately return an IP-based visitor
//   - If the header is not set or not supported (anything non-Basic and non-Bearer),
//     an IP-based visitor is returned
//   - If the header is set, authenticate will be called to check the username/password (Basic auth),
//     or the token (Bearer auth), and read the user from the database
//
// This function will ALWAYS return a visitor, even if an error occurs (e.g. unauthorized), so
// that subsequent logging calls still have a visitor context.
func (s *Server) maybeAuthenticate(r *http.Request) (*http.Request, *visitor, error) {
	// Read the "Authorization" header value and exit out early if it's not set
	ip := extractIPAddress(r, s.config.BehindProxy, s.config.ProxyForwardedHeader, s.config.ProxyTrustedPrefixes)
	// Stash the extracted client IP in the request context so downstream code (the abuse ban-feed in
	// handleError) can reuse it without re-parsing headers, and so an account-keyed (tier'd) visitor --
	// whose shared visitor object has a stale v.ip -- is still attributed to the actual request IP.
	r = withContext(r, map[contextKey]any{contextVisitorIP: ip})
	vip := s.visitor(ip, nil)
	if s.userManager == nil {
		return r, vip, nil
	}
	if s.config.BehindProxy && s.config.AuthUserHeader != "" {
		u, err := s.authenticateProxyHeader(r)
		if err != nil {
			logr(r).Err(err).Debug("Proxy header authentication failed")
			return r, vip, errHTTPUnauthorized
		} else if u != nil {
			return r, s.visitor(ip, u), nil
		}
		return r, vip, nil
	}
	header, err := readAuthHeader(r)
	if err != nil {
		return r, vip, err
	} else if !supportedAuthHeader(header) {
		return r, vip, nil
	}
	// If we're trying to auth, check the rate limiter first
	if !vip.AuthAllowed() {
		return r, vip, errHTTPTooManyRequestsLimitAuthFailure // Always return visitor, even when error occurs!
	}
	u, err := s.authenticate(r, header)
	if err != nil {
		vip.AuthFailed()
		logr(r).Err(err).Debug("Authentication failed")
		return r, vip, errHTTPUnauthorized // Always return visitor, even when error occurs!
	}
	// Authentication with user was successful
	return r, s.visitor(ip, u), nil
}

// authenticate a user based on basic auth username/password (Authorization: Basic ...), or token auth (Authorization: Bearer ...).
// The Authorization header can be passed as a header or the ?auth=... query param. The latter is required only to
// support the WebSocket JavaScript class, which does not support passing headers during the initial request. The auth
// query param is effectively doubly base64 encoded. Its format is base64(Basic base64(user:pass)).
func (s *Server) authenticate(r *http.Request, header string) (user *user.User, err error) {
	if strings.HasPrefix(header, "Bearer") {
		return s.authenticateBearerAuth(r, strings.TrimSpace(strings.TrimPrefix(header, "Bearer")))
	}
	return s.authenticateBasicAuth(r, header)
}

// readAuthHeader reads the raw value of the Authorization header, either from the actual HTTP header,
// or from the ?auth... query parameter
func readAuthHeader(r *http.Request) (string, error) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	queryParam := readQueryParam(r, "authorization", "auth")
	if queryParam != "" {
		a, err := base64.RawURLEncoding.DecodeString(queryParam)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(string(a))
	}
	return value, nil
}

// supportedAuthHeader returns true only if the Authorization header value starts
// with "Basic" or "Bearer". In particular, an empty value is not supported, and neither
// are things like "WebPush", or "vapid" (see #629).
func supportedAuthHeader(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "basic ") || strings.HasPrefix(value, "bearer ")
}

func (s *Server) authenticateBasicAuth(r *http.Request, value string) (user *user.User, err error) {
	r.Header.Set("Authorization", value)
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, errors.New("invalid basic auth")
	} else if username == "" {
		return s.authenticateBearerAuth(r, password) // Treat password as token
	}
	return s.userManager.Authenticate(username, password)
}

func (s *Server) authenticateBearerAuth(r *http.Request, token string) (*user.User, error) {
	u, err := s.userManager.AuthenticateToken(token)
	if err != nil {
		return nil, err
	}
	ip := extractIPAddress(r, s.config.BehindProxy, s.config.ProxyForwardedHeader, s.config.ProxyTrustedPrefixes)
	go s.userManager.EnqueueTokenUpdate(token, &user.TokenUpdate{
		LastAccess: time.Now(),
		LastOrigin: ip,
	})
	return u, nil
}
