package server

import (
	"context"
	"sync"
)

// streamRegistry tracks the open subscriber connections (HTTP streams and WebSockets), one entry
// per connection however many topics it subscribes to, so a drain can close them one at a time.
// Polls are not streams: they end on their own.
type streamRegistry struct {
	nextID  int
	cancels map[int]context.CancelFunc
	mu      sync.Mutex // Protects nextID and cancels
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{
		cancels: make(map[int]context.CancelFunc),
	}
}

// Add registers a stream and returns its ID for Remove
func (r *streamRegistry) Add(cancel context.CancelFunc) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.cancels[r.nextID] = cancel
	return r.nextID
}

// Remove forgets a stream that ended; a stream CancelSome already closed is gone already
func (r *streamRegistry) Remove(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

// Len returns the number of open streams
func (r *streamRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cancels)
}

// CancelSome closes up to n streams, picked in map order (i.e. at random), and returns how
// many it closed. They are removed right away, so the next call picks others.
func (r *streamRegistry) CancelSome(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	canceled := 0
	for id, cancel := range r.cancels {
		if canceled >= n {
			break
		}
		cancel()
		delete(r.cancels, id)
		canceled++
	}
	return canceled
}
