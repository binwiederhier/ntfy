package server

import (
	"fmt"
	"github.com/emersion/go-smtp"
	"golang.org/x/time/rate"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
	"unicode/utf8"
)

func logr(r *http.Request) *log.Event {
	return log.Fields(httpFields(r))
}

func logv(v *visitor) *log.Event {
	return log.Context(v)
}

func logvr(v *visitor, r *http.Request) *log.Event {
	return logv(v).Fields(httpFields(r))
}

func logvrm(v *visitor, r *http.Request, m *message) *log.Event {
	return logvr(v, r).Context(m)
}

func logvm(v *visitor, m *message) *log.Event {
	return logv(v).Context(m)
}

func logem(state *smtp.ConnectionState) *log.Event {
	return log.
		Tag(tagSMTP).
		Fields(map[string]any{
			"smtp_hostname":    state.Hostname,
			"smtp_remote_addr": state.RemoteAddr.String(),
		})
}

func httpFields(r *http.Request) map[string]any {
	requestURI := r.RequestURI
	if requestURI == "" {
		requestURI = r.URL.Path
	}
	return map[string]any{
		"http_method": r.Method,
		"http_path":   requestURI,
	}
}

func requestLimiterFields(limiter *rate.Limiter) map[string]any {
	return map[string]any{
		"visitor_request_limiter_limit":  limiter.Limit(),
		"visitor_request_limiter_tokens": limiter.Tokens(),
	}
}

func renderHTTPRequest(r *http.Request) string {
	peekLimit := 4096
	lines := fmt.Sprintf("%s %s %s\n", r.Method, r.URL.RequestURI(), r.Proto)
	for key, values := range r.Header {
		for _, value := range values {
			lines += fmt.Sprintf("%s: %s\n", key, value)
		}
	}
	lines += "\n"
	body, err := util.Peek(r.Body, peekLimit)
	if err != nil {
		lines = fmt.Sprintf("(could not read body: %s)\n", err.Error())
	} else if utf8.Valid(body.PeekedBytes) {
		lines += string(body.PeekedBytes)
		if body.LimitReached {
			lines += fmt.Sprintf(" ... (peeked %d bytes)", peekLimit)
		}
		lines += "\n"
	} else {
		if body.LimitReached {
			lines += fmt.Sprintf("(peeked bytes not UTF-8, peek limit of %d bytes reached, hex: %x ...)\n", peekLimit, body.PeekedBytes)
		} else {
			lines += fmt.Sprintf("(peeked bytes not UTF-8, %d bytes, hex: %x)\n", len(body.PeekedBytes), body.PeekedBytes)
		}
	}
	r.Body = body // Important: Reset body, so it can be re-read
	return strings.TrimSpace(lines)
}
