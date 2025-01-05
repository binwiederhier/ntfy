package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	"heckel.io/ntfy/v2/util"
)

var (
	mimeDecoder               mime.WordDecoder
	priorityHeaderIgnoreRegex = regexp.MustCompile(`^u=\d,\s*(i|\d)$|^u=\d$`)
)

func readBoolParam(r *http.Request, defaultValue bool, names ...string) bool {
	value := strings.ToLower(readParam(r, names...))
	if value == "" {
		return defaultValue
	}
	return toBool(value)
}

func isBoolValue(value string) bool {
	return value == "1" || value == "yes" || value == "true" || value == "0" || value == "no" || value == "false"
}

func toBool(value string) bool {
	return value == "1" || value == "yes" || value == "true"
}

func readCommaSeparatedParam(r *http.Request, names ...string) (params []string) {
	paramStr := readParam(r, names...)
	if paramStr != "" {
		params = make([]string, 0)
		for _, s := range util.SplitNoEmpty(paramStr, ",") {
			params = append(params, strings.TrimSpace(s))
		}
	}
	return params
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
		value := strings.TrimSpace(maybeDecodeHeader(name, r.Header.Get(name)))
		if value != "" {
			return value
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

func extractIPAddress(r *http.Request, behindProxy bool, proxyClientIPHeader string) netip.Addr {
	logr(r).Debug("Starting IP extraction")

	remoteAddr := r.RemoteAddr
	logr(r).Debug("RemoteAddr: %s", remoteAddr)

	addrPort, err := netip.ParseAddrPort(remoteAddr)
	ip := addrPort.Addr()
	if err != nil {
		logr(r).Warn("Failed to parse RemoteAddr as AddrPort: %v", err)
		ip, err = netip.ParseAddr(remoteAddr)
		if err != nil {
			ip = netip.IPv4Unspecified()
			logr(r).Error("Failed to parse RemoteAddr as IP: %v, defaulting to 0.0.0.0", err)
		}
	}

	// Log initial IP before further processing
	logr(r).Debug("Initial IP after RemoteAddr parsing: %s", ip)

	if proxyClientIPHeader != "" {
		logr(r).Debug("Using ProxyClientIPHeader: %s", proxyClientIPHeader)
		if customHeaderIP := r.Header.Get(proxyClientIPHeader); customHeaderIP != "" {
			logr(r).Debug("Custom header %s value: %s", proxyClientIPHeader, customHeaderIP)
			realIP, err := netip.ParseAddr(customHeaderIP)
			if err != nil {
				logr(r).Error("Invalid IP in %s header: %s, error: %v", proxyClientIPHeader, customHeaderIP, err)
			} else {
				logr(r).Debug("Successfully parsed IP from custom header: %s", realIP)
				ip = realIP
			}
		} else {
			logr(r).Warn("Custom header %s is empty or missing", proxyClientIPHeader)
		}
	} else if behindProxy {
		logr(r).Debug("No ProxyClientIPHeader set, checking X-Forwarded-For")
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			logr(r).Debug("X-Forwarded-For value: %s", xff)
			ips := util.SplitNoEmpty(xff, ",")
			realIP, err := netip.ParseAddr(strings.TrimSpace(util.LastString(ips, remoteAddr)))
			if err != nil {
				logr(r).Error("Invalid IP in X-Forwarded-For header: %s, error: %v", xff, err)
			} else {
				logr(r).Debug("Successfully parsed IP from X-Forwarded-For: %s", realIP)
				ip = realIP
			}
		} else {
			logr(r).Debug("X-Forwarded-For header is empty or missing")
		}
	} else {
		logr(r).Debug("Behind proxy is false, skipping proxy headers")
	}

	// Final resolved IP
	logr(r).Debug("Final resolved IP: %s", ip)
	return ip
}

func readJSONWithLimit[T any](r io.ReadCloser, limit int, allowEmpty bool) (*T, error) {
	obj, err := util.UnmarshalJSONWithLimit[T](r, limit, allowEmpty)
	if errors.Is(err, util.ErrUnmarshalJSON) {
		return nil, errHTTPBadRequestJSONInvalid
	} else if errors.Is(err, util.ErrTooLargeJSON) {
		return nil, errHTTPEntityTooLargeJSONBody
	} else if err != nil {
		return nil, err
	}
	return obj, nil
}

func withContext(r *http.Request, ctx map[contextKey]any) *http.Request {
	c := r.Context()
	for k, v := range ctx {
		c = context.WithValue(c, k, v)
	}
	return r.WithContext(c)
}

func fromContext[T any](r *http.Request, key contextKey) (T, error) {
	t, ok := r.Context().Value(key).(T)
	if !ok {
		return t, fmt.Errorf("cannot find key %v in request context", key)
	}
	return t, nil
}

// maybeDecodeHeader decodes the given header value if it is MIME encoded, e.g. "=?utf-8?q?Hello_World?=",
// or returns the original header value if it is not MIME encoded. It also calls maybeIgnoreSpecialHeader
// to ignore new HTTP "Priority" header.
func maybeDecodeHeader(name, value string) string {
	decoded, err := mimeDecoder.DecodeHeader(value)
	if err != nil {
		return maybeIgnoreSpecialHeader(name, value)
	}
	return maybeIgnoreSpecialHeader(name, decoded)
}

// maybeIgnoreSpecialHeader ignores new HTTP "Priority" header (see https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-priority)
//
// Cloudflare (and potentially other providers) add this to requests when forwarding to the backend (ntfy),
// so we just ignore it. If the "Priority" header is set to "u=*, i" or "u=*" (by Cloudflare), the header will be ignored.
// Returning an empty string will allow the rest of the logic to continue searching for another header (x-priority, prio, p),
// or in the Query parameters.
func maybeIgnoreSpecialHeader(name, value string) string {
	if strings.ToLower(name) == "priority" && priorityHeaderIgnoreRegex.MatchString(strings.TrimSpace(value)) {
		return ""
	}
	return value
}
