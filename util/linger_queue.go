package util

import (
	"sync"
	"time"
)

// LingerQueue is a bounded, non-blocking batching queue: enqueued elements are emitted as
// batches once a linger window expires, a batch count cap is reached, or a batch size cap is
// reached, whichever comes first. Unlike BatchingQueue, producers never block: TryEnqueue drops
// (returns false) when the queue is full, and Close flushes the remainder and terminates the
// consumer channel, so per-entity queues can be created and destroyed dynamically.
//
// Example:
//
//	q := NewLingerQueue[int](64, 10, 0, nil, 500*time.Millisecond)
//	go func() {
//	  for batch := range q.Dequeue() {
//	    send(batch)
//	  }
//	}()
//	q.TryEnqueue(1)
//	q.TryEnqueue(2) // emitted together as [1, 2] after <= 500ms
type LingerQueue[T any] struct {
	in      chan T
	out     chan []T
	max     int           // max elements per batch
	maxSize int           // max cumulative size per batch; 0 = no size cap
	size    func(T) int   // element size function; nil = no size cap
	linger  time.Duration // max time the first element of a batch waits; 0 = emit immediately
	closed  bool
	mu      sync.Mutex // Protects closed, and guards TryEnqueue's send against Close's close(in)
}

// NewLingerQueue creates a LingerQueue holding up to capacity queued elements, emitting batches
// of up to max elements or maxSize cumulative size (as measured by size; pass 0/nil for no size
// cap) after at most linger.
func NewLingerQueue[T any](capacity, max, maxSize int, size func(T) int, linger time.Duration) *LingerQueue[T] {
	q := &LingerQueue[T]{
		in:      make(chan T, capacity),
		out:     make(chan []T),
		max:     max,
		maxSize: maxSize,
		size:    size,
		linger:  linger,
	}
	go q.run()
	return q
}

// TryEnqueue enqueues an element without blocking. It returns false if the queue is full or
// closed; the caller decides how to account for the drop.
func (q *LingerQueue[T]) TryEnqueue(t T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	select {
	case q.in <- t:
		return true
	default:
		return false
	}
}

// Dequeue returns the channel emitting batches. It is closed after Close, once the remaining
// elements have been flushed.
func (q *LingerQueue[T]) Dequeue() <-chan []T {
	return q.out
}

// Close stops the queue: remaining elements are flushed as final batches, then the Dequeue
// channel is closed. TryEnqueue returns false after Close. Close is idempotent.
func (q *LingerQueue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.in)
}

// run is the batching loop: it blocks for the first element of a batch, then collects more until
// the linger expires or a cap is hit, and emits the batch. It exits once the queue is closed and
// drained. Note that receiving from the closed in channel still yields the buffered remainder
// before reporting closed, which is what flushes on Close.
func (q *LingerQueue[T]) run() {
	defer close(q.out)
	for {
		first, ok := <-q.in
		if !ok {
			return
		}
		batch := []T{first}
		bytes := q.sizeOf(first)
		var timeout <-chan time.Time
		if q.linger > 0 {
			timeout = time.After(q.linger)
		}
		closed := false
	collect:
		for len(batch) < q.max && (q.maxSize <= 0 || bytes < q.maxSize) {
			if timeout == nil {
				// Zero linger: greedily drain what is immediately available, never wait
				select {
				case t, ok := <-q.in:
					if !ok {
						closed = true
						break collect
					}
					batch = append(batch, t)
					bytes += q.sizeOf(t)
				default:
					break collect
				}
			} else {
				select {
				case t, ok := <-q.in:
					if !ok {
						closed = true
						break collect
					}
					batch = append(batch, t)
					bytes += q.sizeOf(t)
				case <-timeout:
					break collect
				}
			}
		}
		q.out <- batch
		if closed {
			return
		}
	}
}

func (q *LingerQueue[T]) sizeOf(t T) int {
	if q.size == nil {
		return 0
	}
	return q.size(t)
}
