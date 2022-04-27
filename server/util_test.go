package server

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestReadBoolParam(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://ntfy.sh/mytopic?up=1&firebase=no", nil)
	up := readBoolParam(r, false, "x-up", "up")
	firebase := readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	r.Header.Set("X-Up", "yes")
	r.Header.Set("X-Firebase", "0")
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-up", "up")
	require.Equal(t, false, up)
	require.Equal(t, true, firebase)
}

func TestParseActions(t *testing.T) {
	actions, err := parseActions("[]")
	require.Nil(t, err)
	require.Empty(t, actions)

	actions, err = parseActions("action=http, label=Open door, url=https://door.lan/open; view, Show portal, https://door.lan")
	require.Nil(t, err)
	require.Equal(t, 2, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, "Open door", actions[0].Label)
	require.Equal(t, "https://door.lan/open", actions[0].URL)
	require.Equal(t, "view", actions[1].Action)
	require.Equal(t, "Show portal", actions[1].Label)
	require.Equal(t, "https://door.lan", actions[1].URL)

	actions, err = parseActions(`[{"action":"http","label":"Open door","url":"https://door.lan/open"}, {"action":"view","label":"Show portal","url":"https://door.lan"}]`)
	require.Nil(t, err)
	require.Equal(t, 2, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, "Open door", actions[0].Label)
	require.Equal(t, "https://door.lan/open", actions[0].URL)
	require.Equal(t, "view", actions[1].Action)
	require.Equal(t, "Show portal", actions[1].Label)
	require.Equal(t, "https://door.lan", actions[1].URL)

	actions, err = parseActions("action=http, label=Open door, url=https://door.lan/open, body=this is a body, method=PUT")
	require.Nil(t, err)
	require.Equal(t, 1, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, "Open door", actions[0].Label)
	require.Equal(t, "https://door.lan/open", actions[0].URL)
	require.Equal(t, "PUT", actions[0].Method)
	require.Equal(t, "this is a body", actions[0].Body)

	actions, err = parseActions("action=broadcast, label=Do a thing, extras.command=some command, extras.some_param=a parameter")
	require.Nil(t, err)
	require.Equal(t, 1, len(actions))
	require.Equal(t, "broadcast", actions[0].Action)
	require.Equal(t, "Do a thing", actions[0].Label)
	require.Equal(t, 2, len(actions[0].Extras))
	require.Equal(t, "some command", actions[0].Extras["command"])
	require.Equal(t, "a parameter", actions[0].Extras["some_param"])

	actions, err = parseActions("action=http, label=Send request, url=http://example.com, method=GET, headers.Content-Type=application/json, headers.Authorization=Basic sdasffsf")
	require.Nil(t, err)
	require.Equal(t, 1, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, "Send request", actions[0].Label)
	require.Equal(t, 2, len(actions[0].Headers))
	require.Equal(t, "application/json", actions[0].Headers["Content-Type"])
	require.Equal(t, "Basic sdasffsf", actions[0].Headers["Authorization"])

	actions, err = parseActions(`action=http, "Look ma, \"quotes\"; and semicolons", url=http://example.com`)
	require.Nil(t, err)
	require.Equal(t, 1, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, `Look ma, \"quotes\"; and semicolons`, actions[0].Label)
	require.Equal(t, `http://example.com`, actions[0].URL)

	actions, err = parseActions(`label="Out of order!" , action="http", url=http://example.com`)
	require.Nil(t, err)
	require.Equal(t, 1, len(actions))
	require.Equal(t, "http", actions[0].Action)
	require.Equal(t, `Out of order!`, actions[0].Label)
	require.Equal(t, `http://example.com`, actions[0].URL)

	actions, err = parseActions(`label="Out of order!" x, action="http", url=http://example.com`)
	require.EqualError(t, err, "unexpected character 'x' at position 22")

	actions, err = parseActions(`label="", action="http", url=http://example.com`)
	require.EqualError(t, err, "invalid request: actions invalid, parameter 'label' is required")

	actions, err = parseActions(`label=, action="http", url=http://example.com`)
	require.EqualError(t, err, "invalid request: actions invalid, parameter 'label' is required")

	actions, err = parseActions(`label="xx", action="http", url=http://example.com, what is this anyway`)
	require.EqualError(t, err, "invalid request: actions invalid, term 'what is this anyway' unknown")

}
