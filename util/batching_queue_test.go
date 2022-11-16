package util_test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/util"
	"math/rand"
	"testing"
	"time"
)

func TestBatchingQueue_InfTimeout(t *testing.T) {
	q := util.NewBatchingQueue[int](25, 1*time.Hour)
	batches := make([][]int, 0)
	total := 0
	go func() {
		for batch := range q.Dequeue() {
			batches = append(batches, batch)
			total += len(batch)
		}
	}()
	for i := 0; i < 101; i++ {
		go q.Enqueue(i)
	}
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, 100, total) // One is missing, stuck in the last batch!
	require.Equal(t, 4, len(batches))
}

func TestBatchingQueue_WithTimeout(t *testing.T) {
	q := util.NewBatchingQueue[int](25, 100*time.Millisecond)
	batches := make([][]int, 0)
	total := 0
	go func() {
		for batch := range q.Dequeue() {
			batches = append(batches, batch)
			total += len(batch)
		}
	}()
	for i := 0; i < 101; i++ {
		go func(i int) {
			time.Sleep(time.Duration(rand.Intn(700)) * time.Millisecond)
			q.Enqueue(i)
		}(i)
	}
	time.Sleep(time.Second)
	fmt.Println(len(batches))
	fmt.Println(batches)
	require.Equal(t, 101, total)
	require.True(t, len(batches) > 4) // 101/25
	require.True(t, len(batches) < 21)
}
