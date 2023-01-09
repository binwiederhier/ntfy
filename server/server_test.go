package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/user"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/util"
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

	response = request(t, s, "GET", "/mytopic/sse?poll=1&since=all", "", nil)
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

func TestServer_PublishWithFirebase(t *testing.T) {
	sender := newTestFirebaseSender(10)
	s := newTestServer(t, newTestConfig(t))
	s.firebaseClient = newFirebaseClient(sender, &testAuther{Allow: true})

	response := request(t, s, "PUT", "/mytopic", "my first message", nil)
	msg1 := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg1.ID)
	require.Equal(t, "my first message", msg1.Message)

	time.Sleep(100 * time.Millisecond) // Firebase publishing happens
	require.Equal(t, 1, len(sender.Messages()))
	require.Equal(t, "my first message", sender.Messages()[0].Data["message"])
	require.Equal(t, "my first message", sender.Messages()[0].APNS.Payload.Aps.Alert.Body)
	require.Equal(t, "my first message", sender.Messages()[0].APNS.Payload.CustomData["message"])
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

	rr = request(t, s, "OPTIONS", "/", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/does-not-exist.txt", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/mytopic", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `<meta name="robots" content="noindex, nofollow"/>`)

	rr = request(t, s, "GET", "/static/css/home.css", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Contains(t, rr.Body.String(), `/* general styling */`)

	rr = request(t, s, "GET", "/docs", "", nil)
	require.Equal(t, 301, rr.Code)

	// Docs test removed, it was failing annoyingly.
}

func TestServer_WebEnabled(t *testing.T) {
	conf := newTestConfig(t)
	conf.EnableWeb = false
	s := newTestServer(t, conf)

	rr := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/config.js", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/static/css/home.css", "", nil)
	require.Equal(t, 404, rr.Code)

	conf2 := newTestConfig(t)
	conf2.EnableWeb = true
	s2 := newTestServer(t, conf2)

	rr = request(t, s2, "GET", "/", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s2, "GET", "/config.js", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s2, "GET", "/static/css/home.css", "", nil)
	require.Equal(t, 200, rr.Code)
}

func TestServer_PublishLargeMessage(t *testing.T) {
	c := newTestConfig(t)
	c.AttachmentCacheDir = "" // Disable attachments
	s := newTestServer(t, c)

	body := strings.Repeat("this is a large message", 5000)
	response := request(t, s, "PUT", "/mytopic", body, nil)
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishPriority(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	for prio := 1; prio <= 5; prio++ {
		response := request(t, s, "GET", fmt.Sprintf("/mytopic/publish?priority=%d", prio), fmt.Sprintf("priority %d", prio), nil)
		msg := toMessage(t, response.Body.String())
		require.Equal(t, prio, msg.Priority)
	}

	response := request(t, s, "GET", "/mytopic/publish?priority=min", "test", nil)
	require.Equal(t, 1, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=low", "test", nil)
	require.Equal(t, 2, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=default", "test", nil)
	require.Equal(t, 3, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=high", "test", nil)
	require.Equal(t, 4, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=max", "test", nil)
	require.Equal(t, 5, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/trigger?priority=urgent", "test", nil)
	require.Equal(t, 5, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/trigger?priority=INVALID", "test", nil)
	require.Equal(t, 40007, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PublishGETOnlyOneTopic(t *testing.T) {
	// This tests a bug that allowed publishing topics with a comma in the name (no ticket)

	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "GET", "/mytopic,mytopic2/publish?m=hi", "", nil)
	require.Equal(t, 404, response.Code)
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
	c.DelayedSenderInterval = 100 * time.Millisecond
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
	require.Equal(t, netip.Addr{}, messages[0].Sender) // Never return the sender!

	messages, err := s.messageCache.Messages("mytopic", sinceAllMessages, true)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "a message", messages[0].Message)
	require.Equal(t, "9.9.9.9", messages[0].Sender.String()) // It's stored in the DB though!
}

func TestServer_PublishAtWithCacheError(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"Cache": "no",
		"In":    "30 min",
	})
	require.Equal(t, 400, response.Code)
	require.Equal(t, errHTTPBadRequestDelayNoCache, toHTTPError(t, response.Body.String()))
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

func TestServer_PublishAtInvalidDelay(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?delay=INVALID", "a message", nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 400, response.Code)
	require.Equal(t, 40004, err.Code)
}

func TestServer_PublishAtTooLarge(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?x-in=99999h", "a message", nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 400, response.Code)
	require.Equal(t, 40006, err.Code)
}

func TestServer_PublishAtAndPrune(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1h",
	})
	require.Equal(t, 200, response.Code)
	s.execManager() // Fire pruning

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

func TestServer_PublishAndPollSince(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	request(t, s, "PUT", "/mytopic", "test 1", nil)
	time.Sleep(1100 * time.Millisecond)

	since := time.Now().Unix()
	request(t, s, "PUT", "/mytopic", "test 2", nil)

	response := request(t, s, "GET", fmt.Sprintf("/mytopic/json?poll=1&since=%d", since), "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "test 2", messages[0].Message)

	response = request(t, s, "GET", "/mytopic/json?poll=1&since=10s", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "test 1", messages[0].Message)

	response = request(t, s, "GET", "/mytopic/json?poll=1&since=100ms", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "test 2", messages[0].Message)

	response = request(t, s, "GET", "/mytopic/json?poll=1&since=INVALID", "", nil)
	require.Equal(t, 40008, toHTTPError(t, response.Body.String()).Code)
}

func newMessageWithTimestamp(topic, message string, timestamp int64) *message {
	m := newDefaultMessage(topic, message)
	m.Time = timestamp
	return m
}

func TestServer_PollSinceID_MultipleTopics(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic1", "test 1", 1655740277)))
	markerMessage := newMessageWithTimestamp("mytopic2", "test 2", 1655740283)
	require.Nil(t, s.messageCache.AddMessage(markerMessage))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic1", "test 3", 1655740289)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic2", "test 4", 1655740293)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic1", "test 5", 1655740297)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic2", "test 6", 1655740303)))

	response := request(t, s, "GET", fmt.Sprintf("/mytopic1,mytopic2/json?poll=1&since=%s", markerMessage.ID), "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 4, len(messages))
	require.Equal(t, "test 3", messages[0].Message)
	require.Equal(t, "mytopic1", messages[0].Topic)
	require.Equal(t, "test 4", messages[1].Message)
	require.Equal(t, "mytopic2", messages[1].Topic)
	require.Equal(t, "test 5", messages[2].Message)
	require.Equal(t, "mytopic1", messages[2].Topic)
	require.Equal(t, "test 6", messages[3].Message)
	require.Equal(t, "mytopic2", messages[3].Topic)
}

func TestServer_PollSinceID_MultipleTopics_IDDoesNotMatch(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic1", "test 3", 1655740289)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic2", "test 4", 1655740293)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic1", "test 5", 1655740297)))
	require.Nil(t, s.messageCache.AddMessage(newMessageWithTimestamp("mytopic2", "test 6", 1655740303)))

	response := request(t, s, "GET", "/mytopic1,mytopic2/json?poll=1&since=NoMatchForID", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 4, len(messages))
	require.Equal(t, "test 3", messages[0].Message)
	require.Equal(t, "test 4", messages[1].Message)
	require.Equal(t, "test 5", messages[2].Message)
	require.Equal(t, "test 6", messages[3].Message)
}

func TestServer_PublishViaGET(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "GET", "/mytopic/trigger", "", nil)
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "triggered", msg.Message)

	response = request(t, s, "GET", "/mytopic/send?message=This+is+a+test&t=This+is+a+title&tags=skull&x-priority=5&delay=24h", "", nil)
	msg = toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "This is a test", msg.Message)
	require.Equal(t, "This is a title", msg.Title)
	require.Equal(t, []string{"skull"}, msg.Tags)
	require.Equal(t, 5, msg.Priority)
	require.Greater(t, msg.Time, time.Now().Add(23*time.Hour).Unix())
}

func TestServer_PublishMessageInHeaderWithNewlines(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "", map[string]string{
		"Message": "Line 1\\nLine 2",
	})
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "Line 1\nLine 2", msg.Message) // \\n -> \n !
}

func TestServer_PublishInvalidTopic(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	s.smtpSender = &testMailer{}
	response := request(t, s, "PUT", "/docs", "fail", nil)
	require.Equal(t, 40010, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PollWithQueryFilters(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic?priority=1&tags=tag1,tag2", "my first message", nil)
	msg := toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)

	response = request(t, s, "PUT", "/mytopic?title=a+title", "my second message", map[string]string{
		"Tags": "tag2,tag3",
	})
	msg = toMessage(t, response.Body.String())
	require.NotEmpty(t, msg.ID)

	queriesThatShouldReturnMessageOne := []string{
		"/mytopic/json?poll=1&priority=1",
		"/mytopic/json?poll=1&priority=min",
		"/mytopic/json?poll=1&priority=min,low",
		"/mytopic/json?poll=1&priority=1,2",
		"/mytopic/json?poll=1&p=2,min",
		"/mytopic/json?poll=1&tags=tag1",
		"/mytopic/json?poll=1&tags=tag1,tag2",
		"/mytopic/json?poll=1&message=my+first+message",
	}
	for _, query := range queriesThatShouldReturnMessageOne {
		response = request(t, s, "GET", query, "", nil)
		messages := toMessages(t, response.Body.String())
		require.Equal(t, 1, len(messages), "Query failed: "+query)
		require.Equal(t, "my first message", messages[0].Message, "Query failed: "+query)
	}

	queriesThatShouldReturnMessageTwo := []string{
		"/mytopic/json?poll=1&x-priority=3", // !
		"/mytopic/json?poll=1&priority=3",
		"/mytopic/json?poll=1&priority=default",
		"/mytopic/json?poll=1&p=3",
		"/mytopic/json?poll=1&x-tags=tag2,tag3",
		"/mytopic/json?poll=1&tags=tag2,tag3",
		"/mytopic/json?poll=1&tag=tag2,tag3",
		"/mytopic/json?poll=1&ta=tag2,tag3",
		"/mytopic/json?poll=1&x-title=a+title",
		"/mytopic/json?poll=1&title=a+title",
		"/mytopic/json?poll=1&t=a+title",
		"/mytopic/json?poll=1&x-message=my+second+message",
		"/mytopic/json?poll=1&message=my+second+message",
		"/mytopic/json?poll=1&m=my+second+message",
		"/mytopic/json?x-poll=1&m=my+second+message",
		"/mytopic/json?po=1&m=my+second+message",
	}
	for _, query := range queriesThatShouldReturnMessageTwo {
		response = request(t, s, "GET", query, "", nil)
		messages := toMessages(t, response.Body.String())
		require.Equal(t, 1, len(messages), "Query failed: "+query)
		require.Equal(t, "my second message", messages[0].Message, "Query failed: "+query)
	}

	queriesThatShouldReturnNoMessages := []string{
		"/mytopic/json?poll=1&priority=4",
		"/mytopic/json?poll=1&tags=tag1,tag2,tag3",
		"/mytopic/json?poll=1&title=another+title",
		"/mytopic/json?poll=1&message=my+third+message",
		"/mytopic/json?poll=1&message=my+third+message",
	}
	for _, query := range queriesThatShouldReturnNoMessages {
		response = request(t, s, "GET", query, "", nil)
		messages := toMessages(t, response.Body.String())
		require.Equal(t, 0, len(messages), "Query failed: "+query)
	}
}

func TestServer_SubscribeWithQueryFilters(t *testing.T) {
	c := newTestConfig(t)
	c.KeepaliveInterval = 800 * time.Millisecond
	s := newTestServer(t, c)

	subscribeResponse := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/mytopic/json?tags=zfs-issue", subscribeResponse)

	response := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, 200, response.Code)
	response = request(t, s, "PUT", "/mytopic", "ZFS scrub failed", map[string]string{
		"Tags": "zfs-issue,zfs-scrub",
	})
	require.Equal(t, 200, response.Code)

	time.Sleep(850 * time.Millisecond)
	subscribeCancel()

	messages := toMessages(t, subscribeResponse.Body.String())
	require.Equal(t, 3, len(messages))
	require.Equal(t, openEvent, messages[0].Event)
	require.Equal(t, messageEvent, messages[1].Event)
	require.Equal(t, "ZFS scrub failed", messages[1].Message)
	require.Equal(t, keepaliveEvent, messages[2].Event)
}

func TestServer_Auth_Success_Admin(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": basicAuth("phil:phil"),
	})
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())
}

func TestServer_Auth_Success_User(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("", "ben", "mytopic", true, true))

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": basicAuth("ben:ben"),
	})
	require.Equal(t, 200, response.Code)
}

func TestServer_Auth_Success_User_MultipleTopics(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("", "ben", "mytopic", true, true))
	require.Nil(t, s.userManager.AllowAccess("", "ben", "anothertopic", true, true))

	response := request(t, s, "GET", "/mytopic,anothertopic/auth", "", map[string]string{
		"Authorization": basicAuth("ben:ben"),
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic,anothertopic,NOT-THIS-ONE/auth", "", map[string]string{
		"Authorization": basicAuth("ben:ben"),
	})
	require.Equal(t, 403, response.Code)
}

func TestServer_Auth_Fail_InvalidPass(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": basicAuth("phil:INVALID"),
	})
	require.Equal(t, 401, response.Code)
}

func TestServer_Auth_Fail_Unauthorized(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("", "ben", "sometopic", true, true)) // Not mytopic!

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": basicAuth("ben:ben"),
	})
	require.Equal(t, 403, response.Code)
}

func TestServer_Auth_Fail_CannotPublish(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionReadWrite // Open by default
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, s.userManager.AllowAccess("", user.Everyone, "private", false, false))
	require.Nil(t, s.userManager.AllowAccess("", user.Everyone, "announcements", true, false))

	response := request(t, s, "PUT", "/mytopic", "test", nil)
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	response = request(t, s, "PUT", "/announcements", "test", nil)
	require.Equal(t, 403, response.Code) // Cannot write as anonymous

	response = request(t, s, "PUT", "/announcements", "test", map[string]string{
		"Authorization": basicAuth("phil:phil"),
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/announcements/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code) // Anonymous read allowed

	response = request(t, s, "GET", "/private/json?poll=1", "", nil)
	require.Equal(t, 403, response.Code) // Anonymous read not allowed
}

func TestServer_Auth_ViaQuery(t *testing.T) {
	c := newTestConfig(t)
	c.AuthFile = filepath.Join(t.TempDir(), "user.db")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "some pass", user.RoleAdmin))

	u := fmt.Sprintf("/mytopic/json?poll=1&auth=%s", base64.RawURLEncoding.EncodeToString([]byte(basicAuth("ben:some pass"))))
	response := request(t, s, "GET", u, "", nil)
	require.Equal(t, 200, response.Code)

	u = fmt.Sprintf("/mytopic/json?poll=1&auth=%s", base64.RawURLEncoding.EncodeToString([]byte(basicAuth("ben:WRONNNGGGG"))))
	response = request(t, s, "GET", u, "", nil)
	require.Equal(t, 401, response.Code)
}

type testMailer struct {
	count int
	mu    sync.Mutex
}

func (t *testMailer) Send(v *visitor, m *message, to string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.count++
	return nil
}

func (t *testMailer) Counts() (total int64, success int64, failure int64) {
	return 0, 0, 0
}

func (t *testMailer) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func TestServer_PublishTooRequests_Defaults(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	for i := 0; i < 60; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "message", nil)
	require.Equal(t, 429, response.Code)
}

func TestServer_PublishTooRequests_Defaults_ExemptHosts(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorRequestExemptIPAddrs = []netip.Prefix{netip.MustParsePrefix("9.9.9.9/32")} // see request()
	s := newTestServer(t, c)
	for i := 0; i < 65; i++ { // > 60
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
		require.Equal(t, 200, response.Code)
	}
}

func TestServer_PublishTooRequests_ShortReplenish(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorRequestLimitBurst = 60
	c.VisitorRequestLimitReplenish = 500 * time.Millisecond
	s := newTestServer(t, c)
	for i := 0; i < 60; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "message", nil)
	require.Equal(t, 429, response.Code)

	time.Sleep(520 * time.Millisecond)
	response = request(t, s, "PUT", "/mytopic", "message", nil)
	require.Equal(t, 200, response.Code)
}

func TestServer_PublishTooManyEmails_Defaults(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	s.smtpSender = &testMailer{}
	for i := 0; i < 16; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), map[string]string{
			"E-Mail": "test@example.com",
		})
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "one too many", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 429, response.Code)
}

func TestServer_PublishTooManyEmails_Replenish(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorEmailLimitReplenish = 500 * time.Millisecond
	s := newTestServer(t, c)
	s.smtpSender = &testMailer{}
	for i := 0; i < 16; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), map[string]string{
			"E-Mail": "test@example.com",
		})
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "one too many", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 429, response.Code)

	time.Sleep(510 * time.Millisecond)
	response = request(t, s, "PUT", "/mytopic", "this should be okay again too many", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "PUT", "/mytopic", "and bad again", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 429, response.Code)
}

func TestServer_PublishDelayedEmail_Fail(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	s.smtpSender = &testMailer{}
	response := request(t, s, "PUT", "/mytopic", "fail", map[string]string{
		"E-Mail": "test@example.com",
		"Delay":  "20 min",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishEmailNoMailer_Fail(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "fail", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_UnifiedPushDiscovery(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "GET", "/mytopic?up=1", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"unifiedpush":{"version":1}}`+"\n", response.Body.String())
}

func TestServer_PublishUnifiedPushBinary_AndPoll(t *testing.T) {
	b := make([]byte, 12) // Max length
	_, err := rand.Read(b)
	require.Nil(t, err)

	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?up=1", string(b), nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "base64", m.Encoding)
	b2, err := base64.StdEncoding.DecodeString(m.Message)
	require.Nil(t, err)
	require.Equal(t, b, b2)

	response = request(t, s, "GET", "/mytopic/json?poll=1", string(b), nil)
	require.Equal(t, 200, response.Code)
	m = toMessage(t, response.Body.String())
	require.Equal(t, "base64", m.Encoding)
	b2, err = base64.StdEncoding.DecodeString(m.Message)
	require.Nil(t, err)
	require.Equal(t, b, b2)
}

func TestServer_PublishUnifiedPushBinary_Truncated(t *testing.T) {
	b := make([]byte, 5000) // Longer than max length
	_, err := rand.Read(b)
	require.Nil(t, err)

	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?up=1", string(b), nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "base64", m.Encoding)
	b2, err := base64.StdEncoding.DecodeString(m.Message)
	require.Nil(t, err)
	require.Equal(t, 4096, len(b2))
	require.Equal(t, b[:4096], b2)
}

func TestServer_PublishUnifiedPushText(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?up=1", "this is a unifiedpush text message", nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "", m.Encoding)
	require.Equal(t, "this is a unifiedpush text message", m.Message)
}

func TestServer_MatrixGateway_Discovery_Success(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "GET", "/_matrix/push/v1/notify", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"unifiedpush":{"gateway":"matrix"}}`+"\n", response.Body.String())
}

func TestServer_MatrixGateway_Discovery_Failure_Unconfigured(t *testing.T) {
	c := newTestConfig(t)
	c.BaseURL = ""
	s := newTestServer(t, c)
	response := request(t, s, "GET", "/_matrix/push/v1/notify", "", nil)
	require.Equal(t, 500, response.Code)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 50003, err.Code)
}

func TestServer_MatrixGateway_Push_Success(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":[]}`+"\n", response.Body.String())

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, notification, m.Message)
}

func TestServer_MatrixGateway_Push_Failure_InvalidPushkey(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	notification := `{"notification":{"devices":[{"pushkey":"http://wrong-base-url.com/mytopic?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":["http://wrong-base-url.com/mytopic?up=1"]}`+"\n", response.Body.String())

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, "", response.Body.String()) // Empty!
}

func TestServer_MatrixGateway_Push_Failure_EverythingIsWrong(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	notification := `{"message":"this is not really a Matrix message"}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 400, response.Code)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 40019, err.Code)
	require.Equal(t, 400, err.HTTPCode)
}

func TestServer_MatrixGateway_Push_Failure_Unconfigured(t *testing.T) {
	c := newTestConfig(t)
	c.BaseURL = ""
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 500, response.Code)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 50003, err.Code)
	require.Equal(t, 500, err.HTTPCode)
}

func TestServer_PublishActions_AndPoll(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "my message", map[string]string{
		"Actions": "view, Open portal, https://home.nest.com/; http, Turn down, https://api.nest.com/device/XZ1D2, body=target_temp_f=65",
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, 2, len(m.Actions))
	require.Equal(t, "view", m.Actions[0].Action)
	require.Equal(t, "Open portal", m.Actions[0].Label)
	require.Equal(t, "https://home.nest.com/", m.Actions[0].URL)
	require.Equal(t, "http", m.Actions[1].Action)
	require.Equal(t, "Turn down", m.Actions[1].Label)
	require.Equal(t, "https://api.nest.com/device/XZ1D2", m.Actions[1].URL)
	require.Equal(t, "target_temp_f=65", m.Actions[1].Body)
}

func TestServer_PublishAsJSON(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	body := `{"topic":"mytopic","message":"A message","title":"a title\nwith lines","tags":["tag1","tag 2"],` +
		`"not-a-thing":"ok", "attach":"http://google.com","filename":"google.pdf", "click":"http://ntfy.sh","priority":4,` +
		`"icon":"https://ntfy.sh/static/img/ntfy.png", "delay":"30min"}`
	response := request(t, s, "PUT", "/", body, nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "mytopic", m.Topic)
	require.Equal(t, "A message", m.Message)
	require.Equal(t, "a title\nwith lines", m.Title)
	require.Equal(t, []string{"tag1", "tag 2"}, m.Tags)
	require.Equal(t, "http://google.com", m.Attachment.URL)
	require.Equal(t, "google.pdf", m.Attachment.Name)
	require.Equal(t, "http://ntfy.sh", m.Click)
	require.Equal(t, "https://ntfy.sh/static/img/ntfy.png", m.Icon)

	require.Equal(t, 4, m.Priority)
	require.True(t, m.Time > time.Now().Unix()+29*60)
	require.True(t, m.Time < time.Now().Unix()+31*60)
}

func TestServer_PublishAsJSON_WithEmail(t *testing.T) {
	mailer := &testMailer{}
	s := newTestServer(t, newTestConfig(t))
	s.smtpSender = mailer
	body := `{"topic":"mytopic","message":"A message","email":"phil@example.com"}`
	response := request(t, s, "PUT", "/", body, nil)
	require.Equal(t, 200, response.Code)
	time.Sleep(100 * time.Millisecond) // E-Mail publishing happens in a Go routine

	m := toMessage(t, response.Body.String())
	require.Equal(t, "mytopic", m.Topic)
	require.Equal(t, "A message", m.Message)
	require.Equal(t, 1, mailer.Count())
}

func TestServer_PublishAsJSON_WithActions(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	body := `{
		"topic":"mytopic",
		"message":"A message",
		"actions": [
			  {
				"action": "view",
				"label": "Open portal",
				"url": "https://home.nest.com/"
			  },
			  {
				"action": "http",
				"label": "Turn down",
				"url": "https://api.nest.com/device/XZ1D2",
				"body": "target_temp_f=65"
			  }
		]
	}`
	response := request(t, s, "POST", "/", body, nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "mytopic", m.Topic)
	require.Equal(t, "A message", m.Message)
	require.Equal(t, 2, len(m.Actions))
	require.Equal(t, "view", m.Actions[0].Action)
	require.Equal(t, "Open portal", m.Actions[0].Label)
	require.Equal(t, "https://home.nest.com/", m.Actions[0].URL)
	require.Equal(t, "http", m.Actions[1].Action)
	require.Equal(t, "Turn down", m.Actions[1].Label)
	require.Equal(t, "https://api.nest.com/device/XZ1D2", m.Actions[1].URL)
	require.Equal(t, "target_temp_f=65", m.Actions[1].Body)
}

func TestServer_PublishAsJSON_Invalid(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	body := `{"topic":"mytopic",INVALID`
	response := request(t, s, "PUT", "/", body, nil)
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishWithTierBasedMessageLimitAndExpiry(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	s := newTestServer(t, c)

	// Create tier with certain limits
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		Code:                   "test",
		MessagesLimit:          5,
		MessagesExpiryDuration: -5 * time.Second, // Second, what a hack!
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	// Publish to reach message limit
	for i := 0; i < 5; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("this is message %d", i+1), map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
		msg := toMessage(t, response.Body.String())
		require.True(t, msg.Expires < time.Now().Unix()+5)
	}
	response := request(t, s, "PUT", "/mytopic", "this is too much", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 429, response.Code)

	// Run pruning and see if they are gone
	s.execManager()
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	require.Empty(t, response.Body)
}

func TestServer_PublishAttachment(t *testing.T) {
	content := util.RandomString(5000) // > 4096
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", content, nil)
	msg := toMessage(t, response.Body.String())
	require.Equal(t, "attachment.txt", msg.Attachment.Name)
	require.Equal(t, "text/plain; charset=utf-8", msg.Attachment.Type)
	require.Equal(t, int64(5000), msg.Attachment.Size)
	require.GreaterOrEqual(t, msg.Attachment.Expires, time.Now().Add(179*time.Minute).Unix()) // Almost 3 hours
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	require.Equal(t, netip.Addr{}, msg.Sender) // Should never be returned
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, msg.ID))

	// GET
	path := strings.TrimPrefix(msg.Attachment.URL, "http://127.0.0.1:12345")
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, "5000", response.Header().Get("Content-Length"))
	require.Equal(t, content, response.Body.String())

	// HEAD
	response = request(t, s, "HEAD", path, "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, "5000", response.Header().Get("Content-Length"))
	require.Equal(t, "", response.Body.String())

	// Slightly unrelated cross-test: make sure we add an owner for internal attachments
	size, err := s.messageCache.AttachmentBytesUsedBySender("9.9.9.9") // See request()
	require.Nil(t, err)
	require.Equal(t, int64(5000), size)
}

func TestServer_PublishAttachmentShortWithFilename(t *testing.T) {
	c := newTestConfig(t)
	c.BehindProxy = true
	s := newTestServer(t, c)
	content := "this is an ATTACHMENT"
	response := request(t, s, "PUT", "/mytopic?f=myfile.txt", content, map[string]string{
		"X-Forwarded-For": "1.2.3.4",
	})
	msg := toMessage(t, response.Body.String())
	require.Equal(t, "myfile.txt", msg.Attachment.Name)
	require.Equal(t, "text/plain; charset=utf-8", msg.Attachment.Type)
	require.Equal(t, int64(21), msg.Attachment.Size)
	require.GreaterOrEqual(t, msg.Attachment.Expires, time.Now().Add(3*time.Hour).Unix())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	require.Equal(t, netip.Addr{}, msg.Sender) // Should never be returned
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, msg.ID))

	path := strings.TrimPrefix(msg.Attachment.URL, "http://127.0.0.1:12345")
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, "21", response.Header().Get("Content-Length"))
	require.Equal(t, content, response.Body.String())

	// Slightly unrelated cross-test: make sure we add an owner for internal attachments
	size, err := s.messageCache.AttachmentBytesUsedBySender("1.2.3.4")
	require.Nil(t, err)
	require.Equal(t, int64(21), size)
}

func TestServer_PublishAttachmentExternalWithoutFilename(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "", map[string]string{
		"Attach": "https://upload.wikimedia.org/wikipedia/commons/f/fd/Pink_flower.jpg",
	})
	msg := toMessage(t, response.Body.String())
	require.Equal(t, "You received a file: Pink_flower.jpg", msg.Message)
	require.Equal(t, "Pink_flower.jpg", msg.Attachment.Name)
	require.Equal(t, "https://upload.wikimedia.org/wikipedia/commons/f/fd/Pink_flower.jpg", msg.Attachment.URL)
	require.Equal(t, "", msg.Attachment.Type)
	require.Equal(t, int64(0), msg.Attachment.Size)
	require.Equal(t, int64(0), msg.Attachment.Expires)
	require.Equal(t, netip.Addr{}, msg.Sender)

	// Slightly unrelated cross-test: make sure we don't add an owner for external attachments
	size, err := s.messageCache.AttachmentBytesUsedBySender("127.0.0.1")
	require.Nil(t, err)
	require.Equal(t, int64(0), size)
}

func TestServer_PublishAttachmentExternalWithFilename(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "This is a custom message", map[string]string{
		"X-Attach": "https://upload.wikimedia.org/wikipedia/commons/f/fd/Pink_flower.jpg",
		"File":     "some file.jpg",
	})
	msg := toMessage(t, response.Body.String())
	require.Equal(t, "This is a custom message", msg.Message)
	require.Equal(t, "some file.jpg", msg.Attachment.Name)
	require.Equal(t, "https://upload.wikimedia.org/wikipedia/commons/f/fd/Pink_flower.jpg", msg.Attachment.URL)
	require.Equal(t, "", msg.Attachment.Type)
	require.Equal(t, int64(0), msg.Attachment.Size)
	require.Equal(t, int64(0), msg.Attachment.Expires)
	require.Equal(t, netip.Addr{}, msg.Sender)
}

func TestServer_PublishAttachmentBadURL(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?a=not+a+URL", "", nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 400, response.Code)
	require.Equal(t, 400, err.HTTPCode)
	require.Equal(t, 40013, err.Code)
}

func TestServer_PublishAttachmentTooLargeContentLength(t *testing.T) {
	content := util.RandomString(5000) // > 4096
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", content, map[string]string{
		"Content-Length": "20000000",
	})
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 413, response.Code)
	require.Equal(t, 413, err.HTTPCode)
	require.Equal(t, 41301, err.Code)
}

func TestServer_PublishAttachmentTooLargeBodyAttachmentFileSizeLimit(t *testing.T) {
	content := util.RandomString(5001) // > 5000, see below
	c := newTestConfig(t)
	c.AttachmentFileSizeLimit = 5000
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic", content, nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 413, response.Code)
	require.Equal(t, 413, err.HTTPCode)
	require.Equal(t, 41301, err.Code)
}

func TestServer_PublishAttachmentExpiryBeforeDelivery(t *testing.T) {
	c := newTestConfig(t)
	c.AttachmentExpiryDuration = 10 * time.Minute
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic", util.RandomString(5000), map[string]string{
		"Delay": "11 min", // > AttachmentExpiryDuration
	})
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 400, response.Code)
	require.Equal(t, 400, err.HTTPCode)
	require.Equal(t, 40015, err.Code)
}

func TestServer_PublishAttachmentTooLargeBodyVisitorAttachmentTotalSizeLimit(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorAttachmentTotalSizeLimit = 10000
	s := newTestServer(t, c)

	response := request(t, s, "PUT", "/mytopic", util.RandomString(5000), nil)
	msg := toMessage(t, response.Body.String())
	require.Equal(t, 200, response.Code)
	require.Equal(t, "You received a file: attachment.txt", msg.Message)
	require.Equal(t, int64(5000), msg.Attachment.Size)

	content := util.RandomString(5001) // 5000+5001 > , see below
	response = request(t, s, "PUT", "/mytopic", content, nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 413, response.Code)
	require.Equal(t, 413, err.HTTPCode)
	require.Equal(t, 41301, err.Code)
}

func TestServer_PublishAttachmentAndPrune(t *testing.T) {
	content := util.RandomString(5000) // > 4096

	c := newTestConfig(t)
	c.AttachmentExpiryDuration = time.Millisecond // Hack
	s := newTestServer(t, c)

	// Publish and make sure we can retrieve it
	response := request(t, s, "PUT", "/mytopic", content, nil)
	msg := toMessage(t, response.Body.String())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	file := filepath.Join(s.config.AttachmentCacheDir, msg.ID)
	require.FileExists(t, file)

	path := strings.TrimPrefix(msg.Attachment.URL, "http://127.0.0.1:12345")
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, content, response.Body.String())

	// Prune and makes sure it's gone
	time.Sleep(time.Second) // Sigh ...
	s.execManager()
	require.NoFileExists(t, file)
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 404, response.Code)
}

func TestServer_PublishAttachmentWithTierBasedExpiry(t *testing.T) {
	content := util.RandomString(5000) // > 4096

	c := newTestConfigWithAuthFile(t)
	c.AttachmentExpiryDuration = time.Millisecond // Hack
	s := newTestServer(t, c)

	// Create tier with certain limits
	sevenDays := time.Duration(604800) * time.Second
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		Code:                     "test",
		MessagesLimit:            10,
		MessagesExpiryDuration:   sevenDays,
		AttachmentFileSizeLimit:  50_000,
		AttachmentTotalSizeLimit: 200_000,
		AttachmentExpiryDuration: sevenDays, // 7 days
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	// Publish and make sure we can retrieve it
	response := request(t, s, "PUT", "/mytopic", content, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	msg := toMessage(t, response.Body.String())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	require.True(t, msg.Attachment.Expires > time.Now().Add(sevenDays-30*time.Second).Unix())
	require.True(t, msg.Expires > time.Now().Add(sevenDays-30*time.Second).Unix())
	file := filepath.Join(s.config.AttachmentCacheDir, msg.ID)
	require.FileExists(t, file)

	path := strings.TrimPrefix(msg.Attachment.URL, "http://127.0.0.1:12345")
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, content, response.Body.String())

	// Prune and makes sure it's still there
	time.Sleep(time.Second) // Sigh ...
	s.execManager()
	require.FileExists(t, file)
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, response.Code)
}

func TestServer_PublishAttachmentWithTierBasedLimits(t *testing.T) {
	smallFile := util.RandomString(20_000)
	largeFile := util.RandomString(50_000)

	c := newTestConfigWithAuthFile(t)
	c.AttachmentFileSizeLimit = 20_000
	c.VisitorAttachmentTotalSizeLimit = 40_000
	s := newTestServer(t, c)

	// Create tier with certain limits
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		Code:                     "test",
		MessagesLimit:            100,
		AttachmentFileSizeLimit:  50_000,
		AttachmentTotalSizeLimit: 200_000,
		AttachmentExpiryDuration: 30 * time.Second,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	// Publish small file as anonymous
	response := request(t, s, "PUT", "/mytopic", smallFile, nil)
	msg := toMessage(t, response.Body.String())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, msg.ID))

	// Publish large file as anonymous
	response = request(t, s, "PUT", "/mytopic", largeFile, nil)
	require.Equal(t, 413, response.Code)
	require.Equal(t, 41301, toHTTPError(t, response.Body.String()).Code)

	// Publish too large file as phil
	response = request(t, s, "PUT", "/mytopic", largeFile+" a few more bytes", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 413, response.Code)
	require.Equal(t, 41301, toHTTPError(t, response.Body.String()).Code)

	// Publish large file as phil (4x)
	for i := 0; i < 4; i++ {
		response = request(t, s, "PUT", "/mytopic", largeFile, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
		msg = toMessage(t, response.Body.String())
		require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
		require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, msg.ID))
	}
	response = request(t, s, "PUT", "/mytopic", largeFile, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 413, response.Code)
	require.Equal(t, 41301, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PublishAttachmentBandwidthLimit(t *testing.T) {
	content := util.RandomString(5000) // > 4096

	c := newTestConfig(t)
	c.VisitorAttachmentDailyBandwidthLimit = 5*5000 + 123 // A little more than 1 upload and 3 downloads
	s := newTestServer(t, c)

	// Publish attachment
	response := request(t, s, "PUT", "/mytopic", content, nil)
	msg := toMessage(t, response.Body.String())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")

	// Get it 4 times successfully
	path := strings.TrimPrefix(msg.Attachment.URL, "http://127.0.0.1:12345")
	for i := 1; i <= 4; i++ { // 4 successful downloads
		response = request(t, s, "GET", path, "", nil)
		require.Equal(t, 200, response.Code)
		require.Equal(t, content, response.Body.String())
	}

	// And then fail with a 429
	response = request(t, s, "GET", path, "", nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42905, err.Code)
}

func TestServer_PublishAttachmentBandwidthLimitUploadOnly(t *testing.T) {
	content := util.RandomString(5000) // > 4096

	c := newTestConfig(t)
	c.VisitorAttachmentDailyBandwidthLimit = 5*5000 + 500 // 5 successful uploads
	s := newTestServer(t, c)

	// 5 successful uploads
	for i := 1; i <= 5; i++ {
		response := request(t, s, "PUT", "/mytopic", content, nil)
		msg := toMessage(t, response.Body.String())
		require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")
	}

	// And a failed one
	response := request(t, s, "PUT", "/mytopic", content, nil)
	err := toHTTPError(t, response.Body.String())
	require.Equal(t, 413, response.Code)
	require.Equal(t, 41301, err.Code)
}

func TestServer_PublishAttachmentAccountStats(t *testing.T) {
	content := util.RandomString(4999) // > 4096

	c := newTestConfig(t)
	c.AttachmentFileSizeLimit = 5000
	c.VisitorAttachmentTotalSizeLimit = 6000
	s := newTestServer(t, c)

	// Upload one attachment
	response := request(t, s, "PUT", "/mytopic", content, nil)
	msg := toMessage(t, response.Body.String())
	require.Contains(t, msg.Attachment.URL, "http://127.0.0.1:12345/file/")

	// User stats
	response = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	var account *apiAccountResponse
	require.Nil(t, json.NewDecoder(strings.NewReader(response.Body.String())).Decode(&account))
	require.Equal(t, int64(5000), account.Limits.AttachmentFileSize)
	require.Equal(t, int64(6000), account.Limits.AttachmentTotalSize)
	require.Equal(t, int64(4999), account.Stats.AttachmentTotalSize)
	require.Equal(t, int64(1001), account.Stats.AttachmentTotalSizeRemaining)
}

func TestServer_Visitor_XForwardedFor_None(t *testing.T) {
	c := newTestConfig(t)
	c.BehindProxy = true
	s := newTestServer(t, c)
	r, _ := http.NewRequest("GET", "/bla", nil)
	r.RemoteAddr = "8.9.10.11"
	r.Header.Set("X-Forwarded-For", "  ") // Spaces, not empty!
	v, err := s.visitor(r)
	require.Nil(t, err)
	require.Equal(t, "8.9.10.11", v.ip.String())
}

func TestServer_Visitor_XForwardedFor_Single(t *testing.T) {
	c := newTestConfig(t)
	c.BehindProxy = true
	s := newTestServer(t, c)
	r, _ := http.NewRequest("GET", "/bla", nil)
	r.RemoteAddr = "8.9.10.11"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	v, err := s.visitor(r)
	require.Nil(t, err)
	require.Equal(t, "1.1.1.1", v.ip.String())
}

func TestServer_Visitor_XForwardedFor_Multiple(t *testing.T) {
	c := newTestConfig(t)
	c.BehindProxy = true
	s := newTestServer(t, c)
	r, _ := http.NewRequest("GET", "/bla", nil)
	r.RemoteAddr = "8.9.10.11"
	r.Header.Set("X-Forwarded-For", "1.2.3.4 , 2.4.4.2,234.5.2.1 ")
	v, err := s.visitor(r)
	require.Nil(t, err)
	require.Equal(t, "234.5.2.1", v.ip.String())
}

func TestServer_PublishWhileUpdatingStatsWithLotsOfMessages(t *testing.T) {
	count := 50000
	c := newTestConfig(t)
	c.TotalTopicLimit = 50001
	c.CacheStartupQueries = "pragma journal_mode = WAL; pragma synchronous = normal; pragma temp_store = memory;"
	s := newTestServer(t, c)

	// Add lots of messages
	log.Printf("Adding %d messages", count)
	start := time.Now()
	messages := make([]*message, 0)
	for i := 0; i < count; i++ {
		topicID := fmt.Sprintf("topic%d", i)
		_, err := s.topicsFromIDs(topicID) // Add topic to internal s.topics array
		require.Nil(t, err)
		messages = append(messages, newDefaultMessage(topicID, "some message"))
	}
	require.Nil(t, s.messageCache.addMessages(messages))
	log.Printf("Done: Adding %d messages; took %s", count, time.Since(start).Round(time.Millisecond))

	// Update stats
	statsChan := make(chan bool)
	go func() {
		log.Printf("Updating stats")
		start := time.Now()
		s.execManager()
		log.Printf("Done: Updating stats; took %s", time.Since(start).Round(time.Millisecond))
		statsChan <- true
	}()
	time.Sleep(50 * time.Millisecond) // Make sure it starts first

	// Publish message (during stats update)
	log.Printf("Publishing message")
	start = time.Now()
	response := request(t, s, "PUT", "/mytopic", "some body", nil)
	m := toMessage(t, response.Body.String())
	assert.Equal(t, "some body", m.Message)
	assert.True(t, time.Since(start) < 100*time.Millisecond)
	log.Printf("Done: Publishing message; took %s", time.Since(start).Round(time.Millisecond))

	// Wait for all goroutines
	<-statsChan
	log.Printf("Done: Waiting for all locks")
}

func newTestConfig(t *testing.T) *Config {
	conf := NewConfig()
	conf.BaseURL = "http://127.0.0.1:12345"
	conf.CacheFile = filepath.Join(t.TempDir(), "cache.db")
	conf.AttachmentCacheDir = t.TempDir()
	return conf
}

func newTestConfigWithAuthFile(t *testing.T) *Config {
	conf := newTestConfig(t)
	conf.AuthFile = filepath.Join(t.TempDir(), "user.db")
	return conf
}

func newTestServer(t *testing.T, config *Config) *Server {
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
	req.RemoteAddr = "9.9.9.9" // Used for tests
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

func toHTTPError(t *testing.T, s string) *errHTTP {
	var e errHTTP
	require.Nil(t, json.NewDecoder(strings.NewReader(s)).Decode(&e))
	return &e
}

func basicAuth(s string) string {
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(s)))
}

func readAll(t *testing.T, rc io.ReadCloser) string {
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
