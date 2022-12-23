package server

import (
	"fmt"
	"github.com/emersion/go-smtp"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"net/http"
	"net/netip"
	"strings"
	"unicode/utf8"
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

func extractIPAddress(r *http.Request, behindProxy bool) netip.Addr {
	remoteAddr := r.RemoteAddr
	addrPort, err := netip.ParseAddrPort(remoteAddr)
	ip := addrPort.Addr()
	if err != nil {
		// This should not happen in real life; only in tests. So, using falling back to 0.0.0.0 if address unspecified
		ip, err = netip.ParseAddr(remoteAddr)
		if err != nil {
			ip = netip.IPv4Unspecified()
			if remoteAddr != "@" || !behindProxy { // RemoteAddr is @ when unix socket is used
				log.Warn("unable to parse IP (%s), new visitor with unspecified IP (0.0.0.0) created %s", remoteAddr, err)
			}
		}
	}
	if behindProxy && strings.TrimSpace(r.Header.Get("X-Forwarded-For")) != "" {
		// X-Forwarded-For can contain multiple addresses (see #328). If we are behind a proxy,
		// only the right-most address can be trusted (as this is the one added by our proxy server).
		// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Forwarded-For for details.
		ips := util.SplitNoEmpty(r.Header.Get("X-Forwarded-For"), ",")
		realIP, err := netip.ParseAddr(strings.TrimSpace(util.LastString(ips, remoteAddr)))
		if err != nil {
			log.Error("invalid IP address %s received in X-Forwarded-For header: %s", ip, err.Error())
			// Fall back to regular remote address if X-Forwarded-For is damaged
		} else {
			ip = realIP
		}
	}
	return ip
}
