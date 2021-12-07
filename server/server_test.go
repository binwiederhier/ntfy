package server

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_Publish(t *testing.T) {
	s := newTestServer(t, newTestConfig())

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/mytopic", strings.NewReader("my message"))
	s.handle(rr, req)

	var m message
	assert.Nil(t, json.NewDecoder(rr.Body).Decode(&m))
	assert.NotEmpty(t, m.ID)
	assert.Equal(t, "my message", m.Message)
}

func newTestConfig() *config.Config {
	return config.New(":80")
}

func newTestServer(t *testing.T, config *config.Config) *Server {
	server, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return server
}
