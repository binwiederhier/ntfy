package server

import (
	"encoding/json"
	"firebase.google.com/go/messaging"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestMaybeTruncateFCMMessage(t *testing.T) {
	origMessage := strings.Repeat("this is a long string", 300)
	origFCMMessage := &messaging.Message{
		Topic: "mytopic",
		Data: map[string]string{
			"id":       "abcdefg",
			"time":     "1641324761",
			"event":    "message",
			"topic":    "mytopic",
			"priority": "0",
			"tags":     "",
			"title":    "",
			"message":  origMessage,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
	}
	origMessageLength := len(origFCMMessage.Data["message"])
	serializedOrigFCMMessage, _ := json.Marshal(origFCMMessage)
	require.Greater(t, len(serializedOrigFCMMessage), fcmMessageLimit) // Pre-condition

	truncatedFCMMessage := maybeTruncateFCMMessage(origFCMMessage)
	truncatedMessageLength := len(truncatedFCMMessage.Data["message"])
	serializedTruncatedFCMMessage, _ := json.Marshal(truncatedFCMMessage)
	require.Equal(t, fcmMessageLimit, len(serializedTruncatedFCMMessage))
	require.Equal(t, "1", truncatedFCMMessage.Data["truncated"])
	require.NotEqual(t, origMessageLength, truncatedMessageLength)
}

func TestMaybeTruncateFCMMessage_NotTooLong(t *testing.T) {
	origMessage := "not really a long string"
	origFCMMessage := &messaging.Message{
		Topic: "mytopic",
		Data: map[string]string{
			"id":       "abcdefg",
			"time":     "1641324761",
			"event":    "message",
			"topic":    "mytopic",
			"priority": "0",
			"tags":     "",
			"title":    "",
			"message":  origMessage,
		},
	}
	origMessageLength := len(origFCMMessage.Data["message"])
	serializedOrigFCMMessage, _ := json.Marshal(origFCMMessage)
	require.LessOrEqual(t, len(serializedOrigFCMMessage), fcmMessageLimit) // Pre-condition

	notTruncatedFCMMessage := maybeTruncateFCMMessage(origFCMMessage)
	notTruncatedMessageLength := len(notTruncatedFCMMessage.Data["message"])
	serializedNotTruncatedFCMMessage, _ := json.Marshal(notTruncatedFCMMessage)
	require.Equal(t, origMessageLength, notTruncatedMessageLength)
	require.Equal(t, len(serializedOrigFCMMessage), len(serializedNotTruncatedFCMMessage))
	require.Equal(t, "", notTruncatedFCMMessage.Data["truncated"])
}
