package cluster

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/netip"
	"strings"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
)

// deliverURL derives the peer's fan-out endpoint URL from its advertise URL.
func deliverURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + DeliverPath
}

// marshalMessage serializes one message and its non-JSON fields (Sender, User) as an
// apiDeliverMessage line. Lines are marshaled once per publish and shared across all per-peer
// queues; assembleDeliverBody joins them without re-marshaling.
func marshalMessage(m *model.Message) ([]byte, error) {
	fanoutMessage := &apiDeliverMessage{User: m.User, Message: m}
	if m.Sender.IsValid() {
		fanoutMessage.Sender = m.Sender.String()
	}
	return json.Marshal(fanoutMessage)
}

// assembleDeliverBody builds an NDJSON fan-out request body from pre-marshaled apiDeliverMessage
// lines, avoiding a second JSON marshal of the messages.
func assembleDeliverBody(frags [][]byte) []byte {
	return append(bytes.Join(frags, []byte("\n")), '\n')
}

// decodeDeliverBody reads NDJSON apiDeliverMessage lines from r, reattaches the non-JSON fields
// (Sender, User) onto each message, and hands them to deliver. Malformed or message-less lines
// are skipped and logged, not fatal: fan-out is fire-and-forget, so the valid remainder of a
// request is still delivered. It returns an error only for stream-level failures (e.g. a line
// exceeding maxLineBytes).
func decodeDeliverBody(r io.Reader, maxLineBytes int, deliver DeliverFunc) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var fanoutMessage apiDeliverMessage
		if err := json.Unmarshal(line, &fanoutMessage); err != nil || fanoutMessage.Message == nil {
			log.Tag(tag).Warn("Skipping malformed fan-out line")
			continue
		}
		fanoutMessage.Message.User = fanoutMessage.User
		if fanoutMessage.Sender != "" {
			if addr, err := netip.ParseAddr(fanoutMessage.Sender); err == nil {
				fanoutMessage.Message.Sender = addr
			}
		}
		deliver(fanoutMessage.Message)
	}
	return scanner.Err()
}
