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
	// Allow adds one to the limiters value, or returns false if the limit has been reached
	Allow() bool

	// AllowN adds n to the limiters value, or returns false if the limit has been reached
	AllowN(n int64) bool

	// Value returns the current internal limiter value
	Value() int64

	// Reset resets the state of the limiter
	Reset()
}

// FixedLimiter is a helper that allows adding values up to a well-defined limit. Once the limit is reached
// ErrLimitReached will be returned. FixedLimiter may be used by multiple goroutines.
type FixedLimiter struct {
	value int64
	limit int64
	mu    sync.Mutex
}

var _ Limiter = (*FixedLimiter)(nil)

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

// Allow adds one to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded, false is returned.
func (l *FixedLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN adds n to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded after adding n, false is returned.
func (l *FixedLimiter) AllowN(n int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.value+n > l.limit {
		return false
	}
	l.value += n
	return true
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
	r       rate.Limit
	b       int
	value   int64
	limiter *rate.Limiter
	mu      sync.Mutex
}

var _ Limiter = (*RateLimiter)(nil)

// NewRateLimiter creates a new RateLimiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return NewRateLimiterWithValue(r, b, 0)
}

// NewRateLimiterWithValue creates a new RateLimiter with the given starting value.
//
// Note that the starting value only has informational value. It does not impact the underlying
// value of the rate.Limiter.
func NewRateLimiterWithValue(r rate.Limit, b int, value int64) *RateLimiter {
	return &RateLimiter{
		r:       r,
		b:       b,
		value:   value,
		limiter: rate.NewLimiter(r, b),
	}
}

// NewBytesLimiter creates a RateLimiter that is meant to be used for a bytes-per-interval limit,
// e.g. 250 MB per day. And example of the underlying idea can be found here: https://go.dev/play/p/0ljgzIZQ6dJ
func NewBytesLimiter(bytes int, interval time.Duration) *RateLimiter {
	return NewRateLimiter(rate.Limit(bytes)*rate.Every(interval), bytes)
}

// Allow adds one to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded, false is returned.
func (l *RateLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN adds n to the limiters internal value, but only if the limit has not been reached. If the limit was
// exceeded after adding n, false is returned.
func (l *RateLimiter) AllowN(n int64) bool {
	if n <= 0 {
		return false // No-op. Can't take back bytes you're written!
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.limiter.AllowN(time.Now(), int(n)) {
		return false
	}
	l.value += n
	return true
}

// Value returns the current limiter value
func (l *RateLimiter) Value() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.value
}

// Reset sets the limiter's value back to zero, and resets the underlying rate.Limiter
func (l *RateLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limiter = rate.NewLimiter(l.r, l.b)
	l.value = 0
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
		if !w.limiters[i].AllowN(int64(len(p))) {
			for j := i - 1; j >= 0; j-- {
				w.limiters[j].AllowN(-int64(len(p))) // Revert limiters limits if not allowed
			}
			return 0, ErrLimitReached
		}
	}
	n, err = w.w.Write(p)
	w.written += int64(n)
	return
}
