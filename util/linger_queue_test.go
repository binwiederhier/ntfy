package util_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/util"
)

func TestLingerQueue_LingerFlush(t *testing.T) {
	// Items enqueued within the linger window are emitted as a single batch when it expires
	q := util.NewLingerQueue[int](16, 100, 0, nil, 50*time.Millisecond)
	defer q.Close()
	require.True(t, q.TryEnqueue(1))
	require.True(t, q.TryEnqueue(2))
	require.True(t, q.TryEnqueue(3))
	start := time.Now()
	batch := <-q.Dequeue()
	require.Equal(t, []int{1, 2, 3}, batch)
	require.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond) // Waited out the linger
}

func TestLingerQueue_MaxBatchFlush(t *testing.T) {
	// Hitting the count cap flushes early, before the linger expires
	q := util.NewLingerQueue[int](16, 5, 0, nil, time.Minute)
	defer q.Close()
	for i := 0; i < 12; i++ {
		require.True(t, q.TryEnqueue(i))
	}
	require.Len(t, <-q.Dequeue(), 5)
	require.Len(t, <-q.Dequeue(), 5)
	q.Close() // Flushes the remainder
	require.Len(t, <-q.Dequeue(), 2)
}

func TestLingerQueue_SizeCapFlush(t *testing.T) {
	// Hitting the byte cap flushes early, before count cap or linger
	q := util.NewLingerQueue(16, 100, 10, func(s string) int { return len(s) }, time.Minute)
	defer q.Close()
	require.True(t, q.TryEnqueue("aaaa"))
	require.True(t, q.TryEnqueue("bbbb"))
	require.True(t, q.TryEnqueue("cccc")) // 12 bytes >= 10 -> flush
	batch := <-q.Dequeue()
	require.Equal(t, []string{"aaaa", "bbbb", "cccc"}, batch)
}

func TestLingerQueue_TryEnqueueFull(t *testing.T) {
	// A full queue drops (returns false) instead of blocking the producer
	q := util.NewLingerQueue[int](1, 1, 0, nil, 0)
	defer q.Close()
	require.True(t, q.TryEnqueue(1))                       // Taken by the batcher, blocks emitting (no consumer)
	waitForCond(t, func() bool { return q.TryEnqueue(2) }) // Fills the buffer once slot frees
	require.False(t, q.TryEnqueue(3))                      // Buffer full, batcher blocked -> drop
}

func TestLingerQueue_CloseFlushesAndCloses(t *testing.T) {
	q := util.NewLingerQueue[int](16, 100, 0, nil, time.Minute)
	require.True(t, q.TryEnqueue(1))
	require.True(t, q.TryEnqueue(2))
	q.Close()
	require.Equal(t, []int{1, 2}, <-q.Dequeue()) // Remainder flushed without waiting out the linger
	_, ok := <-q.Dequeue()
	require.False(t, ok) // Channel closed
	require.False(t, q.TryEnqueue(3))
	q.Close() // Idempotent
}

func TestLingerQueue_LingerZeroImmediate(t *testing.T) {
	q := util.NewLingerQueue[int](16, 100, 0, nil, 0)
	defer q.Close()
	require.True(t, q.TryEnqueue(1))
	select {
	case batch := <-q.Dequeue():
		require.Contains(t, batch, 1)
	case <-time.After(time.Second):
		t.Fatal("expected immediate flush with zero linger")
	}
}

func waitForCond(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
