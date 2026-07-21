package cluster

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
)

func TestEnvelope_RoundTrip(t *testing.T) {
	m := model.NewDefaultMessage("mytopic", "my message")
	m.Sender = netip.MustParseAddr("1.2.3.4")
	m.User = "u_abc"
	data, err := marshalEnvelope("node-a", m)
	require.Nil(t, err)
	env, err := unmarshalEnvelope(data)
	require.Nil(t, err)
	require.Equal(t, envelopeKindMessage, env.Kind)
	require.Equal(t, "node-a", env.Origin)
	require.Equal(t, "mytopic", env.Message.Topic)
	require.Equal(t, "my message", env.Message.Message)
	// Sender and User are json:"-" on model.Message; the envelope must carry and reattach them
	require.Equal(t, netip.MustParseAddr("1.2.3.4"), env.Message.Sender)
	require.Equal(t, "u_abc", env.Message.User)
}

func TestEnvelope_UnknownKind(t *testing.T) {
	// Unknown envelope kinds must parse without error so receivers can ignore them; this keeps
	// mixed-version clusters working during rolling deploys when a newer node introduces a new
	// envelope kind.
	env, err := unmarshalEnvelope([]byte(`{"kind":"bloom-gossip","origin":"node-b","filter":"xyz"}`))
	require.Nil(t, err)
	require.Equal(t, "bloom-gossip", env.Kind)
	require.Nil(t, env.Message)
}

func TestEnvelope_MessageKindWithoutMessage(t *testing.T) {
	_, err := unmarshalEnvelope([]byte(`{"kind":"message","origin":"node-b"}`))
	require.Error(t, err)
}

func TestNop(t *testing.T) {
	b, err := New(Config{}, nil, nil) // not enabled -> Nop, no database required
	require.Nil(t, err)
	require.IsType(t, &Nop{}, b)
	require.Nil(t, b.Broadcast(model.NewDefaultMessage("mytopic", "hi")))
	// A single node is trivially the leader, so leader-gated jobs run without special-casing
	require.True(t, b.IsLeader())
	rr := httptest.NewRecorder()
	b.ServeFanout(rr, httptest.NewRequest("POST", FanoutPath, nil))
	require.Equal(t, 404, rr.Code)
	require.Nil(t, b.Close())
}

func TestNew_EnabledRequiresDatabase(t *testing.T) {
	_, err := New(Config{Enabled: true, Secret: "secret"}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database")
}
