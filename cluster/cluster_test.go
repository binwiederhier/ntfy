package cluster

import (
	"bytes"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
)

func TestFanout_RoundTrip(t *testing.T) {
	// The fan-out body is NDJSON: one apiFanoutMessage per line, joined from pre-marshaled
	// fragments; the origin travels in a header, not the body
	m1 := model.NewDefaultMessage("mytopic", "my message")
	m1.Sender = netip.MustParseAddr("1.2.3.4")
	m1.User = "u_abc"
	m2 := model.NewDefaultMessage("othertopic", "other message")
	frag1, err := marshalMessage(m1)
	require.Nil(t, err)
	frag2, err := marshalMessage(m2)
	require.Nil(t, err)
	messages, err := unmarshalFanoutBody(assembleFanoutBody([][]byte{frag1, frag2}), 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "mytopic", messages[0].Topic)
	require.Equal(t, "my message", messages[0].Message)
	// Sender and User are json:"-" on model.Message; the lines must carry and reattach them
	require.Equal(t, netip.MustParseAddr("1.2.3.4"), messages[0].Sender)
	require.Equal(t, "u_abc", messages[0].User)
	require.Equal(t, "othertopic", messages[1].Topic)
	require.False(t, messages[1].Sender.IsValid())
}

func TestFanout_SingleMessage(t *testing.T) {
	// A single message is just a one-line body; there is no separate single-message format
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "hi"))
	require.Nil(t, err)
	messages, err := unmarshalFanoutBody(assembleFanoutBody([][]byte{frag}), 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 1)
}

func TestFanout_MalformedLinesSkipped(t *testing.T) {
	// Fan-out is fire-and-forget: a malformed or message-less line is skipped (and logged), the
	// remaining lines are still delivered
	frag, err := marshalMessage(model.NewDefaultMessage("mytopic", "good"))
	require.Nil(t, err)
	body := []byte("this is not json\n{\"sender\":\"1.2.3.4\"}\n" + string(frag) + "\n\n")
	messages, err := unmarshalFanoutBody(body, 1<<20)
	require.Nil(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "good", messages[0].Message)
}

// unmarshalFanoutBody is a test helper collecting the messages of an NDJSON fan-out body.
func unmarshalFanoutBody(body []byte, maxLineBytes int) ([]*model.Message, error) {
	var messages []*model.Message
	err := decodeFanout(bytes.NewReader(body), maxLineBytes, func(m *model.Message) {
		messages = append(messages, m)
	})
	return messages, err
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
