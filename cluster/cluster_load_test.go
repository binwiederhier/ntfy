package cluster

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
)

// TestMesh_Soak floods the mesh with concurrent publishers and asserts exact delivery: every
// message reaches the peer exactly once, nothing is dropped, and batching keeps the request
// count far below the message count. Skipped unless NTFY_TEST_SOAK is set (it takes a few
// seconds and is meant for pre-deploy verification, not the regular suite).
func TestMesh_Soak(t *testing.T) {
	if os.Getenv("NTFY_TEST_SOAK") == "" {
		t.Skip("NTFY_TEST_SOAK not set")
	}
	// ~1000 msg/s aggregate (10x the ntfy.sh peak of ~88 msg/s): each publisher paces itself to
	// 100 msg/s. Unthrottled publishing intentionally overruns the bounded per-peer queue (load
	// shedding by design), so a zero-drop assertion only holds below the drain ceiling.
	const (
		publishers           = 10
		messagesPerPublisher = 300
		publishInterval      = 10 * time.Millisecond
		total                = publishers * messagesPerPublisher
	)
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	var mu sync.Mutex
	received := make(map[string]int, total) // message body -> count, to catch duplicates
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		messages, err := unmarshalFanoutBody(body, 1<<20)
		require.Nil(t, err)
		mu.Lock()
		requests++
		for _, m := range messages {
			received[m.Message]++
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.BatchLinger = 50 * time.Millisecond
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	_, err = pool.Exec(upsertNodeQuery, "node-peer", srv.URL, time.Now().Unix())
	require.Nil(t, err)
	start := time.Now()
	var wg sync.WaitGroup
	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			ticker := time.NewTicker(publishInterval)
			defer ticker.Stop()
			for i := 0; i < messagesPerPublisher; i++ {
				require.Nil(t, mesh.Broadcast(model.NewDefaultMessage("mytopic", fmt.Sprintf("p%d-m%d", p, i))))
				<-ticker.C
			}
		}(p)
	}
	wg.Wait()
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == total
	})
	elapsed := time.Since(start)
	mu.Lock()
	defer mu.Unlock()
	for body, count := range received {
		require.Equalf(t, 1, count, "message %s delivered %d times", body, count)
	}
	require.Less(t, requests, total/10, "expected strong batching under load")
	t.Logf("soak: %d messages, %d requests (%.1f msgs/request), %.0f msgs/s",
		total, requests, float64(total)/float64(requests), float64(total)/elapsed.Seconds())
}

// BenchmarkBroadcast measures the publish-path cost of Broadcast: marshal + peer lookup (cached)
// + enqueue. The peer never drains, so enqueued fragments are dropped once the queue fills;
// the benchmark measures the hot path, not HTTP delivery.
func BenchmarkBroadcast(b *testing.B) {
	if os.Getenv("NTFY_TEST_DATABASE_URL") == "" {
		b.Skip("NTFY_TEST_DATABASE_URL not set")
	}
	schemaDSN := dbtest.CreateTestPostgresSchema(b)
	pool := openTestPool(b, schemaDSN)
	conf := newTestMeshConfig("node-a", "http://127.0.0.1:1")
	conf.BatchLinger = time.Minute // Never flush; we measure enqueue only
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(b, err)
	defer mesh.Close()
	_, err = pool.Exec(upsertNodeQuery, "node-peer", "http://127.0.0.1:1", time.Now().Unix())
	require.Nil(b, err)
	m := model.NewDefaultMessage("mytopic", "benchmark message body of typical size for a push")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := mesh.Broadcast(m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeFanout measures the receive-path cost of decoding a 100-message NDJSON body.
func BenchmarkDecodeFanout(b *testing.B) {
	frags := make([][]byte, 100)
	for i := range frags {
		frag, err := marshalMessage(model.NewDefaultMessage("mytopic", fmt.Sprintf("benchmark message %d", i)))
		require.Nil(b, err)
		frags[i] = frag
	}
	body := assembleFanoutBody(frags)
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messages, err := unmarshalFanoutBody(body, 1<<20)
		if err != nil || len(messages) != 100 {
			b.Fatal("decode failed")
		}
	}
}
