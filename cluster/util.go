package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"strings"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const randomNodeIDLength = 12

// defaultNodeID returns the hostname, or a random string if it is unavailable. The hostname is
// preferred because it is stable across restarts: a node that restarts under the same ID reuses
// its registry row instead of abandoning it, and log/metric labels stay traceable.
func defaultNodeID() string {
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}
	return util.RandomString(randomNodeIDLength)
}

// fanoutURL derives the peer's fan-out endpoint URL from its advertise URL.
func fanoutURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + FanoutPath
}

// marshalMessage serializes one message and its non-JSON fields (Sender, User) as an
// apiBatchMessage fragment. Fragments are marshaled once per publish and shared across all
// per-peer queues; assembleBatch joins them without re-marshaling.
func marshalMessage(m *model.Message) ([]byte, error) {
	batchMessage := &apiBatchMessage{User: m.User, Message: m}
	if m.Sender.IsValid() {
		batchMessage.Sender = m.Sender.String()
	}
	return json.Marshal(batchMessage)
}

// assembleBatch builds a batch request body from pre-marshaled apiBatchMessage fragments by
// joining them, avoiding a second JSON marshal of the messages.
func assembleBatch(origin string, frags [][]byte) []byte {
	originJSON, _ := json.Marshal(origin) // Marshaling a string cannot fail
	var buf bytes.Buffer
	buf.WriteString(`{"kind":"` + envelopeKindBatch + `","origin":`)
	buf.Write(originJSON)
	buf.WriteString(`,"messages":[`)
	for i, frag := range frags {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(frag)
	}
	buf.WriteString(`]}`)
	return buf.Bytes()
}

// unmarshalBatch parses a wire batch and reattaches the non-JSON fields (Sender, User) onto each
// message. Callers must check the batch kind and ignore kinds they do not understand.
func unmarshalBatch(data []byte) (*apiBatch, error) {
	var batch apiBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, err
	}
	if batch.Kind == envelopeKindBatch {
		for _, batchMessage := range batch.Messages {
			if batchMessage.Message == nil {
				return nil, errors.New("batch entry without message")
			}
			batchMessage.Message.User = batchMessage.User
			if batchMessage.Sender != "" {
				if addr, err := netip.ParseAddr(batchMessage.Sender); err == nil {
					batchMessage.Message.Sender = addr
				}
			}
		}
	}
	return &batch, nil
}
