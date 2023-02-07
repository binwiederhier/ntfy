package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/test"
	"heckel.io/ntfy/util"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCLI_Publish_Subscribe_Poll_Real_Server(t *testing.T) {
	testMessage := util.RandomString(10)
	app, _, _, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "ntfytest", "ntfy unit test " + testMessage}))

	_, err := util.Retry(func() (*int, error) {
		app2, _, stdout, _ := newTestApp()
		if err := app2.Run([]string{"ntfy", "subscribe", "--poll", "ntfytest"}); err != nil {
			return nil, err
		}
		if !strings.Contains(stdout.String(), testMessage) {
			return nil, fmt.Errorf("test message %s not found in topic", testMessage)
		}
		return util.Int(1), nil
	}, time.Second, 2*time.Second, 5*time.Second) // Since #502, ntfy.sh writes messages to the cache asynchronously, after a timeout of ~1.5s
	require.Nil(t, err)
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
		"--icon", "https://ntfy.sh/static/img/ntfy.png",
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
	require.Equal(t, "https://ntfy.sh/static/img/ntfy.png", m.Icon)
}

func TestCLI_Publish_Wait_PID_And_Cmd(t *testing.T) {
	s, port := test.StartServer(t)
	defer test.StopServer(t, s, port)
	topic := fmt.Sprintf("http://127.0.0.1:%d/mytopic", port)

	// Test: sleep 0.5
	sleep := exec.Command("sleep", "0.5")
	require.Nil(t, sleep.Start())
	go sleep.Wait() // Must be called to release resources
	start := time.Now()
	app, _, stdout, _ := newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "--wait-pid", strconv.Itoa(sleep.Process.Pid), topic}))
	m := toMessage(t, stdout.String())
	require.True(t, time.Since(start) >= 500*time.Millisecond)
	require.Regexp(t, `Process with PID \d+ exited after `, m.Message)

	// Test: PID does not exist
	app, _, _, _ = newTestApp()
	err := app.Run([]string{"ntfy", "publish", "--wait-pid", "1234567", topic})
	require.Error(t, err)
	require.Equal(t, "process with PID 1234567 not running", err.Error())

	// Test: Successful command (exit 0)
	start = time.Now()
	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "--wait-cmd", topic, "sleep", "0.5"}))
	m = toMessage(t, stdout.String())
	require.True(t, time.Since(start) >= 500*time.Millisecond)
	require.Contains(t, m.Message, `Command succeeded after `)
	require.Contains(t, m.Message, `: sleep 0.5`)

	// Test: Failing command (exit 1)
	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "--wait-cmd", topic, "/bin/false", "false doesn't care about its args"}))
	m = toMessage(t, stdout.String())
	require.Contains(t, m.Message, `Command failed after `)
	require.Contains(t, m.Message, `(exit code 1): /bin/false "false doesn't care about its args"`, m.Message)

	// Test: Non-existing command (hard fail!)
	app, _, _, _ = newTestApp()
	err = app.Run([]string{"ntfy", "publish", "--wait-cmd", topic, "does-not-exist-no-really", "really though"})
	require.Error(t, err)
	require.Equal(t, `command failed: does-not-exist-no-really "really though", error: exec: "does-not-exist-no-really": executable file not found in $PATH`, err.Error())

	// Tests with NTFY_TOPIC set ////
	require.Nil(t, os.Setenv("NTFY_TOPIC", topic))

	// Test: Successful command with NTFY_TOPIC
	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "--env-topic", "--cmd", "echo", "hi there"}))
	m = toMessage(t, stdout.String())
	require.Equal(t, "mytopic", m.Topic)

	// Test: Successful --wait-pid with NTFY_TOPIC
	sleep = exec.Command("sleep", "0.2")
	require.Nil(t, sleep.Start())
	go sleep.Wait() // Must be called to release resources
	app, _, stdout, _ = newTestApp()
	require.Nil(t, app.Run([]string{"ntfy", "publish", "--env-topic", "--wait-pid", strconv.Itoa(sleep.Process.Pid)}))
	m = toMessage(t, stdout.String())
	require.Regexp(t, `Process with PID \d+ exited after .+ms`, m.Message)
}
