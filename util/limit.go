package util

import (
	"errors"
	"io"
	"sync"
)

// ErrLimitReached is the error returned by the Limiter and LimitWriter when the predefined limit has been reached
var ErrLimitReached = errors.New("limit reached")

// Limiter is a helper that allows adding values up to a well-defined limit. Once the limit is reached
// ErrLimitReached will be returned. Limiter may be used by multiple goroutines.
type Limiter struct {
	value int64
	limit int64
	mu    sync.Mutex
}

// NewLimiter creates a new Limiter
func NewLimiter(limit int64) *Limiter {
	return &Limiter{
		limit: limit,
	}
}

// Add adds n to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded after adding n, ErrLimitReached is returned.
func (l *Limiter) Add(n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.value+n <= l.limit {
		l.value += n
		return nil
	} else {
		return ErrLimitReached
	}
}

// Sub subtracts a value from the limiters internal value
func (l *Limiter) Sub(n int64) {
	l.Add(-n)
}

// Set sets the value of the limiter to n. This function ignores the limit. It is meant to set the value
// based on reality.
func (l *Limiter) Set(n int64) {
	l.mu.Lock()
	l.value = n
	l.mu.Unlock()
}

// Value returns the internal value of the limiter
func (l *Limiter) Value() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.value
}

// Limit returns the defined limit
func (l *Limiter) Limit() int64 {
	return l.limit
}

// LimitWriter implements an io.Writer that will pass through all Write calls to the underlying
// writer w until any of the limiter's limit is reached, at which point a Write will return ErrLimitReached.
// Each limiter's value is increased with every write.
type LimitWriter struct {
	w        io.Writer
	written  int64
	limiters []*Limiter
	mu       sync.Mutex
}

// NewLimitWriter creates a new LimitWriter
func NewLimitWriter(w io.Writer, limiters ...*Limiter) *LimitWriter {
	return &LimitWriter{
		w:        w,
		limiters: limiters,
	}
}

// Write passes through all writes to the underlying writer until any of the given limiter's limit is reached
func (w *LimitWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := 0; i < len(w.limiters); i++ {
		if err := w.limiters[i].Add(int64(len(p))); err != nil {
			for j := i - 1; j >= 0; j-- {
				w.limiters[j].Sub(int64(len(p)))
			}
			return 0, ErrLimitReached
		}
	}
	n, err = w.w.Write(p)
	w.written += int64(n)
	return
}
