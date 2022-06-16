package server

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
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
	require.Equal(t, "https://ntfy.sh/upABCDEFGHI?up=1", newRequest.Header.Get("X-Matrix-Pushkey"))
	require.Equal(t, body, readAll(t, newRequest.Body))
}
