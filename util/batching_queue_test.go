package util_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/util"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestBatchingQueue_InfTimeout(t *testing.T) {
	q := util.NewBatchingQueue[int](25, 1*time.Hour, 0)
	batches, total := make([][]int, 0), 0
	var mu sync.Mutex
	go func() {
		for batch := range q.Dequeue() {
			mu.Lock()
			batches = append(batches, batch)
			total += len(batch)
			mu.Unlock()
		}
	}()
	for i := 0; i < 101; i++ {
		go q.Enqueue(i)
	}
	time.Sleep(time.Second)
	mu.Lock()
	require.Equal(t, 100, total) // One is missing, stuck in the last batch!
	require.Equal(t, 4, len(batches))
	mu.Unlock()
}

func TestBatchingQueue_WithTimeout(t *testing.T) {
	q := util.NewBatchingQueue[int](25, 100*time.Millisecond, 0)
	batches, total := make([][]int, 0), 0
	var mu sync.Mutex
	go func() {
		for batch := range q.Dequeue() {
			mu.Lock()
			batches = append(batches, batch)
			total += len(batch)
			mu.Unlock()
		}
	}()
	for i := 0; i < 101; i++ {
		go func(i int) {
			time.Sleep(time.Duration(rand.Intn(700)) * time.Millisecond)
			q.Enqueue(i)
		}(i)
	}
	time.Sleep(time.Second)
	mu.Lock()
	require.Equal(t, 101, total)
	require.True(t, len(batches) > 4) // 101/25
	require.True(t, len(batches) < 21)
	mu.Unlock()
}

func TestBatchingQueue_CloseFlushesRemaining(t *testing.T) {
	// Elements still waiting for their batch must be emitted on Close (not dropped), and the
	// output channel must close so consumers can drain and exit
	q := util.NewBatchingQueue[int](100, time.Hour, 0)
	done := make(chan []int)
	go func() {
		var all []int
		for batch := range q.Dequeue() {
			all = append(all, batch...)
		}
		done <- all
	}()
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)
	q.Close()
	select {
	case all := <-done:
		require.Equal(t, []int{1, 2, 3}, all)
	case <-time.After(2 * time.Second):
		t.Fatal("output channel not closed after Close")
	}
	q.Enqueue(4) // Must not panic after Close
}

func TestBatchingQueue_EnqueueDoesNotBlockOnBusyConsumer(t *testing.T) {
	// A slow consumer (e.g. the message batch writer on a busy database) must not stall the
	// goroutines that enqueue (e.g. publish requests) while there is room in the buffer
	q := util.NewBatchingQueue[int](2, time.Hour, 10)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ { // 10 full batches, nobody reading yet
			q.Enqueue(i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked while the consumer was busy")
	}
	total := 0
	for i := 0; i < 10; i++ {
		total += len(<-q.Dequeue())
	}
	require.Equal(t, 20, total)
}
