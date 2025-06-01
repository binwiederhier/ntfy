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
	"slices"
	"strings"
)

var (
	mimeDecoder mime.WordDecoder

	// priorityHeaderIgnoreRegex matches specific patterns of the "Priority" header (RFC 9218), so that it can be ignored
	priorityHeaderIgnoreRegex = regexp.MustCompile(`^u=\d,\s*(i|\d)$|^u=\d$`)

	// forwardedHeaderRegex parses IPv4 addresses from the "Forwarded" header (RFC 7239)
	forwardedHeaderRegex = regexp.MustCompile(`(?i)\bfor="?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})"?`)
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

func readCommaSeparatedParam(r *http.Request, names ...string) []string {
	if paramStr := readParam(r, names...); paramStr != "" {
		return util.Map(util.SplitNoEmpty(paramStr, ","), strings.TrimSpace)
	}
	return []string{}
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

func extractIPAddress(r *http.Request, behindProxy bool, proxyForwardedHeader string, proxyTrustedAddresses []string) netip.Addr {
	if behindProxy && proxyForwardedHeader != "" {
		if addr, err := extractIPAddressFromHeader(r, proxyForwardedHeader, proxyTrustedAddresses); err == nil {
			return addr
		}
		// Fall back to the remote address if the header is not found or invalid
	}
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		logr(r).Err(err).Warn("unable to parse IP (%s), new visitor with unspecified IP (0.0.0.0) created", r.RemoteAddr)
		return netip.IPv4Unspecified()
	}
	return addrPort.Addr()
}

// extractIPAddressFromHeader extracts the last IP address from the specified header.
//
// X-Forwarded-For can contain multiple addresses (see #328). If we are behind a proxy,
// only the right-most address can be trusted (as this is the one added by our proxy server).
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Forwarded-For for details.
func extractIPAddressFromHeader(r *http.Request, forwardedHeader string, trustedAddresses []string) (netip.Addr, error) {
	value := strings.TrimSpace(strings.ToLower(r.Header.Get(forwardedHeader)))
	if value == "" {
		return netip.IPv4Unspecified(), fmt.Errorf("no %s header found", forwardedHeader)
	}
	// Extract valid addresses
	addrsStrs := util.Map(util.SplitNoEmpty(value, ","), strings.TrimSpace)
	var validAddrs []netip.Addr
	for _, addrStr := range addrsStrs {
		if addr, err := netip.ParseAddr(addrStr); err == nil {
			validAddrs = append(validAddrs, addr)
		} else if m := forwardedHeaderRegex.FindStringSubmatch(addrStr); len(m) == 2 {
			if addr, err := netip.ParseAddr(m[1]); err == nil {
				validAddrs = append(validAddrs, addr)
			}
		}
	}
	// Filter out proxy addresses
	clientAddrs := util.Filter(validAddrs, func(addr netip.Addr) bool {
		return !slices.Contains(trustedAddresses, addr.String())
	})
	if len(clientAddrs) == 0 {
		return netip.IPv4Unspecified(), fmt.Errorf("no client IP address found in %s header: %s", forwardedHeader, value)
	}
	return clientAddrs[len(clientAddrs)-1], nil
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
// to ignore the new HTTP "Priority" header.
func maybeDecodeHeader(name, value string) string {
	decoded, err := mimeDecoder.DecodeHeader(value)
	if err != nil {
		return maybeIgnoreSpecialHeader(name, value)
	}
	return maybeIgnoreSpecialHeader(name, decoded)
}

// maybeIgnoreSpecialHeader ignores the new HTTP "Priority" header (RFC 9218, see https://datatracker.ietf.org/doc/html/rfc9218)
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
