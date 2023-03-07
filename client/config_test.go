package client_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/client"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Load(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
default-user: philipp
default-password: mypass
default-command: 'echo "Got the message: $message"'
subscribe:
  - topic: no-command-with-auth
    user: phil
    password: mypass
  - topic: echo-this
    command: 'echo "Message received: $message"'
  - topic: alerts
    command: notify-send -i /usr/share/ntfy/logo.png "Important" "$m"
    if:
            priority: high,urgent
  - topic: defaults
`), 0600))

	conf, err := client.LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, "philipp", conf.DefaultUser)
	require.Equal(t, "mypass", *conf.DefaultPassword)
	require.Equal(t, `echo "Got the message: $message"`, conf.DefaultCommand)
	require.Equal(t, 4, len(conf.Subscribe))
	require.Equal(t, "no-command-with-auth", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].Command)
	require.Equal(t, "phil", conf.Subscribe[0].User)
	require.Equal(t, "mypass", *conf.Subscribe[0].Password)
	require.Equal(t, "echo-this", conf.Subscribe[1].Topic)
	require.Equal(t, `echo "Message received: $message"`, conf.Subscribe[1].Command)
	require.Equal(t, "alerts", conf.Subscribe[2].Topic)
	require.Equal(t, `notify-send -i /usr/share/ntfy/logo.png "Important" "$m"`, conf.Subscribe[2].Command)
	require.Equal(t, "high,urgent", conf.Subscribe[2].If["priority"])
	require.Equal(t, "defaults", conf.Subscribe[3].Topic)
}

func TestConfig_EmptyPassword(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
default-user: philipp
default-password: ""
subscribe:
  - topic: no-command-with-auth
    user: phil
    password: ""
`), 0600))

	conf, err := client.LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, "philipp", conf.DefaultUser)
	require.Equal(t, "", *conf.DefaultPassword)
	require.Equal(t, 1, len(conf.Subscribe))
	require.Equal(t, "no-command-with-auth", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].Command)
	require.Equal(t, "phil", conf.Subscribe[0].User)
	require.Equal(t, "", *conf.Subscribe[0].Password)
}

func TestConfig_NullPassword(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
default-user: philipp
default-password: ~
subscribe:
  - topic: no-command-with-auth
    user: phil
    password: ~
`), 0600))

	conf, err := client.LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, "philipp", conf.DefaultUser)
	require.Nil(t, conf.DefaultPassword)
	require.Equal(t, 1, len(conf.Subscribe))
	require.Equal(t, "no-command-with-auth", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].Command)
	require.Equal(t, "phil", conf.Subscribe[0].User)
	require.Nil(t, conf.Subscribe[0].Password)
}

func TestConfig_NoPassword(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
default-user: philipp
subscribe:
  - topic: no-command-with-auth
    user: phil
`), 0600))

	conf, err := client.LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, "philipp", conf.DefaultUser)
	require.Nil(t, conf.DefaultPassword)
	require.Equal(t, 1, len(conf.Subscribe))
	require.Equal(t, "no-command-with-auth", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].Command)
	require.Equal(t, "phil", conf.Subscribe[0].User)
	require.Nil(t, conf.Subscribe[0].Password)
}

func TestConfig_DefaultToken(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "client.yml")
	require.Nil(t, os.WriteFile(filename, []byte(`
default-host: http://localhost
default-token: tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
subscribe:
  - topic: mytopic
`), 0600))

	conf, err := client.LoadConfig(filename)
	require.Nil(t, err)
	require.Equal(t, "http://localhost", conf.DefaultHost)
	require.Equal(t, "", conf.DefaultUser)
	require.Nil(t, conf.DefaultPassword)
	require.Equal(t, "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", conf.DefaultToken)
	require.Equal(t, 1, len(conf.Subscribe))
	require.Equal(t, "mytopic", conf.Subscribe[0].Topic)
	require.Equal(t, "", conf.Subscribe[0].User)
	require.Nil(t, conf.Subscribe[0].Password)
	require.Equal(t, "", conf.Subscribe[0].Token)
}
