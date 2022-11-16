package util

import (
	"sync"
	"time"
)

// BatchingQueue is a queue that creates batches of the enqueued elements based on a
// max batch size and a batch timeout.
//
// Example:
//
//	q := NewBatchingQueue[int](2, 500 * time.Millisecond)
//	go func() {
//	  for batch := range q.Dequeue() {
//	    fmt.Println(batch)
//	  }
//	}()
//	q.Enqueue(1)
//	q.Enqueue(2)
//	q.Enqueue(3)
//	time.Sleep(time.Second)
//
// This example will emit batch [1, 2] immediately (because the batch size is 2), and
// a batch [3] after 500ms.
type BatchingQueue[T any] struct {
	batchSize int
	timeout   time.Duration
	in        []T
	out       chan []T
	mu        sync.Mutex
}

// NewBatchingQueue creates a new BatchingQueue
func NewBatchingQueue[T any](batchSize int, timeout time.Duration) *BatchingQueue[T] {
	q := &BatchingQueue[T]{
		batchSize: batchSize,
		timeout:   timeout,
		in:        make([]T, 0),
		out:       make(chan []T),
	}
	go q.timeoutTicker()
	return q
}

// Enqueue enqueues an element to the queue. If the configured batch size is reached,
// the batch will be emitted immediately.
func (q *BatchingQueue[T]) Enqueue(element T) {
	q.mu.Lock()
	q.in = append(q.in, element)
	var elements []T
	if len(q.in) == q.batchSize {
		elements = q.dequeueAll()
	}
	q.mu.Unlock()
	if len(elements) > 0 {
		q.out <- elements
	}
}

// Dequeue returns a channel emitting batches of elements
func (q *BatchingQueue[T]) Dequeue() <-chan []T {
	return q.out
}

func (q *BatchingQueue[T]) dequeueAll() []T {
	elements := make([]T, len(q.in))
	copy(elements, q.in)
	q.in = q.in[:0]
	return elements
}

func (q *BatchingQueue[T]) timeoutTicker() {
	if q.timeout == 0 {
		return
	}
	ticker := time.NewTicker(q.timeout)
	for range ticker.C {
		q.mu.Lock()
		elements := q.dequeueAll()
		q.mu.Unlock()
		if len(elements) > 0 {
			q.out <- elements
		}
	}
}
