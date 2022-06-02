package server

import (
	"encoding/json"
	"fmt"
	"github.com/emersion/go-smtp"
	"net/http"
	"strings"
)

func readBoolParam(r *http.Request, defaultValue bool, names ...string) bool {
	value := strings.ToLower(readParam(r, names...))
	if value == "" {
		return defaultValue
	}
	return value == "1" || value == "yes" || value == "true"
}

func readParam(r *http.Request, names ...string) string {
	value := readHeaderParam(r, names...)
	if value != "" {
		return value
	}
	return readQueryParam(r, names...)
}

func readHeaderParam(r *http.Request, names ...string) string {
	for _, name := range names {
		value := r.Header.Get(name)
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readQueryParam(r *http.Request, names ...string) string {
	for _, name := range names {
		value := r.URL.Query().Get(strings.ToLower(name))
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func logMessagePrefix(v *visitor, m *message) string {
	return fmt.Sprintf("%s/%s/%s", v.ip, m.Topic, m.ID)
}

func logHTTPPrefix(v *visitor, r *http.Request) string {
	requestURI := r.RequestURI
	if requestURI == "" {
		requestURI = r.URL.Path
	}
	return fmt.Sprintf("%s HTTP %s %s", v.ip, r.Method, requestURI)
}

func logSMTPPrefix(state *smtp.ConnectionState) string {
	return fmt.Sprintf("%s/%s SMTP", state.Hostname, state.RemoteAddr.String())
}

func maybeMarshalJSON(v interface{}) string {
	messageJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "<cannot serialize>"
	}
	if len(messageJSON) > 5000 {
		return string(messageJSON)[:5000]
	}
	return string(messageJSON)
}
