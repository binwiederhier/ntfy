package client_test

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/client"
	"heckel.io/ntfy/server"
	"net/http"
	"testing"
	"time"
)

func TestClient_Publish(t *testing.T) {
	s := startTestServer(t)
	defer s.Stop()
	c := client.New(newTestConfig())

	time.Sleep(time.Second) // FIXME Wait for port up

	_, err := c.Publish("mytopic", "some message")
	require.Nil(t, err)
}

func newTestConfig() *client.Config {
	c := client.NewConfig()
	c.DefaultHost = "http://127.0.0.1:12345"
	return c
}

func startTestServer(t *testing.T) *server.Server {
	conf := server.NewConfig()
	conf.ListenHTTP = ":12345"
	s, err := server.New(conf)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := s.Run(); err != nil && err != http.ErrServerClosed {
			panic(err) // 'go vet' complains about 't.Fatal(err)'
		}
	}()
	return s
}
