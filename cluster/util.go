package cluster

import (
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

// marshalEnvelope serializes a message and its non-JSON fields for transport to peer nodes.
func marshalEnvelope(origin string, m *model.Message) ([]byte, error) {
	env := &envelope{Kind: envelopeKindMessage, Origin: origin, User: m.User, Message: m}
	if m.Sender.IsValid() {
		env.Sender = m.Sender.String()
	}
	return json.Marshal(env)
}

// unmarshalEnvelope parses a wire envelope and, for message envelopes, reattaches the non-JSON
// fields (Sender, User) onto the message. Callers must check the envelope kind and ignore kinds
// they do not understand.
func unmarshalEnvelope(data []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Kind == envelopeKindMessage {
		if env.Message == nil {
			return nil, errors.New("message envelope without message")
		}
		env.Message.User = env.User
		if env.Sender != "" {
			if addr, err := netip.ParseAddr(env.Sender); err == nil {
				env.Message.Sender = addr
			}
		}
	}
	return &env, nil
}
