package message_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/model"
)

func TestStore_MessagesSinceID_SameSecondSequence(t *testing.T) {
	for _, event := range []string{model.MessageEvent, model.MessageDeleteEvent, model.MessageClearEvent} {
		for _, scheduled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/scheduled=%t", event, scheduled), func(t *testing.T) {
				forEachBackend(t, func(t *testing.T, s *message.Cache) {
					previous := model.NewMessage(event, "mytopic", "previous")
					previous.SequenceID, previous.Time = "sequence", 100
					other := model.NewDefaultMessage("mytopic", "other sequence")
					other.SequenceID, other.Time = "other", 100
					marker := model.NewDefaultMessage("mytopic", "current version")
					marker.SequenceID, marker.Time = "sequence", 100
					require.NoError(t, s.AddMessages([]*model.Message{previous, other, marker}))

					// Replaying the preceding delete or update without the marker makes a client
					// delete or replace the current version (#2189). Other sequences still replay.
					replayed := messagesAfter(t, s, "mytopic", marker.ID, scheduled)
					require.Equal(t, []string{"other sequence"}, bodies(replayed))

					// A genuine update, delete or clear after the marker must still be delivered,
					// even when it has exactly the same timestamp.
					next := model.NewMessage(event, "mytopic", "next")
					next.SequenceID, next.Time = "sequence", 100
					require.NoError(t, s.AddMessage(next))
					replayed = messagesAfter(t, s, "mytopic", marker.ID, scheduled)
					require.Equal(t, []string{"other sequence", "next"}, bodies(replayed))
				})
			})
		}
	}
}

func TestStore_MessagesSinceID_SequenceInAnotherTopic(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		other := model.NewDefaultMessage("another-topic", "another topic")
		other.SequenceID, other.Time = "sequence", 100
		marker := model.NewDefaultMessage("mytopic", "marker")
		marker.SequenceID, marker.Time = "sequence", 100
		require.NoError(t, s.AddMessages([]*model.Message{other, marker}))

		for _, scheduled := range []bool{false, true} {
			replayed := messagesAfter(t, s, "another-topic", marker.ID, scheduled)
			require.Equal(t, []string{"another topic"}, bodies(replayed))
		}
	})
}

func TestStore_MessagesSinceID_ImplicitSequence(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		marker := model.NewDefaultMessage("mytopic", "marker")
		marker.Time = 100 // Older cache entries may not have an explicit sequence ID.
		previous := model.NewActionMessage(model.MessageDeleteEvent, "mytopic", marker.ID)
		previous.Time = 100
		other := model.NewDefaultMessage("mytopic", "another implicit sequence")
		other.Time = 100
		require.NoError(t, s.AddMessages([]*model.Message{previous, other, marker}))

		for _, scheduled := range []bool{false, true} {
			replayed := messagesAfter(t, s, "mytopic", marker.ID, scheduled)
			require.Equal(t, []string{"another implicit sequence"}, bodies(replayed))
		}
	})
}

func TestStore_MessagesSinceID_SequenceWrittenOutOfOrder(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *message.Cache) {
		later := model.NewDefaultMessage("mytopic", "published later, written first")
		later.SequenceID, later.Time = "sequence", 101
		marker := model.NewDefaultMessage("mytopic", "marker")
		marker.SequenceID, marker.Time = "sequence", 100
		require.NoError(t, s.AddMessages([]*model.Message{later, marker}))
		skewed := model.NewDefaultMessage("mytopic", "written later with an earlier timestamp")
		skewed.SequenceID, skewed.Time = "sequence", 99
		require.NoError(t, s.AddMessage(skewed))

		for _, scheduled := range []bool{false, true} {
			replayed := messagesAfter(t, s, "mytopic", marker.ID, scheduled)
			require.Equal(t, []string{"written later with an earlier timestamp", "published later, written first"}, bodies(replayed))
		}
	})
}
