package cluster

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/netip"
	"slices"
	"strings"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
)

// messageURL derives the peer's message endpoint URL from its advertise URL.
func messageURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + MessagePath
}

// stateURL derives the peer's state endpoint URL from its advertise URL.
func stateURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + StatePath
}

// marshalMessage serializes one message and its non-JSON fields (Sender, User) as an
// apiMessage line. Lines are marshaled once per publish and shared across all per-peer
// queues; assembleMessageBody joins them without re-marshaling.
func marshalMessage(m *model.Message) ([]byte, error) {
	apiMsg := &apiMessage{User: m.User, Message: m}
	if m.Sender.IsValid() {
		apiMsg.Sender = m.Sender.String()
	}
	return json.Marshal(apiMsg)
}

// assembleMessageBody builds an NDJSON fan-out request body from pre-marshaled apiMessage
// lines, avoiding a second JSON marshal of the messages.
func assembleMessageBody(frags []*fragment) []byte {
	lines := make([][]byte, len(frags))
	for i, f := range frags {
		lines[i] = f.data
	}
	return append(bytes.Join(lines, []byte("\n")), '\n')
}

// fragmentTopics returns the distinct topics of a batch, for reporting a delivery gap
func fragmentTopics(frags []*fragment) []string {
	topics := make([]string, 0, len(frags))
	for _, f := range frags {
		if !slices.Contains(topics, f.topic) {
			topics = append(topics, f.topic)
		}
	}
	return topics
}

// fragmentOldest returns the publish time of the oldest message in a batch, which dates the
// delivery gap a failed batch leaves behind (see GapFunc).
func fragmentOldest(frags []*fragment) int64 {
	var oldest int64
	for _, f := range frags {
		if oldest == 0 || f.time < oldest {
			oldest = f.time
		}
	}
	return oldest
}

// decodeMessageBody reads NDJSON apiMessage lines from r, reattaches the non-JSON fields
// (Sender, User) onto each message, and hands them to deliver. Malformed or message-less lines
// are skipped and logged, not fatal: fan-out is fire-and-forget, so the valid remainder of a
// request is still delivered. It returns an error only for stream-level failures (e.g. a line
// exceeding maxLineBytes).
func decodeMessageBody(r io.Reader, maxLineBytes int, deliver DeliverFunc) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var apiMsg apiMessage
		if err := json.Unmarshal(line, &apiMsg); err != nil || apiMsg.Message == nil {
			log.Tag(tag).Warn("Skipping malformed fan-out line")
			continue
		}
		apiMsg.Message.User = apiMsg.User
		if apiMsg.Sender != "" {
			if addr, err := netip.ParseAddr(apiMsg.Sender); err == nil {
				apiMsg.Message.Sender = addr
			}
		}
		deliver(apiMsg.Message)
	}
	return scanner.Err()
}

// subtractTopics returns the topics in a that are not in b
func subtractTopics(a, b []string) []string {
	out := make([]string, 0, len(a))
	for _, topic := range a {
		if !slices.Contains(b, topic) {
			out = append(out, topic)
		}
	}
	return out
}
