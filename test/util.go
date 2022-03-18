package test

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// WaitForPortUp waits up to 7s for a port to come up and fails t if that fails
func WaitForPortUp(t *testing.T, port int) {
	success := false
	for i := 0; i < 500; i++ {
		startTime := time.Now()
		conn, _ := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 10*time.Millisecond)
		if conn != nil {
			success = true
			conn.Close()
			break
		}
		if time.Since(startTime) < 10*time.Millisecond {
			time.Sleep(10*time.Millisecond - time.Since(startTime))
		}
	}
	if !success {
		t.Fatalf("Failed waiting for port %d to be UP", port)
	}
}

// WaitForPortDown waits up to 5s for a port to come down and fails t if that fails
func WaitForPortDown(t *testing.T, port int) {
	success := false
	for i := 0; i < 100; i++ {
		conn, _ := net.DialTimeout("tcp", net.JoinHostPort("", strconv.Itoa(port)), 50*time.Millisecond)
		if conn == nil {
			success = true
			break
		}
		conn.Close()
	}
	if !success {
		t.Fatalf("Failed waiting for port %d to be DOWN", port)
	}
}
