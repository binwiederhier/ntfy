package server

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
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
	require.NotEmpty(t, msg1.ID)
	require.Equal(t, "my first message", msg1.Message)

	response2 := request(t, s, "PUT", "/mytopic", "my second\n\nmessage", nil)
	msg2 := toMessage(t, response2.Body.String())
	require.NotEqual(t, msg1.ID, msg2.ID)
	require.NotEmpty(t, msg2.ID)
	require.Equal(t, "my second\n\nmessage", msg2.Message)

	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "my first message", messages[0].Message)
	require.Equal(t, "my second\n\nmessage", messages[1].Message)

	response = request(t, s, "GET", "/mytopic/sse?poll=1", "", nil)
	lines := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	require.Equal(t, 3, len(lines))
	require.Equal(t, "my first message", toMessage(t, strings.TrimPrefix(lines[0], "data: ")).Message)
	require.Equal(t, "", lines[1])
	require.Equal(t, "my second\n\nmessage", toMessage(t, strings.TrimPrefix(lines[2], "data: ")).Message)

	response = request(t, s, "GET", "/mytopic/raw?poll=1", "", nil)
	lines = strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	require.Equal(t, 2, len(lines))
	require.Equal(t, "my first message", lines[0])
	require.Equal(t, "my second  message", lines[1]) // \n -> " "
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
	require.Equal(t, 2, len(messages))

	require.Equal(t, openEvent, messages[0].Event)
	require.Equal(t, "mytopic", messages[0].Topic)
	require.Equal(t, "", messages[0].Message)
	require.Equal(t, "", messages[0].Title)
	require.Equal(t, 0, messages[0].Priority)
	require.Nil(t, messages[0].Tags)

	require.Equal(t, keepaliveEvent, messages[1].Event)
	require.Equal(t, "mytopic", messages[1].Topic)
	require.Equal(t, "", messages[1].Message)
	require.Equal(t, "", messages[1].Title)
	require.Equal(t, 0, messages[1].Priority)
	require.Nil(t, messages[1].Tags)
}

func TestServer_PublishAndSubscribe(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/mytopic/json", subscribeRR)

	publishFirstRR := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, 200, publishFirstRR.Code)

	publishSecondRR := request(t, s, "PUT", "/mytopic", "my other message", map[string]string{
		"Title":  " This is a title ",
		"X-Tags": "tag1,tag 2, tag3",
		"p":      "1",
	})
	require.Equal(t, 200, publishSecondRR.Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 3, len(messages))
	require.Equal(t, openEvent, messages[0].Event)

	require.Equal(t, messageEvent, messages[1].Event)
	require.Equal(t, "mytopic", messages[1].Topic)
	require.Equal(t, "my first message", messages[1].Message)
	require.Equal(t, "", messages[1].Title)
	require.Equal(t, 0, messages[1].Priority)
	require.Nil(t, messages[1].Tags)

	require.Equal(t, messageEvent, messages[2].Event)
	require.Equal(t, "mytopic", messages[2].Topic)
	require.Equal(t, "my other message", messages[2].Message)
	require.Equal(t, "This is a title", messages[2].Title)
	require.Equal(t, 1, messages[2].Priority)
	require.Equal(t, []string{"tag1", "tag 2", "tag3"}, messages[2].Tags)
}

func TestServer_StaticSites(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	rr := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), "</html>")

	rr = request(t, s, "HEAD", "/", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/does-not-exist.txt", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/mytopic", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `<meta name="robots" content="noindex, nofollow" />`)

	rr = request(t, s, "GET", "/static/css/app.css", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `html, body {`)

	rr = request(t, s, "GET", "/docs", "", nil)
	require.Equal(t, 301, rr.Code)

	rr = request(t, s, "GET", "/docs/", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `Made with ❤️ by Philipp C. Heckel`)
	require.Contains(t, rr.Body.String(), `<script src=static/js/extra.js></script>`)
}

func TestServer_PublishLargeMessage(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	body := strings.Repeat("this is a large message", 1000)
	truncated := body[0:512]
	response := request(t, s, "PUT", "/mytopic", body, nil)
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, truncated, msg.Message)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, truncated, messages[0].Message)
}

func TestServer_PublishNoCache(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "this message is not cached", map[string]string{
		"Cache": "no",
	})
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "this message is not cached", msg.Message)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Empty(t, messages)
}
func TestServer_PublishAt(t *testing.T) {
	c := newTestConfig(t)
	c.MinDelay = time.Second
	c.AtSenderInterval = 100 * time.Millisecond
	s := newTestServer(t, c)

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1s",
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 0, len(messages))

	time.Sleep(time.Second)
	require.Nil(t, s.sendDelayedMessages())

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "a message", messages[0].Message)
}

func TestServer_PublishAtWithCacheError(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"Cache": "no",
		"In":    "30 min",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishAtTooShortDelay(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1s",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishAtTooLongDelay(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "99999999h",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishAtAndPrune(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1h",
	})
	require.Equal(t, 200, response.Code)
	s.updateStatsAndPrune() // Fire pruning

	response = request(t, s, "GET", "/mytopic/json?poll=1&scheduled=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages)) // Not affected by pruning
	require.Equal(t, "a message", messages[0].Message)
}

func TestServer_PublishAndMultiPoll(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic1", "message 1", nil)
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "mytopic1", msg.Topic)
	require.Equal(t, "message 1", msg.Message)

	response = request(t, s, "PUT", "/mytopic2", "message 2", nil)
	msg = toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "mytopic2", msg.Topic)
	require.Equal(t, "message 2", msg.Message)

	response = request(t, s, "GET", "/mytopic1/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "mytopic1", messages[0].Topic)
	require.Equal(t, "message 1", messages[0].Message)

	response = request(t, s, "GET", "/mytopic1,mytopic2/json?poll=1", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "mytopic1", messages[0].Topic)
	require.Equal(t, "message 1", messages[0].Message)
	require.Equal(t, "mytopic2", messages[1].Topic)
	require.Equal(t, "message 2", messages[1].Message)
}

func TestServer_PublishWithNopCache(t *testing.T) {
	c := newTestConfig(t)
	c.CacheDuration = 0
	s := newTestServer(t, c)

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/mytopic/json", subscribeRR)

	publishRR := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, 200, publishRR.Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, openEvent, messages[0].Event)
	require.Equal(t, messageEvent, messages[1].Event)
	require.Equal(t, "my first message", messages[1].Message)

	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Empty(t, messages)
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
	for k, v := range headers {
		req.Header.Set(k, v)
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
	require.Nil(t, json.NewDecoder(strings.NewReader(s)).Decode(&m))
	return &m
}
