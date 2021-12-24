package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/test"
	"heckel.io/ntfy/util"
	"testing"
)

func TestCLI_Publish_Subscribe_Poll_Real_Server(t *testing.T) {
	testMessage := util.RandomString(10)

	app, _, _, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "ntfytest", "ntfy unit test " + testMessage}))

	app2, _, stdout, _ := newTestApp()
	require.Nil(t, app2.Run([]string{"ntfy", "subscribe", "--poll", "ntfytest"}))
	require.Contains(t, stdout.String(), testMessage)
}

func TestCLI_Publish_Subscribe_Poll(t *testing.T) {
	s, port := test.StartServer(t)
	defer test.StopServer(t, s, port)
	topic := fmt.Sprintf("http://127.0.0.1:%d/mytopic", port)

	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", topic, "some message"}))
	m := toMessage(t, stdout.String())
	require.Equal(t, "some message", m.Message)

	app2, _, stdout, _ := newTestApp()
	require.Nil(t, app2.Run([]string{"ntfy", "subscribe", "--poll", topic}))
	m = toMessage(t, stdout.String())
	require.Equal(t, "some message", m.Message)
}
