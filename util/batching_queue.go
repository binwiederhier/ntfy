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
//	q := NewBatchingQueue[int](2, 500 * time.Millisecond, 0)
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
	done      chan struct{}  // Closed by Close, stops the timeout ticker
	sending   sync.WaitGroup // In-flight sends to out; Close waits for them before closing out
	closed    bool
	mu        sync.Mutex // Protects in, closed, and sending.Add
}

// NewBatchingQueue creates a new BatchingQueue. Up to bufferedBatches full batches can wait for
// the consumer before Enqueue blocks (0 = Enqueue blocks until the consumer takes the batch).
func NewBatchingQueue[T any](batchSize int, timeout time.Duration, bufferedBatches int) *BatchingQueue[T] {
	q := &BatchingQueue[T]{
		batchSize: batchSize,
		timeout:   timeout,
		in:        make([]T, 0),
		out:       make(chan []T, bufferedBatches),
		done:      make(chan struct{}),
	}
	go q.timeoutTicker()
	return q
}

// Enqueue enqueues an element to the queue. If the configured batch size is reached,
// the batch will be emitted immediately. Elements enqueued after Close are dropped.
func (q *BatchingQueue[T]) Enqueue(element T) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.in = append(q.in, element)
	var elements []T
	if len(q.in) == q.batchSize {
		elements = q.dequeueAll()
		q.sending.Add(1)
	}
	q.mu.Unlock()
	if len(elements) > 0 {
		q.out <- elements
		q.sending.Done()
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
	defer ticker.Stop()
	for {
		select {
		case <-q.done:
			return
		case <-ticker.C:
		}
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return
		}
		elements := q.dequeueAll()
		if len(elements) > 0 {
			q.sending.Add(1)
		}
		q.mu.Unlock()
		if len(elements) > 0 {
			q.out <- elements
			q.sending.Done()
		}
	}
}

// Close emits the elements still waiting for their batch and closes the output channel. The
// consumer must keep reading from Dequeue until the channel is closed. Close blocks until it has
// handed those elements over, so a caller that cannot wait forever for a stuck consumer has to
// bound the call itself (see message.Cache.Close).
func (q *BatchingQueue[T]) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	elements := q.dequeueAll()
	q.mu.Unlock()
	close(q.done)
	q.sending.Wait()
	if len(elements) > 0 {
		q.out <- elements
	}
	close(q.out)
}
