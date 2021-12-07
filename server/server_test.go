package server

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/config"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServer_PublishAndPoll(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response1 := request(t, s, "PUT", "/mytopic", "my first message", nil)
	msg1 := toMessage(t, response1.Body.String())
	assert.NotEmpty(t, msg1.ID)
	assert.Equal(t, "my first message", msg1.Message)

	response2 := request(t, s, "PUT", "/mytopic", "my second message", nil)
	msg2 := toMessage(t, response2.Body.String())
	assert.NotEqual(t, msg1.ID, msg2.ID)
	assert.NotEmpty(t, msg2.ID)
	assert.Equal(t, "my second message", msg2.Message)

	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	assert.Equal(t, 2, len(messages))
	assert.Equal(t, "my first message", messages[0].Message)
	assert.Equal(t, "my second message", messages[1].Message)
}

func TestServer_SubscribeOpenAndKeepalive(t *testing.T) {
	c := newTestConfig(t)
	c.KeepaliveInterval = time.Second
	s := newTestServer(t, c)

	rr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", "/mytopic/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	doneChan := make(chan bool)
	go func() {
		s.handle(rr, req)
		doneChan <- true
	}()
	time.Sleep(1300 * time.Millisecond)
	cancel()
	<-doneChan

	messages := toMessages(t, rr.Body.String())
	assert.Equal(t, 2, len(messages))

	assert.Equal(t, openEvent, messages[0].Event)
	assert.Equal(t, "mytopic", messages[0].Topic)
	assert.Equal(t, "", messages[0].Message)
	assert.Equal(t, "", messages[0].Title)
	assert.Equal(t, 0, messages[0].Priority)
	assert.Nil(t, messages[0].Tags)

	assert.Equal(t, keepaliveEvent, messages[1].Event)
	assert.Equal(t, "mytopic", messages[1].Topic)
	assert.Equal(t, "", messages[1].Message)
	assert.Equal(t, "", messages[1].Title)
	assert.Equal(t, 0, messages[1].Priority)
	assert.Nil(t, messages[1].Tags)
}

func TestServer_PublishAndSubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/mytopic/json", subscribeRR)

	publishFirstRR := request(t, s, "PUT", "/mytopic", "my first message", nil)
	assert.Equal(t, 200, publishFirstRR.Code)

	publishSecondRR := request(t, s, "PUT", "/mytopic", "my other message", map[string]string{
		"Title":  " This is a title ",
		"X-Tags": "tag1,tag 2, tag3",
		"p":      "1",
	})
	assert.Equal(t, 200, publishSecondRR.Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	assert.Equal(t, 3, len(messages))
	assert.Equal(t, openEvent, messages[0].Event)

	assert.Equal(t, messageEvent, messages[1].Event)
	assert.Equal(t, "mytopic", messages[1].Topic)
	assert.Equal(t, "my first message", messages[1].Message)
	assert.Equal(t, "", messages[1].Title)
	assert.Equal(t, 0, messages[1].Priority)
	assert.Nil(t, messages[1].Tags)

	assert.Equal(t, messageEvent, messages[2].Event)
	assert.Equal(t, "mytopic", messages[2].Topic)
	assert.Equal(t, "my other message", messages[2].Message)
	assert.Equal(t, "This is a title", messages[2].Title)
	assert.Equal(t, 1, messages[2].Priority)
	assert.Equal(t, []string{"tag1", "tag 2", "tag3"}, messages[2].Tags)
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

func request(t *testing.T, s *Server, method, url, body string, headers map[string]string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	s.handle(rr, req)
	return rr
}

func subscribe(t *testing.T, s *Server, url string, rr *httptest.ResponseRecorder) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool)
	go func() {
		s.handle(rr, req)
		done <- true
	}()
	cancelAndWaitForDone := func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
		<-done
	}
	time.Sleep(100 * time.Millisecond)
	return cancelAndWaitForDone
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
