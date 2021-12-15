package util

import (
	"errors"
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

// Add adds n to the limiters internal value, but only if the limit has not been reached. If the limit would be
// exceeded after adding n, ErrLimitReached is returned.
func (l *Limiter) Add(n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit == 0 {
		l.value += n
		return nil
	} else if l.value+n <= l.limit {
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
