package util

import (
	"sync"
	"time"
)

type BatchingQueue[T any] struct {
	batchSize int
	timeout   time.Duration
	in        []T
	out       chan []T
	mu        sync.Mutex
}

func NewBatchingQueue[T any](batchSize int, timeout time.Duration) *BatchingQueue[T] {
	q := &BatchingQueue[T]{
		batchSize: batchSize,
		timeout:   timeout,
		in:        make([]T, 0),
		out:       make(chan []T),
	}
	ticker := time.NewTicker(timeout)
	go func() {
		for range ticker.C {
			elements := q.popAll()
			if len(elements) > 0 {
				q.out <- elements
			}
		}
	}()
	return q
}

func (c *BatchingQueue[T]) Push(element T) {
	c.mu.Lock()
	c.in = append(c.in, element)
	limitReached := len(c.in) == c.batchSize
	c.mu.Unlock()
	if limitReached {
		c.out <- c.popAll()
	}
}

func (c *BatchingQueue[T]) Pop() <-chan []T {
	return c.out
}

func (c *BatchingQueue[T]) popAll() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	elements := make([]T, len(c.in))
	copy(elements, c.in)
	c.in = c.in[:0]
	return elements
}
