package cluster

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
)

func TestBatch_RoundTrip(t *testing.T) {
	m1 := model.NewDefaultMessage("mytopic", "my message")
	m1.Sender = netip.MustParseAddr("1.2.3.4")
	m1.User = "u_abc"
	m2 := model.NewDefaultMessage("othertopic", "other message")
	frag1, err := marshalMessage(m1)
	require.Nil(t, err)
	frag2, err := marshalMessage(m2)
	require.Nil(t, err)
	batch, err := unmarshalBatch(assembleBatch("node-a", [][]byte{frag1, frag2}))
	require.Nil(t, err)
	require.Equal(t, envelopeKindBatch, batch.Kind)
	require.Equal(t, "node-a", batch.Origin)
	require.Len(t, batch.Messages, 2)
	require.Equal(t, "mytopic", batch.Messages[0].Message.Topic)
	require.Equal(t, "my message", batch.Messages[0].Message.Message)
	// Sender and User are json:"-" on model.Message; the batch must carry and reattach them
	require.Equal(t, netip.MustParseAddr("1.2.3.4"), batch.Messages[0].Message.Sender)
	require.Equal(t, "u_abc", batch.Messages[0].Message.User)
	require.Equal(t, "othertopic", batch.Messages[1].Message.Topic)
	require.False(t, batch.Messages[1].Message.Sender.IsValid())
}

func TestBatch_SingleMessage(t *testing.T) {
	// A batch of one is the degenerate case; there is no separate single-message format
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "hi"))
	require.Nil(t, err)
	batch, err := unmarshalBatch(assembleBatch("node-a", [][]byte{frag}))
	require.Nil(t, err)
	require.Len(t, batch.Messages, 1)
}

func TestBatch_UnknownKind(t *testing.T) {
	// Unknown kinds must parse without error so receivers can ignore them; this keeps
	// mixed-version clusters working during rolling deploys when a newer node introduces a new
	// envelope kind.
	batch, err := unmarshalBatch([]byte(`{"kind":"bloom-gossip","origin":"node-b","filter":"xyz"}`))
	require.Nil(t, err)
	require.Equal(t, "bloom-gossip", batch.Kind)
	require.Empty(t, batch.Messages)
}

func TestBatch_EntryWithoutMessage(t *testing.T) {
	_, err := unmarshalBatch([]byte(`{"kind":"batch","origin":"node-b","messages":[{"sender":"1.2.3.4"}]}`))
	require.Error(t, err)
}

func TestNop(t *testing.T) {
	b, err := New(&Config{}, nil, nil) // not enabled -> nop cluster, no database required
	require.Nil(t, err)
	require.IsType(t, &nopCluster{}, b)
	require.Nil(t, b.Broadcast(model.NewDefaultMessage("mytopic", "hi")))
	// A single node is trivially the leader, so leader-gated jobs run without special-casing
	require.True(t, b.IsLeader())
	rr := httptest.NewRecorder()
	b.ServeFanout(rr, httptest.NewRequest("POST", FanoutPath, nil))
	require.Equal(t, 404, rr.Code)
	require.Nil(t, b.Close())
}

func TestNew_EnabledRequiresDatabase(t *testing.T) {
	_, err := New(&Config{Enabled: true, Secret: "secret"}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database")
}
