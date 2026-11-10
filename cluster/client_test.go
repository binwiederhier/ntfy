package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testClientRequest is what a fake peer saw of one request
type testClientRequest struct {
	method, path, contentType, secret, origin, body string
}

// newTestClientServer answers every request with the given status and body, and records the request
func newTestClientServer(t *testing.T, status int, body string) (*httptest.Server, *testClientRequest) {
	seen := &testClientRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*seen = testClientRequest{r.Method, r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get(secretHeader), r.Header.Get(originHeader), string(b)}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func TestClient_SendMessages(t *testing.T) {
	srv, seen := newTestClientServer(t, http.StatusOK, "")
	client := NewClient(testSecret, "node-a")
	require.Nil(t, client.SendMessages(context.Background(), srv.URL+"/", []byte("line1\nline2\n")))
	require.Equal(t, testClientRequest{http.MethodPost, MessagePath, contentTypeNDJSON, testSecret, "node-a", "line1\nline2\n"}, *seen)
}

func TestClient_SendState(t *testing.T) {
	srv, seen := newTestClientServer(t, http.StatusOK, "")
	client := NewClient(testSecret, "node-a")
	require.Nil(t, client.SendState(context.Background(), srv.URL, []byte(`{"v":1}`)))
	require.Equal(t, testClientRequest{http.MethodPost, StatePath, contentTypeJSON, testSecret, "node-a", `{"v":1}`}, *seen)
}

func TestClient_Status(t *testing.T) {
	body, err := json.Marshal(&Status{NodeID: "node-b", Role: RoleActive, Leader: true, Healthy: true, Time: 1792330000, Peers: []*PeerStatus{{NodeID: "node-a", Out: PeerOutbound{BatchesFailed: 3}}}})
	require.Nil(t, err)
	srv, seen := newTestClientServer(t, http.StatusOK, string(body))
	status, err := NewClient(testSecret, "").Status(context.Background(), srv.URL)
	require.Nil(t, err)
	require.NotNil(t, status)
	require.Equal(t, NodeID("node-b"), status.NodeID)
	require.Equal(t, int64(3), status.Peers[0].Out.BatchesFailed)
	require.Equal(t, http.MethodGet, seen.method)
	require.Equal(t, StatusPath, seen.path)
	require.Equal(t, testSecret, seen.secret)
	require.Equal(t, "", seen.origin) // Not a node: no origin header
}

func TestClient_NonOKIsHTTPError(t *testing.T) {
	srv, _ := newTestClientServer(t, http.StatusUnauthorized, "")
	client := NewClient("wrong", "node-a")
	var httpErr *HTTPError
	_, err := client.Status(context.Background(), srv.URL)
	require.True(t, errors.As(err, &httpErr))
	require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode)
	require.ErrorContains(t, err, "401")
	err = client.SendMessages(context.Background(), srv.URL, []byte("x\n"))
	require.True(t, errors.As(err, &httpErr))
	require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode)
}

func TestClient_ContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := NewClient(testSecret, "node-a").SendMessages(ctx, srv.URL, []byte("x\n"))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

func TestClient_Health(t *testing.T) {
	client := NewClient(testSecret, "node-a")
	srv, seen := newTestClientServer(t, http.StatusOK, `{"healthy":true}`)
	healthy, err := client.Health(context.Background(), srv.URL)
	require.Nil(t, err)
	require.True(t, healthy)
	require.Equal(t, HealthPath, seen.path)
	require.Equal(t, testSecret, seen.secret)

	srv, _ = newTestClientServer(t, http.StatusOK, `{"healthy":false}`)
	healthy, err = client.Health(context.Background(), srv.URL)
	require.Nil(t, err)
	require.False(t, healthy)

	srv, _ = newTestClientServer(t, http.StatusServiceUnavailable, `{"healthy":true}`)
	healthy, err = client.Health(context.Background(), srv.URL)
	require.Error(t, err)
	require.False(t, healthy)

	srv, _ = newTestClientServer(t, http.StatusOK, "OK") // A proxy or parked domain, not a ntfy node
	healthy, err = client.Health(context.Background(), srv.URL)
	require.Error(t, err)
	require.False(t, healthy)
}
