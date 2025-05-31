package server

import (
	"context"
	"errors"
	"fmt"
	"heckel.io/ntfy/v2/util"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
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

func extractIPAddress(r *http.Request, behindProxy bool, proxyForwardedHeader string) netip.Addr {
	remoteAddr := r.RemoteAddr
	addrPort, err := netip.ParseAddrPort(remoteAddr)
	ip := addrPort.Addr()
	if err != nil {
		// This should not happen in real life; only in tests. So, using falling back to 0.0.0.0 if address unspecified
		ip, err = netip.ParseAddr(remoteAddr)
		if err != nil {
			ip = netip.IPv4Unspecified()
			if remoteAddr != "@" && !behindProxy { // RemoteAddr is @ when unix socket is used
				logr(r).Err(err).Warn("unable to parse IP (%s), new visitor with unspecified IP (0.0.0.0) created", remoteAddr)
			}
		}
	}
	if behindProxy && strings.TrimSpace(r.Header.Get(proxyForwardedHeader)) != "" {
		// X-Forwarded-For can contain multiple addresses (see #328). If we are behind a proxy,
		// only the right-most address can be trusted (as this is the one added by our proxy server).
		// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Forwarded-For for details.
		ips := util.SplitNoEmpty(r.Header.Get(proxyForwardedHeader), ",")
		realIP, err := netip.ParseAddr(strings.TrimSpace(util.LastString(ips, remoteAddr)))
		if err != nil {
			logr(r).Err(err).Error("invalid IP address %s received in %s header", ip, proxyForwardedHeader)
			// Fall back to the regular remote address if X-Forwarded-For is damaged
		} else {
			ip = realIP
		}
	}
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
