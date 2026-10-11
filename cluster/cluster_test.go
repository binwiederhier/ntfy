package cluster

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
)

func TestDeliver_RoundTrip(t *testing.T) {
	// The fan-out body is NDJSON: one apiDeliverMessage per line, joined from pre-marshaled
	// fragments; the origin travels in a header, not the body
	m1 := model.NewDefaultMessage("mytopic", "my message")
	m1.Sender = netip.MustParseAddr("1.2.3.4")
	m1.User = "u_abc"
	m2 := model.NewDefaultMessage("othertopic", "other message")
	frag1, err := marshalMessage(m1)
	require.Nil(t, err)
	frag2, err := marshalMessage(m2)
	require.Nil(t, err)
	messages, err := unmarshalMessageBody(assembleMessageBody([][]byte{frag1, frag2}), 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "mytopic", messages[0].Topic)
	require.Equal(t, "my message", messages[0].Message)
	// Sender and User are json:"-" on model.Message; the lines must carry and reattach them
	require.Equal(t, netip.MustParseAddr("1.2.3.4"), messages[0].Sender)
	require.Equal(t, "u_abc", messages[0].User)
	require.Equal(t, "othertopic", messages[1].Topic)
	require.False(t, messages[1].Sender.IsValid())
}

func TestDeliver_SingleMessage(t *testing.T) {
	// A single message is just a one-line body; there is no separate single-message format
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "hi"))
	require.Nil(t, err)
	messages, err := unmarshalMessageBody(assembleMessageBody([][]byte{frag}), 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 1)
}

func TestDeliver_MalformedLinesSkipped(t *testing.T) {
	// Fan-out is fire-and-forget: a malformed or message-less line is skipped (and logged), the
	// remaining lines are still delivered
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "good"))
	require.Nil(t, err)
	body := []byte("this is not json\n{\"sender\":\"1.2.3.4\"}\n" + string(frag) + "\n\n")
	messages, err := unmarshalMessageBody(body, 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "good", messages[0].Message)
}

// unmarshalMessageBody is a test helper collecting the messages of an NDJSON message body.
func unmarshalMessageBody(body []byte, maxLineBytes int) ([]*model.Message, error) {
	var messages []*model.Message
	err := decodeMessageBody(bytes.NewReader(body), maxLineBytes, func(m *model.Message) {
		messages = append(messages, m)
	})
	return messages, err
}

func TestNop(t *testing.T) {
	b, err := New(&Config{}, nil, nil) // not enabled -> nop cluster, no database required
	require.Nil(t, err)
	require.IsType(t, &nopCluster{}, b)
	require.Nil(t, b.ForwardMessage(model.NewDefaultMessage("mytopic", "hi")))
	// A single node is trivially the leader, so leader-gated jobs run without special-casing
	require.True(t, b.IsLeader())
	require.True(t, b.Healthy())
	rr := httptest.NewRecorder()
	b.ServeHTTP(rr, httptest.NewRequest("POST", MessagePath, nil))
	require.Equal(t, 404, rr.Code)
	require.Nil(t, b.Close())
}

func TestNew_EnabledRequiresDatabase(t *testing.T) {
	_, err := New(&Config{Enabled: true, Secret: "secret"}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database")
}

func TestDeliver_LineLimitBelowTheScanBufferIsEnforced(t *testing.T) {
	// The configured limit is what the sender's own message size limit implies (MessageSizeLimit
	// * 4 + 1024, so about 17KB for ntfy's default), which is well under bufio's buffer. A
	// scanner whose initial buffer is bigger than the limit accepts tokens up to the buffer
	// instead, so the limit silently did not apply to anything smaller than 64KB.
	const limit = 512
	big := model.NewDefaultMessage("mytopic", strings.Repeat("x", 4*limit))
	line, err := marshalMessage(big)
	require.Nil(t, err)
	require.Greater(t, len(line), limit) // The line under test really is over the limit

	var delivered []*model.Message
	err = decodeMessageBody(bytes.NewReader(line), limit, func(m *model.Message) {
		delivered = append(delivered, m)
	})

	require.Error(t, err)
	require.Empty(t, delivered, "a line over the configured limit was delivered")
}

func TestDeliver_LineOfExactlyTheLimitIsAccepted(t *testing.T) {
	// maxLineBytes is inclusive: bufio's own limit is exclusive, so this is off by one unless
	// the decoder accounts for it, and a peer sending a message right at the limit would have
	// its whole batch rejected
	line, err := marshalMessage(model.NewDefaultMessage("mytopic", "hello"))
	require.Nil(t, err)
	limit := len(bytes.TrimRight(line, "\n"))

	var delivered []*model.Message
	collect := func(m *model.Message) { delivered = append(delivered, m) }
	require.Nil(t, decodeMessageBody(bytes.NewReader(line), limit, collect))
	require.Len(t, delivered, 1)

	// One byte over, and the batch fails rather than delivering a line past the limit
	delivered = nil
	require.Error(t, decodeMessageBody(bytes.NewReader(line), limit-1, collect))
	require.Empty(t, delivered)
}

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
		messages, err := unmarshalMessageBody(body, 1<<20)
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
	conf.NodeTTL = time.Minute // The fake peer never heartbeats; liveness is not under test here
	registerFakePeer(t, pool, "node-peer", srv.URL)
	mesh, err := newMeshCluster(conf, pool, nil)
	require.Nil(t, err)
	defer mesh.Close()
	start := time.Now()
	var wg sync.WaitGroup
	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			ticker := time.NewTicker(publishInterval)
			defer ticker.Stop()
			for i := 0; i < messagesPerPublisher; i++ {
				require.Nil(t, mesh.ForwardMessage(model.NewDefaultMessage("mytopic", fmt.Sprintf("p%d-m%d", p, i))))
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

// BenchmarkForwardMessage measures the publish-path cost of ForwardMessage: marshal + peer lookup (cached)
// + enqueue. The peer never drains, so enqueued fragments are dropped once the queue fills;
// the benchmark measures the hot path, not HTTP delivery.
func BenchmarkForwardMessage(b *testing.B) {
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
	registerFakePeer(b, pool, "node-peer", "http://127.0.0.1:1")
	m := model.NewDefaultMessage("mytopic", "benchmark message body of typical size for a push")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := mesh.ForwardMessage(m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeFanout measures the receive-path cost of decoding a 100-message NDJSON body.
func BenchmarkDecodeFanout(b *testing.B) {
	frags := make([][]byte, 100)
	for i := range frags {
		data, err := marshalMessage(model.NewDefaultMessage("mytopic", fmt.Sprintf("benchmark message %d", i)))
		require.Nil(b, err)
		frags[i] = data
	}
	body := assembleMessageBody(frags)
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messages, err := unmarshalMessageBody(body, 1<<20)
		if err != nil || len(messages) != 100 {
			b.Fatal("decode failed")
		}
	}
}
