package server

import (
	"bufio"
	"heckel.io/ntfy/util"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
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

type httpResponseWriter struct {
	w             http.ResponseWriter
	headerWritten bool
	mu            sync.Mutex
}

type httpResponseWriterWithHijacker struct {
	httpResponseWriter
}

var _ http.ResponseWriter = (*httpResponseWriter)(nil)
var _ http.Flusher = (*httpResponseWriter)(nil)
var _ http.Hijacker = (*httpResponseWriterWithHijacker)(nil)

func newHTTPResponseWriter(w http.ResponseWriter) http.ResponseWriter {
	if _, ok := w.(http.Hijacker); ok {
		return &httpResponseWriterWithHijacker{httpResponseWriter: httpResponseWriter{w: w}}
	}
	return &httpResponseWriter{w: w}
}

func (w *httpResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *httpResponseWriter) Write(bytes []byte) (int, error) {
	w.mu.Lock()
	w.headerWritten = true
	w.mu.Unlock()
	return w.w.Write(bytes)
}

func (w *httpResponseWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	if w.headerWritten {
		w.mu.Unlock()
		return
	}
	w.headerWritten = true
	w.mu.Unlock()
	w.w.WriteHeader(statusCode)
}

func (w *httpResponseWriter) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *httpResponseWriterWithHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, _ := w.w.(http.Hijacker)
	return h.Hijack()
}
