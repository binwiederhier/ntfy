package cmd

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/test"
	"heckel.io/ntfy/util"
)

func init() {
	rand.Seed(time.Now().UnixMilli())
}

func TestCLI_Serve_Unix_Curl(t *testing.T) {
	sockFile := filepath.Join(t.TempDir(), "ntfy.sock")
	configFile := newEmptyFile(t) // Avoid issues with existing server.yml file on system
	go func() {
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", "--config=" + configFile, "--listen-http=-", "--listen-unix=" + sockFile})
		require.Nil(t, err)
	}()
	for i := 0; i < 40 && !util.FileExists(sockFile); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, util.FileExists(sockFile))

	cmd := exec.Command("curl", "-s", "--unix-socket", sockFile, "-d", "this is a message", "localhost/mytopic")
	out, err := cmd.Output()
	require.Nil(t, err)
	m := toMessage(t, string(out))
	require.Equal(t, "this is a message", m.Message)
}

func TestCLI_Serve_WebSocket(t *testing.T) {
	port := 10000 + rand.Intn(20000)
	go func() {
		configFile := newEmptyFile(t) // Avoid issues with existing server.yml file on system
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", "--config=" + configFile, fmt.Sprintf("--listen-http=:%d", port)})
		require.Nil(t, err)
	}()
	test.WaitForPortUp(t, port)

	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/mytopic/ws", port), nil)
	require.Nil(t, err)

	messageType, data, err := ws.ReadMessage()
	require.Nil(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, "open", toMessage(t, string(data)).Event)

	c := client.New(client.NewConfig())
	_, err = c.Publish(fmt.Sprintf("http://127.0.0.1:%d/mytopic", port), "my message")
	require.Nil(t, err)

	messageType, data, err = ws.ReadMessage()
	require.Nil(t, err)
	require.Equal(t, websocket.TextMessage, messageType)

	m := toMessage(t, string(data))
	require.Equal(t, "my message", m.Message)
	require.Equal(t, "mytopic", m.Topic)
}

func TestIP_Host_Parsing(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1":          "1.1.1.1/32",
		"fd00::1234":       "fd00::1234/128",
		"192.168.0.3/24":   "192.168.0.0/24",
		"10.1.2.3/8":       "10.0.0.0/8",
		"201:be93::4a6/21": "201:b800::/21",
	}
	for q, expectedAnswer := range cases {
		ips, err := parseIPHostPrefix(q)
		require.Nil(t, err)
		assert.Equal(t, 1, len(ips))
		assert.Equal(t, expectedAnswer, ips[0].String())
	}
}

func newEmptyFile(t *testing.T) string {
	filename := filepath.Join(t.TempDir(), "empty")
	require.Nil(t, os.WriteFile(filename, []byte{}, 0600))
	return filename
}
