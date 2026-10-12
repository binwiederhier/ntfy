package message_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func TestTail_ReturnsWhatWasWrittenSinceTheLastCall(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		require.Nil(t, s.AddMessages([]*model.Message{model.NewDefaultMessage("mytopic", "before")}))
		tail, err := s.Tail()
		require.Nil(t, err)
		requireTail(t, tail)

		require.Nil(t, s.AddMessages([]*model.Message{
			model.NewDefaultMessage("mytopic", "one"),
			model.NewDefaultMessage("othertopic", "two"),
		}))
		requireTail(t, tail, "one", "two")
		requireTail(t, tail)

		require.Nil(t, s.AddMessages([]*model.Message{model.NewDefaultMessage("mytopic", "three")}))
		requireTail(t, tail, "three")
	})
}

func TestTail_SkipsScheduledMessages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		tail, err := s.Tail()
		require.Nil(t, err)
		scheduled := model.NewDefaultMessage("mytopic", "later")
		scheduled.Time = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessages([]*model.Message{scheduled, model.NewDefaultMessage("mytopic", "now")}))
		requireTail(t, tail, "now")
	})
}

func TestTail_ReturnsRowsCommittedOutOfOrder(t *testing.T) {
	// Concurrent writers (cluster nodes) commit in their own time, so a row with a lower id can
	// become visible after one with a higher id. The tail must not skip it.
	testDB := dbtest.CreateTestPostgres(t)
	s, err := message.NewPostgresStore(testDB, 0, 0)
	require.Nil(t, err)
	tail, err := s.Tail()
	require.Nil(t, err)

	// Take a row id in a transaction that stays open, like a slow batch on another node
	tx, err := testDB.Primary().Begin()
	require.Nil(t, err)
	t.Cleanup(func() { tx.Rollback() }) // Before the schema is dropped, which would wait for it
	var rowID int64
	require.Nil(t, tx.QueryRow(`SELECT nextval(pg_get_serial_sequence('message', 'id'))`).Scan(&rowID))

	// A later row commits first
	require.Nil(t, s.AddMessages([]*model.Message{model.NewDefaultMessage("mytopic", "second")}))
	requireTail(t, tail, "second")

	// The earlier row commits now
	_, err = tx.Exec(`
		INSERT INTO message (id, mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, published)
		VALUES ($1, 'first0000000', 'first0000000', $2, 'message', 0, 'mytopic', 'first', '', 0, '', '', '', '', '', '', 0, 0, '', '', '', '', '', TRUE)
	`, rowID, time.Now().Unix())
	require.Nil(t, err)
	require.Nil(t, tx.Commit())
	requireTail(t, tail, "first")
	requireTail(t, tail)
}

func requireTail(t *testing.T, tail *message.Tail, expected ...string) {
	t.Helper()
	messages, err := tail.Next()
	require.Nil(t, err)
	actual := make([]string, 0)
	for _, m := range messages {
		actual = append(actual, m.Message)
	}
	require.Equal(t, append([]string{}, expected...), actual)
}
