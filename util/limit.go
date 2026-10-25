package util

import (
	"errors"
	"io"
	"sync"
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

// CountingReader wraps an io.Reader and counts the number of bytes read through it.
type CountingReader struct {
	r     io.Reader
	total int64
}

// NewCountingReader creates a new CountingReader
func NewCountingReader(r io.Reader) *CountingReader {
	return &CountingReader{r: r}
}

// Read passes through to the underlying reader and counts the bytes read
func (r *CountingReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	r.total += int64(n)
	return
}

// Total returns the total number of bytes read so far
func (r *CountingReader) Total() int64 {
	return r.total
}

// LimitReader implements an io.Reader that will pass through all Read calls to the underlying
// reader r until any of the limiter's limit is reached, at which point a Read will return ErrLimitReached.
// Each limiter's value is increased after every read based on the number of bytes actually read.
type LimitReader struct {
	r        io.Reader
	limiters []Limiter
}

// NewLimitReader creates a new LimitReader
func NewLimitReader(r io.Reader, limiters ...Limiter) *LimitReader {
	return &LimitReader{
		r:        r,
		limiters: limiters,
	}
}

// Read passes through all reads to the underlying reader until any of the given limiter's limit is reached
func (r *LimitReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if n > 0 {
		for i := 0; i < len(r.limiters); i++ {
			if !r.limiters[i].AllowN(int64(n)) {
				for j := i - 1; j >= 0; j-- {
					r.limiters[j].AllowN(-int64(n)) // Revert limiters if not allowed
				}
				return 0, ErrLimitReached
			}
		}
	}
	return
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
