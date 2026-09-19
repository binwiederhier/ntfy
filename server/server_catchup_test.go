package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
)

// newCatchUpTestServer returns a server whose cache writes in batches of two, so a test controls
// exactly when a message reaches the database (the second publish flushes the pair)
func newCatchUpTestServer(t *testing.T) *Server {
	conf := newTestConfig(t, "")
	conf.CacheBatchSize = 2
	conf.CacheBatchTimeout = time.Hour
	s := newTestServer(t, conf)
	s.catchUpDelay = 500 * time.Millisecond
	return s
}

// publishStored publishes two messages (one full batch) and waits until both are in the cache
func publishStored(t *testing.T, s *Server, topic string) (*model.Message, *model.Message) {
	m1 := toMessage(t, request(t, s, "PUT", "/"+topic, "stored 1", nil).Body.String())
	m2 := toMessage(t, request(t, s, "PUT", "/"+topic, "stored 2", nil).Body.String())
	waitFor(t, func() bool {
		_, err := s.messageCache.Message(m2.ID)
		return err == nil
	})
	return m1, m2
}

func countByBody(messages []*model.Message) map[string]int {
	counts := make(map[string]int)
	for _, m := range messages {
		if m.Event == model.MessageEvent {
			counts[m.Message]++
		}
	}
	return counts
}

func TestServer_SubscribeCatchUp_MessageNotYetStoredAtReconnect(t *testing.T) {
	// The reconnect gap: a message published while the client was away is fanned out live (to
	// nobody) but still sits in the cache write batch when the client re-subscribes with
	// since=<last id>, so the initial replay misses it. The catch-up replay must deliver it.
	s := newCatchUpTestServer(t)
	_, last := publishStored(t, s, "mytopic")
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "in flight", nil).Code) // Queued, not stored

	rr := httptest.NewRecorder()
	cancel := subscribe(t, s, "/mytopic/json?since="+last.ID, rr)
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "live", nil).Code) // Flushes the batch
	time.Sleep(time.Second)
	cancel()

	// "stored 1" lies in the overlap window before the marker and is re-sent once: the price of
	// covering the lower-row-id case (clients dedupe by ID). Nothing arrives twice on this connection.
	counts := countByBody(toMessages(t, rr.Body.String()))
	require.Equal(t, map[string]int{"in flight": 1, "live": 1, "stored 1": 1}, counts)
}

func TestServer_SubscribeCatchUp_MessageStoredWithLowerRowID(t *testing.T) {
	// Cluster nodes flush their batches independently, so a message published around the
	// marker can be stored with a LOWER row id than the marker and is skipped by id > marker.
	// The catch-up replays a short time window before the marker; the marker itself and
	// anything already sent on this connection are not sent again.
	s := newCatchUpTestServer(t)
	now := time.Now().Unix()
	early := model.NewDefaultMessage("mytopic", "stored first")
	early.Time = now - 1
	marker := model.NewDefaultMessage("mytopic", "marker")
	marker.Time = now
	require.Nil(t, s.messageCache.AddMessages([]*model.Message{early, marker}))

	rr := httptest.NewRecorder()
	cancel := subscribe(t, s, "/mytopic/json?since="+marker.ID, rr)
	time.Sleep(time.Second)
	cancel()

	counts := countByBody(toMessages(t, rr.Body.String()))
	require.Equal(t, map[string]int{"stored first": 1}, counts)
}

func TestServer_SubscribeCatchUp_WebSocket(t *testing.T) {
	s := newCatchUpTestServer(t)
	_, last := publishStored(t, s, "mytopic")
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "in flight", nil).Code)

	public := httptest.NewServer(http.HandlerFunc(s.handle))
	defer public.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(public.URL, "http")+"/mytopic/ws?since="+last.ID, nil)
	require.Nil(t, err)
	defer conn.Close()
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "live", nil).Code)

	var received []*model.Message
	conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			break
		}
		received = append(received, toMessage(t, string(frame)))
	}
	require.Equal(t, map[string]int{"in flight": 1, "live": 1, "stored 1": 1}, countByBody(received))
}
