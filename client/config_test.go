package client

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Load(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
subscribe:
  - topic: no-command
  - topic: echo-this
    command: 'echo "Message received: $message"'
  - topic: alerts
    command: notify-send -i /usr/share/ntfy/logo.png "Important" "$m"
    if:
            priority: high,urgent
`), 0600))

	conf, err := LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, 3, len(conf.Subscribe))
	require.Equal(t, "no-command", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].Command)
	require.Equal(t, "echo-this", conf.Subscribe[1].Topic)
	require.Equal(t, `echo "Message received: $message"`, conf.Subscribe[1].Command)
	require.Equal(t, "alerts", conf.Subscribe[2].Topic)
	require.Equal(t, `notify-send -i /usr/share/ntfy/logo.png "Important" "$m"`, conf.Subscribe[2].Command)
	require.Equal(t, "high,urgent", conf.Subscribe[2].If["priority"])
}
