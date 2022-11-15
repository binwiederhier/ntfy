package util_test

import (
	"fmt"
	"heckel.io/ntfy/util"
	"math/rand"
	"testing"
	"time"
)

func TestConcurrentQueue_Next(t *testing.T) {
	q := util.NewBatchingQueue[int](25, 200*time.Millisecond)
	go func() {
		for batch := range q.Pop() {
			fmt.Printf("Batch of %d items\n", len(batch))
		}
	}()
	for i := 0; i < 1000; i++ {
		go func(i int) {
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			q.Push(i)
		}(i)
	}
	time.Sleep(2 * time.Second)
}
