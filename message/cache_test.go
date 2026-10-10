package message_test

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func newSqliteTestStore(t *testing.T) *message.Cache {
	filename := filepath.Join(t.TempDir(), "cache.db")
	s, err := message.NewSQLiteStore(filename, "", time.Hour, 0, 0, false)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func newMemTestStore(t *testing.T) *message.Cache {
	s, err := message.NewMemStore()
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestPostgresStore(t *testing.T) *message.Cache {
	testDB := dbtest.CreateTestPostgres(t)
	store, err := message.NewPostgresStore(testDB, 0, 0)
	require.Nil(t, err)
	return store
}

func forEachBackend(t *testing.T, f func(t *testing.T, s *message.Cache)) {
	t.Run("sqlite", func(t *testing.T) {
		f(t, newSqliteTestStore(t))
	})
	t.Run("mem", func(t *testing.T) {
		f(t, newMemTestStore(t))
	})
	t.Run("postgres", func(t *testing.T) {
		f(t, newTestPostgresStore(t))
	})
}

func TestStore_Messages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m1 := model.NewDefaultMessage("mytopic", "my message")
		m1.Time = 1

		m2 := model.NewDefaultMessage("mytopic", "my other message")
		m2.Time = 2

		require.Nil(t, s.AddMessage(m1))
		require.Nil(t, s.AddMessage(model.NewDefaultMessage("example", "my example message")))
		require.Nil(t, s.AddMessage(m2))

		// Adding invalid
		require.Equal(t, model.ErrUnexpectedMessageType, s.AddMessage(model.NewKeepaliveMessage("mytopic"))) // These should not be added!
		require.Equal(t, model.ErrUnexpectedMessageType, s.AddMessage(model.NewOpenMessage("example")))      // These should not be added!

		// count
		count, err := s.MessagesCount()
		require.Nil(t, err)
		require.Equal(t, 3, count)

		// mytopic: since all
		messages, _ := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Equal(t, 2, len(messages))
		require.Equal(t, "my message", messages[0].Message)
		require.Equal(t, "mytopic", messages[0].Topic)
		require.Equal(t, model.MessageEvent, messages[0].Event)
		require.Equal(t, "", messages[0].Title)
		require.Equal(t, 0, messages[0].Priority)
		require.Nil(t, messages[0].Tags)
		require.Equal(t, "my other message", messages[1].Message)

		// mytopic: since none
		messages, _ = s.Messages("mytopic", model.SinceNoMessages, false)
		require.Empty(t, messages)

		// mytopic: since m1 (by ID)
		messages, _ = s.Messages("mytopic", model.NewSinceID(m1.ID), false)
		require.Equal(t, 1, len(messages))
		require.Equal(t, m2.ID, messages[0].ID)
		require.Equal(t, "my other message", messages[0].Message)
		require.Equal(t, "mytopic", messages[0].Topic)

		// mytopic: since 2
		messages, _ = s.Messages("mytopic", model.NewSinceTime(2), false)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "my other message", messages[0].Message)

		// mytopic: latest
		messages, _ = s.Messages("mytopic", model.SinceLatestMessage, false)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "my other message", messages[0].Message)

		// example: since all
		messages, _ = s.Messages("example", model.SinceAllMessages, false)
		require.Equal(t, "my example message", messages[0].Message)

		// non-existing: since all
		messages, _ = s.Messages("doesnotexist", model.SinceAllMessages, false)
		require.Empty(t, messages)
	})
}

func TestStore_MessagesLock(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		var wg sync.WaitGroup
		for i := 0; i < 5000; i++ {
			wg.Add(1)
			go func() {
				assert.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "test message")))
				wg.Done()
			}()
		}
		wg.Wait()
	})
}

func TestStore_MessagesScheduled(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m1 := model.NewDefaultMessage("mytopic", "message 1")
		m2 := model.NewDefaultMessage("mytopic", "message 2")
		m2.Time = time.Now().Add(time.Hour).Unix()
		m3 := model.NewDefaultMessage("mytopic", "message 3")
		m3.Time = time.Now().Add(time.Minute).Unix() // earlier than m2!
		m4 := model.NewDefaultMessage("mytopic2", "message 4")
		m4.Time = time.Now().Add(time.Minute).Unix()
		require.Nil(t, s.AddMessage(m1))
		require.Nil(t, s.AddMessage(m2))
		require.Nil(t, s.AddMessage(m3))

		messages, _ := s.Messages("mytopic", model.SinceAllMessages, false) // exclude scheduled
		require.Equal(t, 1, len(messages))
		require.Equal(t, "message 1", messages[0].Message)

		messages, _ = s.Messages("mytopic", model.SinceAllMessages, true) // include scheduled
		require.Equal(t, 3, len(messages))
		require.Equal(t, "message 1", messages[0].Message)
		require.Equal(t, "message 3", messages[1].Message) // Order!
		require.Equal(t, "message 2", messages[2].Message)

		messages, _ = s.MessagesDue()
		require.Empty(t, messages)
	})
}

func TestStore_Topics(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		require.Nil(t, s.AddMessage(model.NewDefaultMessage("topic1", "my example message")))
		require.Nil(t, s.AddMessage(model.NewDefaultMessage("topic2", "message 1")))
		require.Nil(t, s.AddMessage(model.NewDefaultMessage("topic2", "message 2")))
		require.Nil(t, s.AddMessage(model.NewDefaultMessage("topic2", "message 3")))

		topics, err := s.Topics()
		if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, 2, len(topics))
		require.Contains(t, topics, "topic1")
		require.Contains(t, topics, "topic2")
	})
}

func TestStore_MessagesTagsPrioAndTitle(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m := model.NewDefaultMessage("mytopic", "some message")
		m.Tags = []string{"tag1", "tag2"}
		m.Priority = 5
		m.Title = "some title"
		require.Nil(t, s.AddMessage(m))

		messages, _ := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Equal(t, []string{"tag1", "tag2"}, messages[0].Tags)
		require.Equal(t, 5, messages[0].Priority)
		require.Equal(t, "some title", messages[0].Title)
	})
}

func TestStore_MessagesSinceID(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m1 := model.NewDefaultMessage("mytopic", "message 1")
		m1.Time = 100
		m2 := model.NewDefaultMessage("mytopic", "message 2")
		m2.Time = 200
		m3 := model.NewDefaultMessage("mytopic", "message 3")
		m3.Time = time.Now().Add(time.Hour).Unix() // Scheduled, in the future, later than m7 and m5
		m4 := model.NewDefaultMessage("mytopic", "message 4")
		m4.Time = 400
		m5 := model.NewDefaultMessage("mytopic", "message 5")
		m5.Time = time.Now().Add(time.Minute).Unix() // Scheduled, in the future, later than m7
		m6 := model.NewDefaultMessage("mytopic", "message 6")
		m6.Time = 600
		m7 := model.NewDefaultMessage("mytopic", "message 7")
		m7.Time = 700

		require.Nil(t, s.AddMessage(m1))
		require.Nil(t, s.AddMessage(m2))
		require.Nil(t, s.AddMessage(m3))
		require.Nil(t, s.AddMessage(m4))
		require.Nil(t, s.AddMessage(m5))
		require.Nil(t, s.AddMessage(m6))
		require.Nil(t, s.AddMessage(m7))

		// Case 1: Since ID exists, exclude scheduled
		messages, _ := s.Messages("mytopic", model.NewSinceID(m2.ID), false)
		require.Equal(t, 3, len(messages))
		require.Equal(t, "message 4", messages[0].Message)
		require.Equal(t, "message 6", messages[1].Message) // Not scheduled m3/m5!
		require.Equal(t, "message 7", messages[2].Message)

		// Case 2: Since ID exists, include scheduled
		messages, _ = s.Messages("mytopic", model.NewSinceID(m2.ID), true)
		require.Equal(t, 5, len(messages))
		require.Equal(t, "message 4", messages[0].Message)
		require.Equal(t, "message 6", messages[1].Message)
		require.Equal(t, "message 7", messages[2].Message)
		require.Equal(t, "message 5", messages[3].Message) // Order!
		require.Equal(t, "message 3", messages[4].Message) // Order!

		// Case 3: Since ID does not exist (-> Return all messages), include scheduled
		messages, _ = s.Messages("mytopic", model.NewSinceID("doesntexist"), true)
		require.Equal(t, 7, len(messages))
		require.Equal(t, "message 1", messages[0].Message)
		require.Equal(t, "message 2", messages[1].Message)
		require.Equal(t, "message 4", messages[2].Message)
		require.Equal(t, "message 6", messages[3].Message)
		require.Equal(t, "message 7", messages[4].Message)
		require.Equal(t, "message 5", messages[5].Message) // Order!
		require.Equal(t, "message 3", messages[6].Message) // Order!

		// Case 4: Since ID exists and is last message (-> Return no messages), exclude scheduled
		messages, _ = s.Messages("mytopic", model.NewSinceID(m7.ID), false)
		require.Equal(t, 0, len(messages))

		// Case 5: Since ID exists and is last message (-> Return no messages), include scheduled
		messages, _ = s.Messages("mytopic", model.NewSinceID(m7.ID), true)
		require.Equal(t, 2, len(messages))
		require.Equal(t, "message 5", messages[0].Message)
		require.Equal(t, "message 3", messages[1].Message)
	})
}

func TestStore_Prune(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		now := time.Now().Unix()

		m1 := model.NewDefaultMessage("mytopic", "my message")
		m1.Time = now - 10
		m1.Expires = now - 5

		m2 := model.NewDefaultMessage("mytopic", "my other message")
		m2.Time = now - 5
		m2.Expires = now + 5 // In the future

		m3 := model.NewDefaultMessage("another_topic", "and another one")
		m3.Time = now - 12
		m3.Expires = now - 2

		require.Nil(t, s.AddMessage(m1))
		require.Nil(t, s.AddMessage(m2))
		require.Nil(t, s.AddMessage(m3))

		count, err := s.MessagesCount()
		require.Nil(t, err)
		require.Equal(t, 3, count)

		deleted, err := s.DeleteExpiredMessages(10)
		require.Nil(t, err)
		require.Equal(t, int64(2), deleted)

		count, err = s.MessagesCount()
		require.Nil(t, err)
		require.Equal(t, 1, count)

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "my other message", messages[0].Message)
	})
}

func TestStore_Attachments(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		expires1 := time.Now().Add(-4 * time.Hour).Unix() // Expired
		m := model.NewDefaultMessage("mytopic", "flower for you")
		m.ID = "m1"
		m.SequenceID = "m1"
		m.Sender = netip.MustParseAddr("1.2.3.4")
		m.Attachment = &model.Attachment{
			Name:    "flower.jpg",
			Type:    "image/jpeg",
			Size:    5000,
			Expires: expires1,
			URL:     "https://ntfy.sh/file/AbDeFgJhal.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		expires2 := time.Now().Add(2 * time.Hour).Unix() // Future
		m = model.NewDefaultMessage("mytopic", "sending you a car")
		m.ID = "m2"
		m.SequenceID = "m2"
		m.Sender = netip.MustParseAddr("1.2.3.4")
		m.Attachment = &model.Attachment{
			Name:    "car.jpg",
			Type:    "image/jpeg",
			Size:    10000,
			Expires: expires2,
			URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		expires3 := time.Now().Add(1 * time.Hour).Unix() // Future
		m = model.NewDefaultMessage("another-topic", "sending you another car")
		m.ID = "m3"
		m.SequenceID = "m3"
		m.User = "u_BAsbaAa"
		m.Sender = netip.MustParseAddr("5.6.7.8")
		m.Attachment = &model.Attachment{
			Name:    "another-car.jpg",
			Type:    "image/jpeg",
			Size:    20000,
			Expires: expires3,
			URL:     "https://ntfy.sh/file/zakaDHFW.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))

		require.Equal(t, "flower for you", messages[0].Message)
		require.Equal(t, "flower.jpg", messages[0].Attachment.Name)
		require.Equal(t, "image/jpeg", messages[0].Attachment.Type)
		require.Equal(t, int64(5000), messages[0].Attachment.Size)
		require.Equal(t, expires1, messages[0].Attachment.Expires)
		require.Equal(t, "https://ntfy.sh/file/AbDeFgJhal.jpg", messages[0].Attachment.URL)
		require.Equal(t, "1.2.3.4", messages[0].Sender.String())

		require.Equal(t, "sending you a car", messages[1].Message)
		require.Equal(t, "car.jpg", messages[1].Attachment.Name)
		require.Equal(t, "image/jpeg", messages[1].Attachment.Type)
		require.Equal(t, int64(10000), messages[1].Attachment.Size)
		require.Equal(t, expires2, messages[1].Attachment.Expires)
		require.Equal(t, "https://ntfy.sh/file/aCaRURL.jpg", messages[1].Attachment.URL)
		require.Equal(t, "1.2.3.4", messages[1].Sender.String())

		size, err := s.AttachmentBytesUsedBySender("1.2.3.4")
		require.Nil(t, err)
		require.Equal(t, int64(10000), size)

		size, err = s.AttachmentBytesUsedBySender("5.6.7.8")
		require.Nil(t, err)
		require.Equal(t, int64(0), size) // Accounted to the user, not the IP!

		size, err = s.AttachmentBytesUsedByUser("u_BAsbaAa")
		require.Nil(t, err)
		require.Equal(t, int64(20000), size)
	})
}

func TestStore_AttachmentsExpired(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m := model.NewDefaultMessage("mytopic", "flower for you")
		m.ID = "m1"
		m.SequenceID = "m1"
		m.Expires = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(m))

		m = model.NewDefaultMessage("mytopic", "message with attachment")
		m.ID = "m2"
		m.SequenceID = "m2"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &model.Attachment{
			Name:    "car.jpg",
			Type:    "image/jpeg",
			Size:    10000,
			Expires: time.Now().Add(2 * time.Hour).Unix(),
			URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		m = model.NewDefaultMessage("mytopic", "message with external attachment")
		m.ID = "m3"
		m.SequenceID = "m3"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &model.Attachment{
			Name:    "car.jpg",
			Type:    "image/jpeg",
			Expires: 0, // Unknown!
			URL:     "https://somedomain.com/car.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		m = model.NewDefaultMessage("mytopic2", "message with expired attachment")
		m.ID = "m4"
		m.SequenceID = "m4"
		m.Expires = time.Now().Add(2 * time.Hour).Unix()
		m.Attachment = &model.Attachment{
			Name:    "expired-car.jpg",
			Type:    "image/jpeg",
			Size:    20000,
			Expires: time.Now().Add(-1 * time.Hour).Unix(),
			URL:     "https://ntfy.sh/file/aCaRURL.jpg",
		}
		require.Nil(t, s.AddMessage(m))

		count, err := s.MarkExpiredAttachmentsDeleted(10)
		require.Nil(t, err)
		require.Equal(t, int64(1), count)
	})
}

func TestStore_Sender(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		m1 := model.NewDefaultMessage("mytopic", "mymessage")
		m1.Sender = netip.MustParseAddr("1.2.3.4")
		require.Nil(t, s.AddMessage(m1))

		m2 := model.NewDefaultMessage("mytopic", "mymessage without sender")
		require.Nil(t, s.AddMessage(m2))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))
		require.Equal(t, messages[0].Sender, netip.MustParseAddr("1.2.3.4"))
		require.Equal(t, messages[1].Sender, netip.Addr{})
	})
}

func TestStore_DeleteScheduledBySequenceID(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Create a scheduled (unpublished) message
		scheduledMsg := model.NewDefaultMessage("mytopic", "scheduled message")
		scheduledMsg.ID = "scheduled1"
		scheduledMsg.SequenceID = "seq123"
		scheduledMsg.Time = time.Now().Add(time.Hour).Unix() // Future time makes it scheduled
		require.Nil(t, s.AddMessage(scheduledMsg))

		// Create a published message with different sequence ID
		publishedMsg := model.NewDefaultMessage("mytopic", "published message")
		publishedMsg.ID = "published1"
		publishedMsg.SequenceID = "seq456"
		publishedMsg.Time = time.Now().Add(-time.Hour).Unix() // Past time makes it published
		require.Nil(t, s.AddMessage(publishedMsg))

		// Create a scheduled message in a different topic
		otherTopicMsg := model.NewDefaultMessage("othertopic", "other scheduled")
		otherTopicMsg.ID = "other1"
		otherTopicMsg.SequenceID = "seq123" // Same sequence ID as scheduledMsg
		otherTopicMsg.Time = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(otherTopicMsg))

		// Verify all messages exist (including scheduled)
		messages, err := s.Messages("mytopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))

		messages, err = s.Messages("othertopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))

		// Delete scheduled message by sequence ID and verify returned IDs
		deletedIDs, err := s.DeleteScheduledBySequenceID("mytopic", "seq123")
		require.Nil(t, err)
		require.Equal(t, 1, len(deletedIDs))
		require.Equal(t, "scheduled1", deletedIDs[0])

		// Verify scheduled message is deleted
		messages, err = s.Messages("mytopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "published message", messages[0].Message)

		// Verify other topic's message still exists (topic-scoped deletion)
		messages, err = s.Messages("othertopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "other scheduled", messages[0].Message)

		// Deleting non-existent sequence ID should return empty list
		deletedIDs, err = s.DeleteScheduledBySequenceID("mytopic", "nonexistent")
		require.Nil(t, err)
		require.Empty(t, deletedIDs)

		// Deleting published message should not affect it (only deletes unpublished)
		deletedIDs, err = s.DeleteScheduledBySequenceID("mytopic", "seq456")
		require.Nil(t, err)
		require.Empty(t, deletedIDs)

		messages, err = s.Messages("mytopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "published message", messages[0].Message)
	})
}

func TestStore_MessageByID(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Add a message
		m := model.NewDefaultMessage("mytopic", "some message")
		m.Title = "some title"
		m.Priority = 4
		m.Tags = []string{"tag1", "tag2"}
		require.Nil(t, s.AddMessage(m))

		// Retrieve by ID
		retrieved, err := s.Message(m.ID)
		require.Nil(t, err)
		require.Equal(t, m.ID, retrieved.ID)
		require.Equal(t, "mytopic", retrieved.Topic)
		require.Equal(t, "some message", retrieved.Message)
		require.Equal(t, "some title", retrieved.Title)
		require.Equal(t, 4, retrieved.Priority)
		require.Equal(t, []string{"tag1", "tag2"}, retrieved.Tags)

		// Non-existent ID returns ErrMessageNotFound
		_, err = s.Message("doesnotexist")
		require.Equal(t, model.ErrMessageNotFound, err)
	})
}

func TestStore_MarkPublished(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Add a scheduled message (future time -> unpublished)
		m := model.NewDefaultMessage("mytopic", "scheduled message")
		m.Time = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(m))

		// Verify it does not appear in non-scheduled queries
		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 0, len(messages))

		// Verify it does appear in scheduled queries
		messages, err = s.Messages("mytopic", model.SinceAllMessages, true)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))

		// Mark as published
		require.Nil(t, s.MarkPublished(m))

		// Now it should appear in non-scheduled queries too
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "scheduled message", messages[0].Message)
	})
}

func TestStore_MessagesDue_ClaimExactlyOnce(t *testing.T) {
	// Postgres-only: exercises "FOR UPDATE SKIP LOCKED" claiming so that concurrent delayed
	// senders running on different cluster nodes never pick up (and deliver) the same due
	// message twice. With the non-claiming implementation, every concurrent caller sees every
	// due row, so this test fails; with claiming, each row is returned to exactly one caller.
	s := newTestPostgresStore(t) // skips if NTFY_TEST_DATABASE_URL is unset
	const n = 40
	for i := 0; i < n; i++ {
		m := model.NewDefaultMessage("mytopic", fmt.Sprintf("scheduled %d", i))
		m.Time = time.Now().Add(time.Hour).Unix() // future -> stored as published=FALSE
		require.Nil(t, s.AddMessage(m))
		// Move the time into the past so the message is due now (but still unpublished)
		require.Nil(t, s.UpdateMessageTime(m.ID, time.Now().Add(-time.Minute).Unix()))
	}
	var mu sync.Mutex
	seen := make(map[string]int)
	var wg sync.WaitGroup
	for c := 0; c < 6; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			due, err := s.MessagesDue()
			require.Nil(t, err)
			mu.Lock()
			defer mu.Unlock()
			for _, m := range due {
				seen[m.ID]++
			}
		}()
	}
	wg.Wait()
	require.Len(t, seen, n) // every due message was claimed
	for id, count := range seen {
		require.Equalf(t, 1, count, "message %s was claimed %d times, want exactly 1", id, count)
	}
}

func TestStore_MessagesDue_AbandonedClaimIsReclaimed(t *testing.T) {
	// A node that dies between claiming and delivering leaves its rows claimed but unpublished.
	// Nothing would ever deliver them again if the claim were permanent, so it expires: until it
	// does, no other node sees the message; after it does, any node may claim it afresh.
	s := newTestPostgresStore(t) // skips if NTFY_TEST_DATABASE_URL is unset
	m := model.NewDefaultMessage("mytopic", "scheduled")
	m.Time = time.Now().Add(time.Hour).Unix() // future -> stored unpublished
	require.Nil(t, s.AddMessage(m))
	require.Nil(t, s.UpdateMessageTime(m.ID, time.Now().Add(-time.Minute).Unix()))

	// The claim takes it, and nothing else can have it while the claim holds
	due, err := s.MessagesDue()
	require.Nil(t, err)
	require.Len(t, due, 1)
	require.Equal(t, m.ID, due[0].ID)
	due, err = s.MessagesDue()
	require.Nil(t, err)
	require.Empty(t, due, "a claimed message was handed out again while the claim still held")

	// The claimer never published it (it died); past the timeout the message is claimable again
	s.SetClaimTimeoutForTest(0)
	due, err = s.MessagesDue()
	require.Nil(t, err)
	require.Len(t, due, 1, "an abandoned claim was never reclaimed")
	require.Equal(t, m.ID, due[0].ID)

	// Once it is actually delivered, no timeout brings it back
	require.Nil(t, s.MarkPublished(due[0]))
	due, err = s.MessagesDue()
	require.Nil(t, err)
	require.Empty(t, due, "a published message was handed out again")
}

func TestStore_ExpireMessages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Add messages to two topics
		m1 := model.NewDefaultMessage("topic1", "message 1")
		m1.Expires = time.Now().Add(time.Hour).Unix()
		m2 := model.NewDefaultMessage("topic1", "message 2")
		m2.Expires = time.Now().Add(time.Hour).Unix()
		m3 := model.NewDefaultMessage("topic2", "message 3")
		m3.Expires = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(m1))
		require.Nil(t, s.AddMessage(m2))
		require.Nil(t, s.AddMessage(m3))

		// Verify all messages exist
		messages, err := s.Messages("topic1", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))
		messages, err = s.Messages("topic2", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))

		// Expire topic1 messages
		require.Nil(t, s.ExpireMessages("topic1"))

		// topic1 messages should now be expired (expires set to past)
		deleted, err := s.DeleteExpiredMessages(100)
		require.Nil(t, err)
		require.Equal(t, int64(2), deleted)

		// topic2 should be unaffected
		messages, err = s.Messages("topic2", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "message 3", messages[0].Message)
	})
}

func TestStore_MarkAttachmentsDeleted(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Add a message with an expired attachment (file needs cleanup)
		m1 := model.NewDefaultMessage("mytopic", "old file")
		m1.ID = "msg1"
		m1.SequenceID = "msg1"
		m1.Expires = time.Now().Add(time.Hour).Unix()
		m1.Attachment = &model.Attachment{
			Name:    "old.pdf",
			Type:    "application/pdf",
			Size:    50000,
			Expires: time.Now().Add(-time.Hour).Unix(), // Expired
			URL:     "https://ntfy.sh/file/old.pdf",
		}
		require.Nil(t, s.AddMessage(m1))

		// Add a message with another expired attachment
		m2 := model.NewDefaultMessage("mytopic", "another old file")
		m2.ID = "msg2"
		m2.SequenceID = "msg2"
		m2.Expires = time.Now().Add(time.Hour).Unix()
		m2.Attachment = &model.Attachment{
			Name:    "another.pdf",
			Type:    "application/pdf",
			Size:    30000,
			Expires: time.Now().Add(-time.Hour).Unix(), // Expired
			URL:     "https://ntfy.sh/file/another.pdf",
		}
		require.Nil(t, s.AddMessage(m2))

		// Both should be marked as deleted in one batch
		count, err := s.MarkExpiredAttachmentsDeleted(10)
		require.Nil(t, err)
		require.Equal(t, int64(2), count)

		// No more expired attachments to clean up
		count, err = s.MarkExpiredAttachmentsDeleted(10)
		require.Nil(t, err)
		require.Equal(t, int64(0), count)

		// Messages themselves still exist
		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))
	})
}

func TestStore_Stats(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Initial stats should be zero
		messages, err := s.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(0), messages)

		// Update stats
		require.Nil(t, s.UpdateStats(42))
		messages, err = s.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(42), messages)

		// Update again (overwrites)
		require.Nil(t, s.UpdateStats(100))
		messages, err = s.Stats()
		require.Nil(t, err)
		require.Equal(t, int64(100), messages)
	})
}

func TestStore_AddMessages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Batch add multiple messages
		msgs := []*model.Message{
			model.NewDefaultMessage("mytopic", "batch 1"),
			model.NewDefaultMessage("mytopic", "batch 2"),
			model.NewDefaultMessage("othertopic", "batch 3"),
		}
		require.Nil(t, s.AddMessages(msgs))

		// Verify all were inserted
		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 2, len(messages))

		messages, err = s.Messages("othertopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "batch 3", messages[0].Message)

		// Empty batch should succeed
		require.Nil(t, s.AddMessages([]*model.Message{}))

		// Batch with invalid event type should fail
		badMsgs := []*model.Message{
			model.NewKeepaliveMessage("mytopic"),
		}
		require.NotNil(t, s.AddMessages(badMsgs))
	})
}

func TestStore_AddMessages_AllFields(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Every column must land in the right row when several messages are written in one batch
		msgs := make([]*model.Message, 0)
		for i := 0; i < 3; i++ {
			m := model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i))
			m.Time = int64(1000 + i)
			m.SequenceID = fmt.Sprintf("seq%d", i)
			m.Title = fmt.Sprintf("title %d", i)
			m.Priority = i + 1
			m.Tags = []string{fmt.Sprintf("tag%d", i), "common"}
			m.Click = fmt.Sprintf("https://example.com/%d", i)
			m.Icon = fmt.Sprintf("https://example.com/%d.png", i)
			m.Actions = []*model.Action{{ID: fmt.Sprintf("a%d", i), Action: "view", Label: fmt.Sprintf("label %d", i), URL: "https://example.com"}}
			m.Attachment = &model.Attachment{Name: fmt.Sprintf("file%d.txt", i), Type: "text/plain", Size: int64(10 + i), Expires: int64(2000 + i), URL: fmt.Sprintf("https://example.com/file%d.txt", i)}
			m.Sender = netip.MustParseAddr(fmt.Sprintf("1.2.3.%d", i+1))
			m.User = fmt.Sprintf("u_%d", i)
			m.ContentType = "text/markdown"
			m.Encoding = "base64"
			msgs = append(msgs, m)
		}
		require.Nil(t, s.AddMessages(msgs))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 3, len(messages))
		for i, m := range messages {
			require.Equal(t, msgs[i].ID, m.ID)
			require.Equal(t, fmt.Sprintf("seq%d", i), m.SequenceID)
			require.Equal(t, int64(1000+i), m.Time)
			require.Equal(t, fmt.Sprintf("message %d", i), m.Message)
			require.Equal(t, fmt.Sprintf("title %d", i), m.Title)
			require.Equal(t, i+1, m.Priority)
			require.Equal(t, []string{fmt.Sprintf("tag%d", i), "common"}, m.Tags)
			require.Equal(t, fmt.Sprintf("https://example.com/%d", i), m.Click)
			require.Equal(t, fmt.Sprintf("https://example.com/%d.png", i), m.Icon)
			require.Equal(t, 1, len(m.Actions))
			require.Equal(t, fmt.Sprintf("label %d", i), m.Actions[0].Label)
			require.Equal(t, fmt.Sprintf("file%d.txt", i), m.Attachment.Name)
			require.Equal(t, int64(10+i), m.Attachment.Size)
			require.Equal(t, int64(2000+i), m.Attachment.Expires)
			require.Equal(t, fmt.Sprintf("https://example.com/file%d.txt", i), m.Attachment.URL)
			require.Equal(t, fmt.Sprintf("1.2.3.%d", i+1), m.Sender.String())
			require.Equal(t, fmt.Sprintf("u_%d", i), m.User)
			require.Equal(t, "text/markdown", m.ContentType)
			require.Equal(t, "base64", m.Encoding)
		}
	})
}

func TestPostgresStore_AddMessages_OneRoundTripPerBatch(t *testing.T) {
	// The batch writer is the only writer, so its round trips cap the publish rate on cron bursts.
	// With one INSERT per row, a batch of 100 against a remote database costs 100+ round trips.
	testDB, proxy := dbtest.CreateTestPostgresWithLatency(t, time.Millisecond)
	s, err := message.NewPostgresStore(testDB, 0, 0)
	require.Nil(t, err)
	msgs := make([]*model.Message, 0)
	for i := 0; i < 100; i++ {
		msgs = append(msgs, model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i)))
	}
	require.Nil(t, s.AddMessages(msgs[:1])) // Warm up the pool connection
	before := proxy.Requests()
	require.Nil(t, s.AddMessages(msgs[1:]))
	roundTrips := proxy.Requests() - before
	require.Less(t, roundTrips, int64(10), "writing 99 messages took %d round trips", roundTrips)

	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 100, len(messages))
}

func TestPostgresStore_AddMessages_OnePreparedStatementForAnyBatchSize(t *testing.T) {
	// pgx prepares every distinct statement text server-side and keeps up to 512 per connection.
	// A multi-row INSERT whose text depends on the batch size leaves hundreds of multi-MB plans
	// in every pooled backend (ntfy.sh primary memory crept from 50% to 65% in 3h after 2.29.0).
	testDB := dbtest.CreateTestPostgres(t)
	testDB.Primary().SetMaxOpenConns(1) // Every statement lands on the same backend
	s, err := message.NewPostgresStore(testDB, 0, 0)
	require.Nil(t, err)
	for _, size := range []int{1, 2, 50, 300} {
		msgs := make([]*model.Message, 0)
		for i := 0; i < size; i++ {
			msgs = append(msgs, model.NewDefaultMessage("mytopic", fmt.Sprintf("message %d", i)))
		}
		require.Nil(t, s.AddMessages(msgs))
	}
	var prepared int
	require.Nil(t, testDB.Primary().QueryRow("SELECT COUNT(*) FROM pg_prepared_statements WHERE statement ~ '^\\s*INSERT INTO message '").Scan(&prepared)) // Anchored, so this SELECT does not count itself
	require.Equal(t, 1, prepared, "one INSERT shape regardless of batch size")

	messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 353, len(messages))
}

func TestStore_MessagesDue(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Add a message scheduled in the past (i.e. it's due now)
		m1 := model.NewDefaultMessage("mytopic", "due message")
		m1.Time = time.Now().Add(-time.Second).Unix()
		// Set expires in the future so it doesn't get pruned
		m1.Expires = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(m1))

		// Add a message scheduled in the future (not due)
		m2 := model.NewDefaultMessage("mytopic", "future message")
		m2.Time = time.Now().Add(time.Hour).Unix()
		require.Nil(t, s.AddMessage(m2))

		// Mark m1 as published so it won't be "due"
		// (MessagesDue returns unpublished messages whose time <= now)
		// m1 is auto-published (time <= now), so it should not be due
		// m2 is unpublished (time in future), not due yet
		due, err := s.MessagesDue()
		require.Nil(t, err)
		require.Equal(t, 0, len(due))

		// Add a message that was explicitly scheduled in the past but time has "arrived"
		// We need to manipulate the database to create a truly "due" message:
		// a message with published=false and time <= now
		m3 := model.NewDefaultMessage("mytopic", "truly due message")
		m3.Time = time.Now().Add(2 * time.Second).Unix() // 2 seconds from now
		require.Nil(t, s.AddMessage(m3))

		// Not due yet
		due, err = s.MessagesDue()
		require.Nil(t, err)
		require.Equal(t, 0, len(due))

		// Wait for it to become due
		time.Sleep(3 * time.Second)

		due, err = s.MessagesDue()
		require.Nil(t, err)
		require.Equal(t, 1, len(due))
		require.Equal(t, "truly due message", due[0].Message)
	})
}

func TestStore_MessageFieldRoundTrip(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Create a message with all fields populated
		m := model.NewDefaultMessage("mytopic", "hello world")
		m.SequenceID = "custom_seq_id"
		m.Title = "A Title"
		m.Priority = 4
		m.Tags = []string{"warning", "srv01"}
		m.Click = "https://example.com/click"
		m.Icon = "https://example.com/icon.png"
		m.Actions = []*model.Action{
			{
				ID:     "action1",
				Action: "view",
				Label:  "Open Site",
				URL:    "https://example.com",
				Clear:  true,
			},
			{
				ID:      "action2",
				Action:  "http",
				Label:   "Call Webhook",
				URL:     "https://example.com/hook",
				Method:  "PUT",
				Headers: map[string]string{"X-Token": "secret"},
				Body:    `{"key":"value"}`,
			},
		}
		m.ContentType = "text/markdown"
		m.Encoding = "base64"
		m.Sender = netip.MustParseAddr("9.8.7.6")
		m.User = "u_TestUser123"
		require.Nil(t, s.AddMessage(m))

		// Retrieve and verify every field
		retrieved, err := s.Message(m.ID)
		require.Nil(t, err)

		require.Equal(t, m.ID, retrieved.ID)
		require.Equal(t, "custom_seq_id", retrieved.SequenceID)
		require.Equal(t, m.Time, retrieved.Time)
		require.Equal(t, m.Expires, retrieved.Expires)
		require.Equal(t, model.MessageEvent, retrieved.Event)
		require.Equal(t, "mytopic", retrieved.Topic)
		require.Equal(t, "hello world", retrieved.Message)
		require.Equal(t, "A Title", retrieved.Title)
		require.Equal(t, 4, retrieved.Priority)
		require.Equal(t, []string{"warning", "srv01"}, retrieved.Tags)
		require.Equal(t, "https://example.com/click", retrieved.Click)
		require.Equal(t, "https://example.com/icon.png", retrieved.Icon)
		require.Equal(t, "text/markdown", retrieved.ContentType)
		require.Equal(t, "base64", retrieved.Encoding)
		require.Equal(t, netip.MustParseAddr("9.8.7.6"), retrieved.Sender)
		require.Equal(t, "u_TestUser123", retrieved.User)

		// Verify actions round-trip
		require.Equal(t, 2, len(retrieved.Actions))

		require.Equal(t, "action1", retrieved.Actions[0].ID)
		require.Equal(t, "view", retrieved.Actions[0].Action)
		require.Equal(t, "Open Site", retrieved.Actions[0].Label)
		require.Equal(t, "https://example.com", retrieved.Actions[0].URL)
		require.Equal(t, true, retrieved.Actions[0].Clear)

		require.Equal(t, "action2", retrieved.Actions[1].ID)
		require.Equal(t, "http", retrieved.Actions[1].Action)
		require.Equal(t, "Call Webhook", retrieved.Actions[1].Label)
		require.Equal(t, "https://example.com/hook", retrieved.Actions[1].URL)
		require.Equal(t, "PUT", retrieved.Actions[1].Method)
		require.Equal(t, "secret", retrieved.Actions[1].Headers["X-Token"])
		require.Equal(t, `{"key":"value"}`, retrieved.Actions[1].Body)
	})
}

func TestStore_AddMessage_InvalidUTF8(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// 0xc9 0x43: Latin-1 "ÉC" — 0xc9 starts a 2-byte UTF-8 sequence but 0x43 ('C') is not a continuation byte
		m := model.NewDefaultMessage("mytopic", "\xc9Cas du serveur")
		require.Nil(t, s.AddMessage(m))
		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "\uFFFDCas du serveur", messages[0].Message)

		// 0xae: Latin-1 "®" — isolated byte above 0x7F, not a valid UTF-8 start for single byte
		m2 := model.NewDefaultMessage("mytopic", "Product\xae Pro")
		require.Nil(t, s.AddMessage(m2))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "Product\uFFFD Pro", messages[1].Message)

		// 0xe8 0x6d 0x65: Latin-1 "ème" — 0xe8 starts a 3-byte UTF-8 sequence but 0x6d ('m') is not a continuation byte
		m3 := model.NewDefaultMessage("mytopic", "probl\xe8me critique")
		require.Nil(t, s.AddMessage(m3))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "probl\uFFFDme critique", messages[2].Message)

		// 0xb2: Latin-1 "²" — isolated byte in 0x80-0xBF range (UTF-8 continuation byte without lead)
		m4 := model.NewDefaultMessage("mytopic", "CO\xb2 level high")
		require.Nil(t, s.AddMessage(m4))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "CO\uFFFD level high", messages[3].Message)

		// 0xe9 0x6d 0x61: Latin-1 "éma" — 0xe9 starts a 3-byte UTF-8 sequence but 0x6d ('m') is not a continuation byte
		m5 := model.NewDefaultMessage("mytopic", "th\xe9matique")
		require.Nil(t, s.AddMessage(m5))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "th\uFFFDmatique", messages[4].Message)

		// 0xed 0x64 0x65: Latin-1 "íde" — 0xed starts a 3-byte UTF-8 sequence but 0x64 ('d') is not a continuation byte
		m6 := model.NewDefaultMessage("mytopic", "vid\xed\x64eo surveillance")
		require.Nil(t, s.AddMessage(m6))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "vid\uFFFDdeo surveillance", messages[5].Message)

		// 0xf3 0x6e 0x3a 0x20: Latin-1 "ón: " — 0xf3 starts a 4-byte UTF-8 sequence but 0x6e ('n') is not a continuation byte
		m7 := model.NewDefaultMessage("mytopic", "notificaci\xf3n: alerta")
		require.Nil(t, s.AddMessage(m7))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "notificaci\uFFFDn: alerta", messages[6].Message)

		// 0xb7: Latin-1 "·" — isolated continuation byte
		m8 := model.NewDefaultMessage("mytopic", "item\xb7value")
		require.Nil(t, s.AddMessage(m8))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "item\uFFFDvalue", messages[7].Message)

		// 0xa8: Latin-1 "¨" — isolated continuation byte
		m9 := model.NewDefaultMessage("mytopic", "na\xa8ve")
		require.Nil(t, s.AddMessage(m9))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "na\uFFFDve", messages[8].Message)

		// 0xdf 0x64: Latin-1 "ßd" — 0xdf starts a 2-byte UTF-8 sequence but 0x64 ('d') is not a continuation byte
		m10 := model.NewDefaultMessage("mytopic", "gro\xdf\x64ruck")
		require.Nil(t, s.AddMessage(m10))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "gro\uFFFDdruck", messages[9].Message)

		// 0xe4 0x67 0x74: Latin-1 "ägt" — 0xe4 starts a 3-byte UTF-8 sequence but 0x67 ('g') is not a continuation byte
		m11 := model.NewDefaultMessage("mytopic", "tr\xe4gt Last")
		require.Nil(t, s.AddMessage(m11))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "tr\uFFFDgt Last", messages[10].Message)

		// 0xe9 0x65 0x20: Latin-1 "ée " — 0xe9 starts a 3-byte UTF-8 sequence but 0x65 ('e') is not a continuation byte
		m12 := model.NewDefaultMessage("mytopic", "journ\xe9\x65 termin\xe9\x65")
		require.Nil(t, s.AddMessage(m12))
		messages, err = s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, "journ\uFFFDe termin\uFFFDe", messages[11].Message)
	})
}

func TestStore_AddMessage_NullByte(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// 0x00: NUL byte — valid UTF-8 but rejected by PostgreSQL
		m := model.NewDefaultMessage("mytopic", "hello\x00world")
		require.Nil(t, s.AddMessage(m))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "helloworld", messages[0].Message)
	})
}

func TestStore_AddMessage_InvalidUTF8InTitleAndTags(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Invalid UTF-8 can arrive via HTTP headers (Title, Tags) which bypass body validation
		m := model.NewDefaultMessage("mytopic", "valid message")
		m.Title = "\xc9clipse du syst\xe8me"
		m.Tags = []string{"probl\xe8me", "syst\xe9me"}
		m.Click = "https://example.com/\xae"
		require.Nil(t, s.AddMessage(m))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 1, len(messages))
		require.Equal(t, "\uFFFDclipse du syst\uFFFDme", messages[0].Title)
		require.Equal(t, "probl\uFFFDme", messages[0].Tags[0])
		require.Equal(t, "syst\uFFFDme", messages[0].Tags[1])
		require.Equal(t, "https://example.com/\uFFFD", messages[0].Click)
	})
}

func TestStore_AddMessage_InvalidUTF8BatchDoesNotDropValidMessages(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		// Previously, a single invalid message would roll back the entire batch transaction.
		// Sanitization ensures all messages in a batch are written successfully.
		msgs := []*model.Message{
			model.NewDefaultMessage("mytopic", "valid message 1"),
			model.NewDefaultMessage("mytopic", "notificaci\xf3n: alerta"),
			model.NewDefaultMessage("mytopic", "valid message 3"),
		}
		require.Nil(t, s.AddMessages(msgs))

		messages, err := s.Messages("mytopic", model.SinceAllMessages, false)
		require.Nil(t, err)
		require.Equal(t, 3, len(messages))
	})
}

func TestStore_CloseGivesUpOnAStuckBatchWrite(t *testing.T) {
	// A wedged database must not make Close, and with it the entire shutdown, block forever.
	// systemd would SIGKILL us eventually, but only after its stop timeout.
	testDB := dbtest.CreateTestPostgres(t)
	s, err := message.NewPostgresStore(testDB, 100, time.Hour)
	require.Nil(t, err)

	// Hold a lock on the message table, so the final batch write cannot make progress
	tx, err := testDB.Begin()
	require.Nil(t, err)
	defer tx.Rollback()
	_, err = tx.Exec("LOCK TABLE message IN EXCLUSIVE MODE")
	require.Nil(t, err)

	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "my message")))
	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return while the database was stuck")
	}
}

func TestStore_UnwrittenUntilTheBatchIsWritten(t *testing.T) {
	// A slow database keeps a queued message out of the table for as long as it likes; the
	// server keeps such a message in its topic's window until it is in (see server.topic)
	testDB := dbtest.CreateTestPostgres(t)
	s, err := message.NewPostgresStore(testDB, 100, 10*time.Millisecond)
	require.Nil(t, err)
	t.Cleanup(func() { s.Close() })

	tx, err := testDB.Begin()
	require.Nil(t, err)
	defer tx.Rollback()
	_, err = tx.Exec("LOCK TABLE message IN EXCLUSIVE MODE")
	require.Nil(t, err)

	m := model.NewDefaultMessage("mytopic", "stalled")
	require.Nil(t, s.AddMessage(m))
	time.Sleep(200 * time.Millisecond) // Many batch timeouts: the batch is with the writer, stuck on the lock
	require.True(t, s.Unwritten(m.ID))

	require.Nil(t, tx.Rollback())
	require.Eventually(t, func() bool { return !s.Unwritten(m.ID) }, 5*time.Second, 10*time.Millisecond)
	_, err = s.Message(m.ID)
	require.Nil(t, err)
}

func TestStore_CloseGivesUpOnAnInFlightBatchWrite(t *testing.T) {
	// Same as TestStore_CloseGivesUpOnAStuckBatchWrite, but with a batch already handed to the
	// batch writer, so Close cannot even hand over the messages that are still pending
	testDB := dbtest.CreateTestPostgres(t)
	s, err := message.NewPostgresStore(testDB, 2, time.Hour)
	require.Nil(t, err)

	tx, err := testDB.Begin()
	require.Nil(t, err)
	defer tx.Rollback()
	_, err = tx.Exec("LOCK TABLE message IN EXCLUSIVE MODE")
	require.Nil(t, err)

	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "stuck 1")))
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "stuck 2"))) // Batch of 2, handed over
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("mytopic", "pending")))
	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return while the database was stuck")
	}
}

func TestStore_MessagesSinceID_ForeignMidActsAsPositionalMarker(t *testing.T) {
	// Multi-topic subscriptions apply ONE since=<mid> marker to every subscribed topic, so for
	// all but the marker's own topic the mid is foreign; it must act as a positional cut-off
	// (everything the marker's row precedes), not as "unknown -> full replay"
	s := newTestPostgresStore(t)
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("topicb", "b old")))
	marker := model.NewDefaultMessage("topica", "marker a")
	require.Nil(t, s.AddMessage(marker))
	require.Nil(t, s.AddMessage(model.NewDefaultMessage("topicb", "b new")))
	messages, err := s.Messages("topicb", model.NewSinceID(marker.ID), false)
	require.Nil(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "b new", messages[0].Message)
}

func TestStore_MessagesSinceID_ReplicaLagFallsBackToPrimary(t *testing.T) {
	// since=<mid> resolution must fall back to the primary when the replica does not know the
	// mid yet (publish on node A, client reconnects to node B before replication catches up).
	// Without the fallback, the unknown mid resolves to "id > 0" and the client is flooded
	// with the topic's whole retained history.
	primaryDSN, replicaDSN := dbtest.CreateTestPostgresSchema(t), dbtest.CreateTestPostgresSchema(t)
	openStoreOn := func(dsn string, replicas []*db.Host) *message.Cache {
		host, err := pg.Open(dsn)
		require.Nil(t, err)
		d := db.New(host, replicas)
		d.MarkReplicasHealthyForTest() // Route ReadOnly() to the lagged replica deterministically
		store, err := message.NewPostgresStore(d, 0, 0)
		require.Nil(t, err)
		t.Cleanup(func() { store.Close() })
		return store
	}
	// The "replica" is a separate schema holding only the first 3 messages (fully lagged after
	// that); row ids align because both schemas insert in the same order
	replicaOnly := openStoreOn(replicaDSN, nil)
	primaryOnly := openStoreOn(primaryDSN, nil)
	for i := 1; i <= 3; i++ {
		m := model.NewDefaultMessage("mytopic", fmt.Sprintf("m%d", i))
		require.Nil(t, primaryOnly.AddMessage(m))
		require.Nil(t, replicaOnly.AddMessage(m))
	}
	m4 := model.NewDefaultMessage("mytopic", "m4")
	require.Nil(t, primaryOnly.AddMessage(m4)) // Not yet replicated

	replicaHost, err := pg.OpenReplica(replicaDSN)
	require.Nil(t, err)
	combined := openStoreOn(primaryDSN, []*db.Host{replicaHost})
	messages, err := combined.Messages("mytopic", model.NewSinceID(m4.ID), false)
	require.Nil(t, err)
	require.Len(t, messages, 0) // Correct cut-off via primary; the lagged replica has nothing newer
}
