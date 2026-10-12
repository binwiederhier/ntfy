package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/model"
)

func TestServer_Tail_MessageWrittenAfterTheSubscriberReconnected(t *testing.T) {
	// A message published while the client was away goes out live to nobody, and still sits in
	// the write batch when the client reconnects, so the database read misses it. The tail sends
	// it once it is written.
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTailTestServer(t, newTestConfig(t, databaseURL))
		first := publishAndWaitForWrite(t, s, "mytopic", "first")
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "in flight", nil).Code) // Queued, not written

		rr := httptest.NewRecorder()
		cancel := subscribe(t, s, "/mytopic/json?since="+first.ID, rr)
		time.Sleep(3 * time.Second)
		cancel()
		require.Equal(t, 1, countMessages(t, rr)["in flight"]) // "first" may come again: it went out just before
	})
}

func TestServer_Tail_DatabaseStalledLongerThanTheReconnect(t *testing.T) {
	// The batch write hangs (a locked table, an overloaded database), so the message is written
	// long after the client reconnected. The tail sends it whenever that is.
	conf := newTestConfig(t, dbtest.CreateTestPostgresSchema(t))
	s := newTailTestServer(t, conf)
	first := publishAndWaitForWrite(t, s, "mytopic", "first")
	lockDB, err := pg.Open(conf.DatabaseURL)
	require.Nil(t, err)
	defer lockDB.DB.Close()
	tx, err := lockDB.DB.Begin()
	require.Nil(t, err)
	t.Cleanup(func() { tx.Rollback() })
	_, err = tx.Exec("LOCK TABLE message IN EXCLUSIVE MODE") // Blocks writes, not reads
	require.Nil(t, err)
	require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "stalled", nil).Code)

	rr := httptest.NewRecorder()
	cancel := subscribe(t, s, "/mytopic/json?since="+first.ID, rr)
	time.Sleep(3 * time.Second)
	require.Nil(t, tx.Rollback())
	time.Sleep(2 * time.Second)
	cancel()
	require.Equal(t, 1, countMessages(t, rr)["stalled"])
}

func TestServer_Tail_MessageWrittenByAnotherNode(t *testing.T) {
	// A peer wrote the message, but its fan-out to this node was lost
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTailTestServer(t, newTestConfig(t, databaseURL))
		rr := httptest.NewRecorder()
		cancel := subscribe(t, s, "/mytopic/json", rr)
		require.Nil(t, s.messageCache.AddMessages([]*model.Message{model.NewDefaultMessage("mytopic", "from peer")}))
		time.Sleep(2 * time.Second)
		cancel()
		require.Equal(t, map[string]int{"from peer": 1}, countMessages(t, rr))
	})
}

func TestServer_Tail_DeliveredLiveIsNotSentAgain(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTailTestServer(t, newTestConfig(t, databaseURL))
		rr := httptest.NewRecorder()
		cancel := subscribe(t, s, "/mytopic/json", rr)
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "live", nil).Code)
		time.Sleep(3 * time.Second)
		cancel()
		require.Equal(t, map[string]int{"live": 1}, countMessages(t, rr))
	})
}

func TestServer_Tail_ClusterMarkerWrittenBeforeAnEarlierMessage(t *testing.T) {
	// Cluster nodes write their batches independently, so a message the client never got (its
	// fan-out was late) can have a lower row id than the client's since=<id> marker. In a cluster,
	// the marker is read as a time, a little earlier than the marker message.
	conf := newTestConfig(t, dbtest.CreateTestPostgresSchema(t))
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1" // Enables clustering; not bound since Run() is not called
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	s := newTailTestServer(t, conf)
	now := time.Now().Unix()
	early := model.NewDefaultMessage("mytopic", "early")
	early.Time = now - 1
	marker := model.NewDefaultMessage("mytopic", "marker")
	marker.Time = now
	require.Nil(t, s.messageCache.AddMessages([]*model.Message{early, marker})) // early gets the lower row id
	time.Sleep(2 * time.Second)                                                 // The tail has passed both

	rr := httptest.NewRecorder()
	cancel := subscribe(t, s, "/mytopic/json?since="+marker.ID, rr)
	time.Sleep(time.Second)
	cancel()
	require.Equal(t, 1, countMessages(t, rr)["early"])
}

func newTailTestServer(t *testing.T, conf *Config) *Server {
	conf.CacheBatchSize = 0
	conf.CacheBatchTimeout = 500 * time.Millisecond
	return newTestServer(t, conf)
}

func publishAndWaitForWrite(t *testing.T, s *Server, topic, body string) *model.Message {
	response := request(t, s, "PUT", "/"+topic, body, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	waitFor(t, func() bool {
		_, err := s.messageCache.Message(m.ID)
		return err == nil
	})
	return m
}

func countMessages(t *testing.T, rr *httptest.ResponseRecorder) map[string]int {
	counts := make(map[string]int)
	for _, m := range toMessages(t, rr.Body.String()) {
		if m.Event == model.MessageEvent {
			counts[m.Message]++
		}
	}
	return counts
}
