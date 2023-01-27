package util

import (
	"errors"
	"golang.org/x/time/rate"
	"io"
	"sync"
	"time"
)

// ErrLimitReached is the error returned by the Limiter and LimitWriter when the predefined limit has been reached
var ErrLimitReached = errors.New("limit reached")

// Limiter is an interface that implements a rate limiting mechanism, e.g. based on time or a fixed value
type Limiter interface {
	// Allow adds n to the limiters internal value, or returns ErrLimitReached if the limit has been reached
	Allow(n int64) error
}

// FixedLimiter is a helper that allows adding values up to a well-defined limit. Once the limit is reached
// ErrLimitReached will be returned. FixedLimiter may be used by multiple goroutines.
type FixedLimiter struct {
	value int64
	limit int64
	mu    sync.Mutex
}

// NewFixedLimiter creates a new Limiter
func NewFixedLimiter(limit int64) *FixedLimiter {
	return NewFixedLimiterWithValue(limit, 0)
}

// NewFixedLimiterWithValue creates a new Limiter and sets the initial value
func NewFixedLimiterWithValue(limit, value int64) *FixedLimiter {
	return &FixedLimiter{
		limit: limit,
		value: value,
	}
}

// Allow adds n to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded after adding n, ErrLimitReached is returned.
func (l *FixedLimiter) Allow(n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.value+n > l.limit {
		return ErrLimitReached
	}
	l.value += n
	return nil
}

// Value returns the current limiter value
func (l *FixedLimiter) Value() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.value
}

// Reset sets the limiter's value back to zero
func (l *FixedLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.value = 0
}

// RateLimiter is a Limiter that wraps a rate.Limiter, allowing a floating time-based limit.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

// NewBytesLimiter creates a RateLimiter that is meant to be used for a bytes-per-interval limit,
// e.g. 250 MB per day. And example of the underlying idea can be found here: https://go.dev/play/p/0ljgzIZQ6dJ
func NewBytesLimiter(bytes int, interval time.Duration) *RateLimiter {
	return NewRateLimiter(rate.Limit(bytes)*rate.Every(interval), bytes)
}

// Allow adds n to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded after adding n, ErrLimitReached is returned.
func (l *RateLimiter) Allow(n int64) error {
	if n <= 0 {
		return nil // No-op. Can't take back bytes you're written!
	}
	if !l.limiter.AllowN(time.Now(), int(n)) {
		return ErrLimitReached
	}
	return nil
}

// LimitWriter implements an io.Writer that will pass through all Write calls to the underlying
// writer w until any of the limiter's limit is reached, at which point a Write will return ErrLimitReached.
// Each limiter's value is increased with every write.
type LimitWriter struct {
	w        io.Writer
	written  int64
	limiters []Limiter
	mu       sync.Mutex
}

// NewLimitWriter creates a new LimitWriter
func NewLimitWriter(w io.Writer, limiters ...Limiter) *LimitWriter {
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
		if err := w.limiters[i].Allow(int64(len(p))); err != nil {
			for j := i - 1; j >= 0; j-- {
				w.limiters[j].Allow(-int64(len(p))) // Revert limiters limits if allowed
			}
			return 0, ErrLimitReached
		}
	}
	n, err = w.w.Write(p)
	w.written += int64(n)
	return
}
