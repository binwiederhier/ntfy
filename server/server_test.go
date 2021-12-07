package server

import (
	"bufio"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/config"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestServer_PublishAndPoll(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response1 := request(t, s, "PUT", "/mytopic", "my first message")
	msg1 := toMessage(t, response1.Body.String())
	assert.NotEmpty(t, msg1.ID)
	assert.Equal(t, "my first message", msg1.Message)

	response2 := request(t, s, "PUT", "/mytopic", "my second message")
	msg2 := toMessage(t, response2.Body.String())
	assert.NotEqual(t, msg1.ID, msg2.ID)
	assert.NotEmpty(t, msg2.ID)
	assert.Equal(t, "my second message", msg2.Message)

	response := request(t, s, "GET", "/mytopic/json?poll=1", "")
	messages := toMessages(t, response.Body.String())
	assert.Equal(t, 2, len(messages))
}

func TestServer_PublishAndSubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response1 := request(t, s, "PUT", "/mytopic", "my first message")
	msg1 := toMessage(t, response1.Body.String())
	assert.NotEmpty(t, msg1.ID)
	assert.Equal(t, "my first message", msg1.Message)

	response2 := request(t, s, "PUT", "/mytopic", "my second message")
	msg2 := toMessage(t, response2.Body.String())
	assert.NotEqual(t, msg1.ID, msg2.ID)
	assert.NotEmpty(t, msg2.ID)
	assert.Equal(t, "my second message", msg2.Message)

	response := request(t, s, "GET", "/mytopic/json?poll=1", "")
	messages := toMessages(t, response.Body.String())
	assert.Equal(t, 2, len(messages))
}

func newTestConfig(t *testing.T) *config.Config {
	conf := config.New(":80")
	conf.CacheFile = filepath.Join(t.TempDir(), "cache.db")
	return conf
}

func newTestServer(t *testing.T, config *config.Config) *Server {
	server, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func request(t *testing.T, s *Server, method, url, body string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	s.handle(rr, req)
	return rr
}

func toMessages(t *testing.T, s string) []*message {
	messages := make([]*message, 0)
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		messages = append(messages, toMessage(t, scanner.Text()))
	}
	return messages
}

func toMessage(t *testing.T, s string) *message {
	var m message
	assert.Nil(t, json.NewDecoder(strings.NewReader(s)).Decode(&m))
	return &m
}
