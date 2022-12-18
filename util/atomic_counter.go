package util

import "sync"

type AtomicCounter[T int | int32 | int64] struct {
	value T
	mu    sync.Mutex
}

func NewAtomicCounter[T int | int32 | int64](value T) *AtomicCounter[T] {
	return &AtomicCounter[T]{
		value: value,
	}
}
func (c *AtomicCounter[T]) Inc() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
	return c.value
}

func (c *AtomicCounter[T]) Value() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *AtomicCounter[T]) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = 0
}
