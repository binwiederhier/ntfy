package server

import (
	"fmt"
	"github.com/emersion/go-smtp"
	"github.com/gorilla/websocket"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Log tags
const (
	tagStartup      = "startup"
	tagHTTP         = "http"
	tagPublish      = "publish"
	tagSubscribe    = "subscribe"
	tagFirebase     = "firebase"
	tagSMTP         = "smtp"  // Receive email
	tagEmail        = "email" // Send email
	tagFileCache    = "file_cache"
	tagMessageCache = "message_cache"
	tagStripe       = "stripe"
	tagAccount      = "account"
	tagManager      = "manager"
	tagResetter     = "resetter"
	tagWebsocket    = "websocket"
	tagMatrix       = "matrix"
)

var (
	normalErrorCodes = []int{http.StatusNotFound, http.StatusBadRequest, http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusInsufficientStorage}
)

// logr creates a new log event with HTTP request fields
func logr(r *http.Request) *log.Event {
	return log.Tag(tagHTTP).Fields(httpContext(r)) // Tag may be overwritten
}

// logv creates a new log event with visitor fields
func logv(v *visitor) *log.Event {
	return log.With(v)
}

// logvr creates a new log event with HTTP request and visitor fields
func logvr(v *visitor, r *http.Request) *log.Event {
	return logr(r).With(v)
}

// logvrm creates a new log event with HTTP request, visitor fields and message fields
func logvrm(v *visitor, r *http.Request, m *message) *log.Event {
	return logvr(v, r).With(m)
}

// logvrm creates a new log event with visitor fields and message fields
func logvm(v *visitor, m *message) *log.Event {
	return logv(v).With(m)
}

// logem creates a new log event with email fields
func logem(smtpConn *smtp.Conn) *log.Event {
	ev := log.Tag(tagSMTP).Field("smtp_hostname", smtpConn.Hostname())
	if smtpConn.Conn() != nil {
		ev.Field("smtp_remote_addr", smtpConn.Conn().RemoteAddr().String())
	}
	return ev
}

func httpContext(r *http.Request) log.Context {
	requestURI := r.RequestURI
	if requestURI == "" {
		requestURI = r.URL.Path
	}
	return log.Context{
		"http_method": r.Method,
		"http_path":   requestURI,
	}
}

func websocketErrorContext(err error) log.Context {
	if c, ok := err.(*websocket.CloseError); ok {
		return log.Context{
			"error":      c.Error(),
			"error_code": c.Code,
			"error_type": "websocket.CloseError",
		}
	}
	return log.Context{
		"error": err.Error(),
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
