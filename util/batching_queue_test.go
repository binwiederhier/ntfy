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
	q := util.NewBatchingQueue[int](25, 1*time.Hour)
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
	q := util.NewBatchingQueue[int](25, 100*time.Millisecond)
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
	q := util.NewBatchingQueue[int](100, time.Hour)
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
