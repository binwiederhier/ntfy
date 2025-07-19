package util

import (
	"errors"
	"io"
	"time"
)

// ErrWriteTimeout is returned when a write timed out
var ErrWriteTimeout = errors.New("write operation failed due to timeout")

// TimeoutWriter wraps an io.Writer that will time out after the given timeout
type TimeoutWriter struct {
	writer  io.Writer
	timeout time.Duration
	start   time.Time
}

// NewTimeoutWriter creates a new TimeoutWriter
func NewTimeoutWriter(w io.Writer, timeout time.Duration) *TimeoutWriter {
	return &TimeoutWriter{
		writer:  w,
		timeout: timeout,
		start:   time.Now(),
	}
}

// Write implements the io.Writer interface, failing if called after the timeout period from creation.
func (tw *TimeoutWriter) Write(p []byte) (n int, err error) {
	if time.Since(tw.start) > tw.timeout {
		return 0, ErrWriteTimeout
	}
	return tw.writer.Write(p)
}
