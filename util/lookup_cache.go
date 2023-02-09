package util

import (
	"sync"
	"time"
)

// LookupCache is a single-value cache with a time-to-live (TTL). The cache has a lookup function
// to retrieve the value and stores it until TTL is reached.
//
// Example:
//
//	lookup := func() (string, error) {
//	   r, _ := http.Get("...")
//	   s, _ := io.ReadAll(r.Body)
//	   return string(s), nil
//	}
//	c := NewLookupCache[string](lookup, time.Hour)
//	fmt.Println(c.Get()) // Fetches the string via HTTP
//	fmt.Println(c.Get()) // Uses cached value
type LookupCache[T any] struct {
	value   *T
	lookup  func() (T, error)
	ttl     time.Duration
	updated time.Time
	mu      sync.Mutex
}

// LookupFunc is a function that is called by the LookupCache if the underlying
// value is out-of-date. It returns the new value, or an error.
type LookupFunc[T any] func() (T, error)

// NewLookupCache creates a new LookupCache with a given time-to-live (TTL)
func NewLookupCache[T any](lookup LookupFunc[T], ttl time.Duration) *LookupCache[T] {
	return &LookupCache[T]{
		value:  nil,
		lookup: lookup,
		ttl:    ttl,
	}
}

// Value returns the cached value, or retrieves it via the lookup function
func (c *LookupCache[T]) Value() (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value == nil || (c.ttl > 0 && time.Since(c.updated) > c.ttl) {
		value, err := c.lookup()
		if err != nil {
			var t T
			return t, err
		}
		c.value = &value
		c.updated = time.Now()
	}
	return *c.value, nil
}
