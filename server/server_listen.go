package server

import (
	"context"
	"net"
)

// listenTCP listens for the clients (listen-http, listen-https) without TCP keep-alive probes. Go
// sends them every 15s on an idle accepted connection by default; to a phone holding a stream, each
// is a radio wake-up, on top of ntfy's own keepalive event (keepalive-interval, 45s), which already
// keeps the connection alive and finds dead ones.
func listenTCP(addr string) (net.Listener, error) {
	lc := &net.ListenConfig{KeepAlive: -1}
	return lc.Listen(context.Background(), "tcp", addr)
}
