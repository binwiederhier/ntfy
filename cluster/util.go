package cluster

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/netip"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
)

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

// decodeMessageBody reads NDJSON apiMessage lines from r, reattaches the non-JSON fields
// (Sender, User) onto each message, and hands them to deliver. Malformed or message-less lines
// are skipped and logged, not fatal: fan-out is fire-and-forget, so the valid remainder of a
// request is still delivered. It returns an error only for stream-level failures (e.g. a line
// exceeding maxLineBytes).
// initialScanBuffer is how much the NDJSON decoder allocates up front per batch; lines over it
// grow the buffer up to the caller's limit.
const initialScanBuffer = 64 * 1024

func decodeMessageBody(r io.Reader, maxLineBytes int, deliver DeliverFunc) error {
	scanner := bufio.NewScanner(r)
	// Two bufio details decide what actually gets enforced here. Its token limit is the LARGER
	// of the max and the initial buffer, so a buffer bigger than the limit quietly raises it:
	// with a 64KB buffer, a 17KB limit (what ntfy's default message size implies) accepted lines
	// of up to 64KB. And the max is exclusive, so it takes one more byte to accept a line of
	// exactly maxLineBytes, which is what the name promises.
	maxToken := maxLineBytes + 1
	scanner.Buffer(make([]byte, min(maxToken, initialScanBuffer)), maxToken)
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
