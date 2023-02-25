package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatrix_NewRequestFromMatrixJSON_Success(t *testing.T) {
	baseURL := "https://ntfy.sh"
	maxLength := 4096
	body := `{"notification":{"content":{"body":"I'm floating in a most peculiar way.","msgtype":"m.text"},"counts":{"missed_calls":1,"unread":2},"devices":[{"app_id":"org.matrix.matrixConsole.ios","data":{},"pushkey":"https://ntfy.sh/upABCDEFGHI?up=1","pushkey_ts":12345678,"tweaks":{"sound":"bing"}}],"event_id":"$3957tyerfgewrf384","prio":"high","room_alias":"#exampleroom:matrix.org","room_id":"!slw48wfj34rtnrf:example.com","room_name":"Mission Control","sender":"@exampleuser:matrix.org","sender_display_name":"Major Tom","type":"m.room.message"}}`
	r, _ := http.NewRequest("POST", "http://ntfy.example.com/_matrix/push/v1/notify", strings.NewReader(body))
	newRequest, err := newRequestFromMatrixJSON(r, baseURL, maxLength)
	require.Nil(t, err)
	require.Equal(t, "POST", newRequest.Method)
	require.Equal(t, "https://ntfy.sh/upABCDEFGHI?up=1", newRequest.URL.String())
	require.Equal(t, body, readAll(t, newRequest.Body))
}

func TestMatrix_NewRequestFromMatrixJSON_TooLarge(t *testing.T) {
	baseURL := "https://ntfy.sh"
	maxLength := 10 // Small
	body := `{"notification":{"content":{"body":"I'm floating in a most peculiar way.","msgtype":"m.text"},"counts":{"missed_calls":1,"unread":2},"devices":[{"app_id":"org.matrix.matrixConsole.ios","data":{},"pushkey":"https://ntfy.sh/upABCDEFGHI?up=1","pushkey_ts":12345678,"tweaks":{"sound":"bing"}}],"event_id":"$3957tyerfgewrf384","prio":"high","room_alias":"#exampleroom:matrix.org","room_id":"!slw48wfj34rtnrf:example.com","room_name":"Mission Control","sender":"@exampleuser:matrix.org","sender_display_name":"Major Tom","type":"m.room.message"}}`
	r, _ := http.NewRequest("POST", "http://ntfy.example.com/_matrix/push/v1/notify", strings.NewReader(body))
	_, err := newRequestFromMatrixJSON(r, baseURL, maxLength)
	require.Equal(t, errHTTPEntityTooLargeMatrixRequest, err)
}

func TestMatrix_NewRequestFromMatrixJSON_InvalidJSON(t *testing.T) {
	baseURL := "https://ntfy.sh"
	maxLength := 4096
	body := `this is not json`
	r, _ := http.NewRequest("POST", "http://ntfy.example.com/_matrix/push/v1/notify", strings.NewReader(body))
	_, err := newRequestFromMatrixJSON(r, baseURL, maxLength)
	require.Equal(t, errHTTPBadRequestMatrixMessageInvalid, err)
}

func TestMatrix_NewRequestFromMatrixJSON_NotAMatrixMessage(t *testing.T) {
	baseURL := "https://ntfy.sh"
	maxLength := 4096
	body := `{"message":"this is not a matrix message, but valid json"}`
	r, _ := http.NewRequest("POST", "http://ntfy.example.com/_matrix/push/v1/notify", strings.NewReader(body))
	_, err := newRequestFromMatrixJSON(r, baseURL, maxLength)
	require.Equal(t, errHTTPBadRequestMatrixMessageInvalid, err)
}

func TestMatrix_NewRequestFromMatrixJSON_MismatchingPushKey(t *testing.T) {
	baseURL := "https://ntfy.sh" // Mismatch!
	maxLength := 4096
	body := `{"notification":{"content":{"body":"I'm floating in a most peculiar way.","msgtype":"m.text"},"counts":{"missed_calls":1,"unread":2},"devices":[{"app_id":"org.matrix.matrixConsole.ios","data":{},"pushkey":"https://ntfy.example.com/upABCDEFGHI?up=1","pushkey_ts":12345678,"tweaks":{"sound":"bing"}}],"event_id":"$3957tyerfgewrf384","prio":"high","room_alias":"#exampleroom:matrix.org","room_id":"!slw48wfj34rtnrf:example.com","room_name":"Mission Control","sender":"@exampleuser:matrix.org","sender_display_name":"Major Tom","type":"m.room.message"}}`
	r, _ := http.NewRequest("POST", "http://ntfy.example.com/_matrix/push/v1/notify", strings.NewReader(body))
	_, err := newRequestFromMatrixJSON(r, baseURL, maxLength)
	matrixErr, ok := err.(*errMatrixPushkeyRejected)
	require.True(t, ok)
	require.Equal(t, "push key must be prefixed with base URL, received push key: https://ntfy.example.com/upABCDEFGHI?up=1, configured base URL: https://ntfy.sh", matrixErr.Error())
	require.Equal(t, "https://ntfy.example.com/upABCDEFGHI?up=1", matrixErr.rejectedPushKey)
}

func TestMatrix_WriteMatrixDiscoveryResponse(t *testing.T) {
	w := httptest.NewRecorder()
	require.Nil(t, writeMatrixDiscoveryResponse(w))
	require.Equal(t, 200, w.Result().StatusCode)
	require.Equal(t, `{"unifiedpush":{"gateway":"matrix"}}`+"\n", w.Body.String())
}

func TestMatrix_WriteMatrixError(t *testing.T) {
	w := httptest.NewRecorder()
	require.Nil(t, writeMatrixResponse(w, "https://ntfy.example.com/upABCDEFGHI?up=1"))
	require.Equal(t, 200, w.Result().StatusCode)
	require.Equal(t, `{"rejected":["https://ntfy.example.com/upABCDEFGHI?up=1"]}`+"\n", w.Body.String())
}

func TestMatrix_WriteMatrixSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	require.Nil(t, writeMatrixSuccess(w))
	require.Equal(t, 200, w.Result().StatusCode)
	require.Equal(t, `{"rejected":[]}`+"\n", w.Body.String())
}
