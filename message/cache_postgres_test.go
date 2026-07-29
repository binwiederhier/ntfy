package message_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func TestPostgresStore_Migration_From14(t *testing.T) {
	// A pre-framework database at version 14: full v14 schema, version tracked in the
	// hand-rolled schema_version table, and no idx_message_attachment_expires yet
	testDB := dbtest.CreateTestPostgres(t)
	_, err := testDB.Exec(`
		CREATE TABLE message (
			id BIGSERIAL PRIMARY KEY,
			mid TEXT NOT NULL,
			sequence_id TEXT NOT NULL,
			time BIGINT NOT NULL,
			event TEXT NOT NULL,
			expires BIGINT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			icon TEXT NOT NULL,
			actions TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size BIGINT NOT NULL,
			attachment_expires BIGINT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_deleted BOOLEAN NOT NULL DEFAULT FALSE,
			sender TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published BOOLEAN NOT NULL DEFAULT FALSE
		);
		CREATE INDEX idx_message_mid ON message (mid);
		CREATE INDEX idx_message_sequence_id ON message (sequence_id);
		CREATE INDEX idx_message_topic_published_time ON message (topic, published, time, id);
		CREATE INDEX idx_message_published_expires ON message (published, expires);
		CREATE INDEX idx_message_sender_attachment_expires ON message (sender, attachment_expires) WHERE user_id = '';
		CREATE INDEX idx_message_user_id_attachment_expires ON message (user_id, attachment_expires);
		CREATE TABLE message_stats (key TEXT PRIMARY KEY, value BIGINT);
		INSERT INTO message_stats (key, value) VALUES ('messages', 0);
		CREATE TABLE schema_version (store TEXT PRIMARY KEY, version INT NOT NULL);
		INSERT INTO schema_version (store, version) VALUES ('message', 14);
	`)
	require.Nil(t, err)
	store, err := message.NewPostgresStore(testDB, 0, 0)
	require.Nil(t, err)
	// The 14 -> 15 step ran: version bumped, partial index created
	var version int
	require.Nil(t, testDB.QueryRow(`SELECT version FROM schema_version WHERE store = 'message'`).Scan(&version))
	require.Equal(t, 15, version)
	var indexCount int
	require.Nil(t, testDB.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'idx_message_attachment_expires' AND schemaname = current_schema()`).Scan(&indexCount))
	require.Equal(t, 1, indexCount)
	// And the store works
	require.Nil(t, store.AddMessage(model.NewDefaultMessage("mytopic", "hi there")))
	messages, err := store.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Len(t, messages, 1)

	// The migrated database must be structurally identical to a freshly created one
	freshDB := dbtest.CreateTestPostgres(t)
	_, err = message.NewPostgresStore(freshDB, 0, 0)
	require.Nil(t, err)
	require.Equal(t, dbtest.PostgresSchema(t, freshDB), dbtest.PostgresSchema(t, testDB))
}
