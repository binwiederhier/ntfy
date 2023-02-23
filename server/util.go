package server

import (
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

func readBoolParam(r *http.Request, defaultValue bool, names ...string) bool {
	value := strings.ToLower(readParam(r, names...))
	if value == "" {
		return defaultValue
	}
	return value == "1" || value == "yes" || value == "true"
}

func readCommaSeperatedParam(r *http.Request, names ...string) (params []string) {
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
		value := r.Header.Get(name)
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readHeaderParamValues(r *http.Request, names ...string) (values []string) {
	for _, name := range names {
		values = append(values, r.Header.Values(name)...)
	}
	return
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
				logr(r).Err(err).Warn("unable to parse IP (%s), new visitor with unspecified IP (0.0.0.0) created", remoteAddr)
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
			logr(r).Err(err).Error("invalid IP address %s received in X-Forwarded-For header", ip)
			// Fall back to regular remote address if X-Forwarded-For is damaged
		} else {
			ip = realIP
		}
	}
	return ip
}

func readJSONWithLimit[T any](r io.ReadCloser, limit int, allowEmpty bool) (*T, error) {
	obj, err := util.UnmarshalJSONWithLimit[T](r, limit, allowEmpty)
	if err == util.ErrUnmarshalJSON {
		return nil, errHTTPBadRequestJSONInvalid
	} else if err == util.ErrTooLargeJSON {
		return nil, errHTTPEntityTooLargeJSONBody
	} else if err != nil {
		return nil, err
	}
	return obj, nil
}
