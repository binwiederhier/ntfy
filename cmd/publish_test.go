package cmd

import (
	"github.com/stretchr/testify/require"
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
