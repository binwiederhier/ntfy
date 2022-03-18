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

func TestCLI_Publish_All_The_Things(t *testing.T) {
	s, port := test.StartServer(t)
	defer test.StopServer(t, s, port)
	topic := fmt.Sprintf("http://127.0.0.1:%d/mytopic", port)

	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{
		"ntfy", "publish",
		"--title", "this is a title",
		"--priority", "high",
		"--tags", "tag1,tag2",
		// No --delay, --email
		"--click", "https://ntfy.sh",
		"--attach", "https://f-droid.org/F-Droid.apk",
		"--filename", "fdroid.apk",
		"--no-cache",
		"--no-firebase",
		topic,
		"some message",
	}))
	m := toMessage(t, stdout.String())
	require.Equal(t, "message", m.Event)
	require.Equal(t, "mytopic", m.Topic)
	require.Equal(t, "some message", m.Message)
	require.Equal(t, "this is a title", m.Title)
	require.Equal(t, 4, m.Priority)
	require.Equal(t, []string{"tag1", "tag2"}, m.Tags)
	require.Equal(t, "https://ntfy.sh", m.Click)
	require.Equal(t, "https://f-droid.org/F-Droid.apk", m.Attachment.URL)
	require.Equal(t, "fdroid.apk", m.Attachment.Name)
	require.Equal(t, int64(0), m.Attachment.Size)
	require.Equal(t, "", m.Attachment.Owner)
	require.Equal(t, int64(0), m.Attachment.Expires)
	require.Equal(t, "", m.Attachment.Type)
}
