package server

import (
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/log"
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

func TestServer_Tail_SubscriberErrorDoesNotCrash(t *testing.T) {
	// Sending to a subscriber fails (its connection broke) and the failure is logged
	log.SetLevel(log.WarnLevel)
	t.Cleanup(func() { log.SetLevel(log.ErrorLevel) })
	s := newTailTestServer(t, newTestConfig(t, ""))
	topics, err := s.topicsFromIDs(nil, "mytopic")
	require.Nil(t, err)
	var calls atomic.Int32
	topics[0].Subscribe(func(_ *visitor, _ *model.Message) error {
		calls.Add(1)
		return errors.New("broken pipe")
	}, "", func() {})
	require.Nil(t, s.messageCache.AddMessages([]*model.Message{model.NewDefaultMessage("mytopic", "from peer")}))
	waitFor(t, func() bool { return calls.Load() == 1 })
	time.Sleep(200 * time.Millisecond) // The failure is logged after the call
}

func TestServer_TailLag(t *testing.T) {
	now := time.Unix(1700000100, 0)
	older, newer := model.NewDefaultMessage("mytopic", "older"), model.NewDefaultMessage("mytopic", "newer")
	older.Time, newer.Time = 1700000000, 1700000090
	require.Equal(t, 10*time.Second, tailLag([]*model.Message{newer, older}, now)) // The newest message read
	require.Equal(t, time.Duration(0), tailLag([]*model.Message{}, now))           // Nothing new: caught up
}

func TestServer_Tail_UnwrittenMarkerReplaysFromItsDeliveryHere(t *testing.T) {
	// The client's marker is not in the cache: the node that accepted it is cut off from the
	// database and its write hangs. This node saw the message go by (the fan-out still works),
	// so it replays from then, not the topic's whole history.
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTailTestServer(t, newTestConfig(t, databaseURL))
		older := model.NewDefaultMessage("mytopic", "older")
		older.Time = time.Now().Unix() - 30
		require.Nil(t, s.messageCache.AddMessages([]*model.Message{older}))
		time.Sleep(1500 * time.Millisecond) // The tail passes "older" before anyone subscribes
		topics, err := s.topicsFromIDs(nil, "mytopic")
		require.Nil(t, err)
		marker := model.NewDefaultMessage("mytopic", "marker")
		require.Nil(t, topics[0].Publish(nil, marker)) // Went out here, never written
		time.Sleep(100 * time.Millisecond)
		publishAndWaitForWrite(t, s, "mytopic", "later")

		rr := httptest.NewRecorder()
		cancel := subscribe(t, s, "/mytopic/json?since="+marker.ID, rr)
		time.Sleep(500 * time.Millisecond) // Less than the wait for an unwritten marker: the topic knows it, so no waiting
		cancel()
		counts := countMessages(t, rr)
		require.Equal(t, 0, counts["older"])
		require.GreaterOrEqual(t, counts["later"], 1) // Twice at most: the read and the tail's resend window
	})
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
