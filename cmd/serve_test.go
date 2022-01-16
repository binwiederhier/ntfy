package cmd

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/test"
	"heckel.io/ntfy/util"
	"math/rand"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixMilli())
}

func TestCLI_Serve_Unix_Curl(t *testing.T) {
	sockFile := filepath.Join(t.TempDir(), "ntfy.sock")
	go func() {
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", "--listen-http=-", "--listen-unix=" + sockFile})
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
		app, _, _, _ := newTestApp()
		err := app.Run([]string{"ntfy", "serve", fmt.Sprintf("--listen-http=:%d", port)})
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
