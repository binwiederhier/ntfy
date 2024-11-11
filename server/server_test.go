package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/v2/user"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.ErrorLevel)
	os.Exit(m.Run())
}

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

func TestServer_PublishWithFirebase_WithoutUsers_AndWithoutPanic(t *testing.T) {
	// This tests issue #641, which used to panic before the fix

	firebaseKeyFile := filepath.Join(t.TempDir(), "firebase.json")
	contents := `{
  "type": "service_account",
  "project_id": "ntfy-test",
  "private_key_id": "fsfhskjdfhskdhfskdjfhsdf",
  "private_key": "lalala",
  "client_email": "firebase-adminsdk-muv04@ntfy-test.iam.gserviceaccount.com",
  "client_id": "123123213",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/firebase-adminsdk-muv04%40ntfy-test.iam.gserviceaccount.com"
}
`
	require.Nil(t, os.WriteFile(firebaseKeyFile, []byte(contents), 0600))
	c := newTestConfig(t)
	c.FirebaseKeyFile = firebaseKeyFile
	s := newTestServer(t, c)

	response := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, "my first message", toMessage(t, response.Body.String()).Message)
}

func TestServer_SubscribeOpenAndKeepalive(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, s, "/mytopic/json", subscribeRR)

	publishFirstRR := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, 200, publishFirstRR.Code)
	time.Sleep(500 * time.Millisecond) // Publishing is done asynchronously, this avoids races

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
	require.True(t, time.Now().Add(12*time.Hour-5*time.Second).Unix() < messages[1].Expires)
	require.True(t, time.Now().Add(12*time.Hour+5*time.Second).Unix() > messages[1].Expires)

	require.Equal(t, messageEvent, messages[2].Event)
	require.Equal(t, "mytopic", messages[2].Topic)
	require.Equal(t, "my other message", messages[2].Message)
	require.Equal(t, "This is a title", messages[2].Title)
	require.Equal(t, 1, messages[2].Priority)
	require.Equal(t, []string{"tag1", "tag 2", "tag3"}, messages[2].Tags)
}

func TestServer_Publish_Disallowed_Topic(t *testing.T) {
	c := newTestConfig(t)
	c.DisallowedTopics = []string{"about", "time", "this", "got", "added"}
	s := newTestServer(t, c)

	rr := request(t, s, "PUT", "/mytopic", "my first message", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "PUT", "/about", "another message", nil)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40010, toHTTPError(t, rr.Body.String()).Code)
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
	require.Contains(t, rr.Body.String(), `<meta name="robots" content="noindex, nofollow" />`)

	rr = request(t, s, "GET", "/docs", "", nil)
	require.Equal(t, 301, rr.Code)

	// Docs test removed, it was failing annoyingly.
}

func TestServer_WebEnabled(t *testing.T) {
	conf := newTestConfig(t)
	conf.WebRoot = "" // Disable web app
	s := newTestServer(t, conf)

	rr := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/config.js", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/sw.js", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/app.html", "", nil)
	require.Equal(t, 404, rr.Code)

	rr = request(t, s, "GET", "/static/css/home.css", "", nil)
	require.Equal(t, 404, rr.Code)

	conf2 := newTestConfig(t)
	conf2.WebRoot = "/"
	s2 := newTestServer(t, conf2)

	rr = request(t, s2, "GET", "/", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s2, "GET", "/config.js", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s2, "GET", "/sw.js", "", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s2, "GET", "/app.html", "", nil)
	require.Equal(t, 200, rr.Code)
}

func TestServer_WebPushEnabled(t *testing.T) {
	conf := newTestConfig(t)
	conf.WebRoot = "" // Disable web app
	s := newTestServer(t, conf)

	rr := request(t, s, "GET", "/manifest.webmanifest", "", nil)
	require.Equal(t, 404, rr.Code)

	conf2 := newTestConfig(t)
	s2 := newTestServer(t, conf2)

	rr = request(t, s2, "GET", "/manifest.webmanifest", "", nil)
	require.Equal(t, 404, rr.Code)

	conf3 := newTestConfigWithWebPush(t)
	s3 := newTestServer(t, conf3)

	rr = request(t, s3, "GET", "/manifest.webmanifest", "", nil)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "application/manifest+json", rr.Header().Get("Content-Type"))

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

func TestServer_PublishPriority_SpecialHTTPHeader(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"Priority":   "u=4",
		"X-Priority": "5",
	})
	require.Equal(t, 5, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "POST", "/mytopic?priority=4", "test", map[string]string{
		"Priority": "u=9",
	})
	require.Equal(t, 4, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "POST", "/mytopic", "test", map[string]string{
		"p":        "2",
		"priority": "u=9, i",
	})
	require.Equal(t, 2, toMessage(t, response.Body.String()).Priority)
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
	require.Equal(t, int64(0), msg.Expires)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Empty(t, messages)
}

func TestServer_PublishAt(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "1h",
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 0, len(messages))

	// Update message time to the past
	fakeTime := time.Now().Add(-10 * time.Second).Unix()
	_, err := s.messageCache.db.Exec(`UPDATE messages SET time=?`, fakeTime)
	require.Nil(t, err)

	// Trigger delayed message sending
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "a message", messages[0].Message)
	require.Equal(t, netip.Addr{}, messages[0].Sender) // Never return the sender!

	messages, err = s.messageCache.Messages("mytopic", sinceAllMessages, true)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "a message", messages[0].Message)
	require.Equal(t, "9.9.9.9", messages[0].Sender.String()) // It's stored in the DB though!
}

func TestServer_PublishAt_FromUser(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfigWithAuthFile(t))

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"In":            "1h",
	})
	require.Equal(t, 200, response.Code)

	// Message doesn't show up immediately
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages := toMessages(t, response.Body.String())
	require.Equal(t, 0, len(messages))

	// Update message time to the past
	fakeTime := time.Now().Add(-10 * time.Second).Unix()
	_, err := s.messageCache.db.Exec(`UPDATE messages SET time=?`, fakeTime)
	require.Nil(t, err)

	// Trigger delayed message sending
	require.Nil(t, s.sendDelayedMessages())
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	messages = toMessages(t, response.Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, fakeTime, messages[0].Time)
	require.Equal(t, "a message", messages[0].Message)

	messages, err = s.messageCache.Messages("mytopic", sinceAllMessages, true)
	require.Nil(t, err)
	require.Equal(t, 1, len(messages))
	require.Equal(t, "a message", messages[0].Message)
	require.True(t, strings.HasPrefix(messages[0].User, "u_"))
}

func TestServer_PublishAt_Expires(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
		"In": "2 days",
	})
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.True(t, m.Expires > time.Now().Add(12*time.Hour+48*time.Hour-time.Minute).Unix())
	require.True(t, m.Expires < time.Now().Add(12*time.Hour+48*time.Hour+time.Minute).Unix())
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

	time.Sleep(time.Second) // FIXME CI failing not sure why
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
	t.Parallel()
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
	t.Parallel()
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
	c := newTestConfigWithAuthFile(t)
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"success":true}`+"\n", response.Body.String())
}

func TestServer_Auth_Success_User(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "mytopic", user.PermissionReadWrite))

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, response.Code)
}

func TestServer_Auth_Success_User_MultipleTopics(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "mytopic", user.PermissionReadWrite))
	require.Nil(t, s.userManager.AllowAccess("ben", "anothertopic", user.PermissionReadWrite))

	response := request(t, s, "GET", "/mytopic,anothertopic/auth", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic,anothertopic,NOT-THIS-ONE/auth", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
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
		"Authorization": util.BasicAuth("phil", "INVALID"),
	})
	require.Equal(t, 401, response.Code)
}

func TestServer_Auth_Fail_Unauthorized(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	require.Nil(t, s.userManager.AllowAccess("ben", "sometopic", user.PermissionReadWrite)) // Not mytopic!

	response := request(t, s, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 403, response.Code)
}

func TestServer_Auth_Fail_CannotPublish(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionReadWrite // Open by default
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, s.userManager.AllowAccess(user.Everyone, "private", user.PermissionDenyAll))
	require.Nil(t, s.userManager.AllowAccess(user.Everyone, "announcements", user.PermissionRead))

	response := request(t, s, "PUT", "/mytopic", "test", nil)
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	response = request(t, s, "PUT", "/announcements", "test", nil)
	require.Equal(t, 403, response.Code) // Cannot write as anonymous

	response = request(t, s, "PUT", "/announcements", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "GET", "/announcements/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code) // Anonymous read allowed

	response = request(t, s, "GET", "/private/json?poll=1", "", nil)
	require.Equal(t, 403, response.Code) // Anonymous read not allowed
}

func TestServer_Auth_Fail_Rate_Limiting(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.VisitorAuthFailureLimitBurst = 10
	s := newTestServer(t, c)

	for i := 0; i < 10; i++ {
		response := request(t, s, "PUT", "/announcements", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 401, response.Code)
	}

	response := request(t, s, "PUT", "/announcements", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42909, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Auth_ViaQuery(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)

	require.Nil(t, s.userManager.AddUser("ben", "some pass", user.RoleAdmin))

	u := fmt.Sprintf("/mytopic/json?poll=1&auth=%s", base64.RawURLEncoding.EncodeToString([]byte(util.BasicAuth("ben", "some pass"))))
	response := request(t, s, "GET", u, "", nil)
	require.Equal(t, 200, response.Code)

	u = fmt.Sprintf("/mytopic/json?poll=1&auth=%s", base64.RawURLEncoding.EncodeToString([]byte(util.BasicAuth("ben", "WRONNNGGGG"))))
	response = request(t, s, "GET", u, "", nil)
	require.Equal(t, 401, response.Code)
}

func TestServer_Auth_NonBasicHeader(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))

	response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Authorization": "WebPush not-supported",
	})
	require.Equal(t, 200, response.Code)

	response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Authorization": "Bearer supported",
	})
	require.Equal(t, 401, response.Code)

	response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Authorization": "basic supported",
	})
	require.Equal(t, 401, response.Code)
}

func TestServer_StatsResetter(t *testing.T) {
	t.Parallel()
	// This tests the stats resetter for
	// - an anonymous user
	// - a user without a tier (treated like the same as the anonymous user)
	// - a user with a tier

	c := newTestConfigWithAuthFile(t)
	c.VisitorStatsResetTime = time.Now().Add(2 * time.Second)
	s := newTestServer(t, c)
	go s.runStatsResetter()

	// Create user with tier (tieruser) and user without tier (phil)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                  "test",
		MessageLimit:          5,
		MessageExpiryDuration: -5 * time.Second, // Second, what a hack!
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.AddUser("tieruser", "tieruser", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("tieruser", "test"))

	// Send an anonymous message
	response := request(t, s, "PUT", "/mytopic", "test", nil)
	require.Equal(t, 200, response.Code)

	// Send messages from user without tier (phil)
	for i := 0; i < 5; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
	}

	// Send messages from user with tier
	for i := 0; i < 2; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("tieruser", "tieruser"),
		})
		require.Equal(t, 200, response.Code)
	}

	// User stats show 6 messages (for user without tier)
	response = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(6), account.Stats.Messages)

	// User stats show 6 messages (for anonymous visitor)
	response = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(6), account.Stats.Messages)

	// User stats show 2 messages (for user with tier)
	response = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("tieruser", "tieruser"),
	})
	require.Equal(t, 200, response.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(2), account.Stats.Messages)

	// Wait for stats resetter to run
	waitFor(t, func() bool {
		response = request(t, s, "GET", "/v1/account", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
		account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
		require.Nil(t, err)
		return account.Stats.Messages == 0
	})

	// User stats show 0 messages now!
	response = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)

	// Since this is a user without a tier, the anonymous user should have the same stats
	response = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)

	// User stats show 0 messages (for user with tier)
	response = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("tieruser", "tieruser"),
	})
	require.Equal(t, 200, response.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)
}

func TestServer_StatsResetter_MessageLimiter_EmailsLimiter(t *testing.T) {
	// This tests that the messageLimiter (the only fixed limiter) and the emailsLimiter (token bucket)
	// is reset by the stats resetter

	c := newTestConfigWithAuthFile(t)
	s := newTestServer(t, c)
	s.smtpSender = &testMailer{}

	// Publish some messages, and check stats
	for i := 0; i < 3; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", nil)
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Email": "test@email.com",
	})
	require.Equal(t, 200, response.Code)

	rr := request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, rr.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.Equal(t, int64(4), account.Stats.Messages)
	require.Equal(t, int64(1), account.Stats.Emails)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	require.Equal(t, int64(4), v.Stats().Messages)
	require.Equal(t, int64(4), v.messagesLimiter.Value())
	require.Equal(t, int64(1), v.Stats().Emails)
	require.Equal(t, int64(1), v.emailsLimiter.Value())

	// Reset stats and check again
	s.resetStats()
	rr = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, rr.Code)
	account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)
	require.Equal(t, int64(0), account.Stats.Emails)
	v = s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	require.Equal(t, int64(0), v.Stats().Messages)
	require.Equal(t, int64(0), v.messagesLimiter.Value())
	require.Equal(t, int64(0), v.Stats().Emails)
	require.Equal(t, int64(0), v.emailsLimiter.Value())
}

func TestServer_DailyMessageQuotaFromDatabase(t *testing.T) {
	t.Parallel()

	// This tests that the daily message quota is prefilled originally from the database,
	// if the visitor is unknown

	c := newTestConfigWithAuthFile(t)
	c.AuthStatsQueueWriterInterval = 100 * time.Millisecond
	s := newTestServer(t, c)

	// Create user, and update it with some message and email stats
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code: "test",
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	s.userManager.EnqueueUserStats(u.ID, &user.Stats{
		Messages: 123456,
		Emails:   999,
	})
	time.Sleep(400 * time.Millisecond)

	// Get account and verify stats are read from the DB, and that the visitor also has these stats
	rr := request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.Equal(t, int64(123456), account.Stats.Messages)
	require.Equal(t, int64(999), account.Stats.Emails)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), u)
	require.Equal(t, int64(123456), v.Stats().Messages)
	require.Equal(t, int64(123456), v.messagesLimiter.Value())
	require.Equal(t, int64(999), v.Stats().Emails)
	require.Equal(t, int64(999), v.emailsLimiter.Value())
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
	c.VisitorRequestLimitBurst = 3
	c.VisitorRequestExemptIPAddrs = []netip.Prefix{netip.MustParsePrefix("9.9.9.9/32")} // see request()
	s := newTestServer(t, c)
	for i := 0; i < 5; i++ { // > 3
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
		require.Equal(t, 200, response.Code)
	}
}

func TestServer_PublishTooRequests_Defaults_ExemptHosts_MessageDailyLimit(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorRequestLimitBurst = 10
	c.VisitorMessageDailyLimit = 4
	c.VisitorRequestExemptIPAddrs = []netip.Prefix{netip.MustParsePrefix("9.9.9.9/32")} // see request()
	s := newTestServer(t, c)
	for i := 0; i < 8; i++ { // 4
		response := request(t, s, "PUT", "/mytopic", "message", nil)
		require.Equal(t, 200, response.Code)
	}
}

func TestServer_PublishTooRequests_ShortReplenish(t *testing.T) {
	t.Parallel()
	c := newTestConfig(t)
	c.VisitorRequestLimitBurst = 60
	c.VisitorRequestLimitReplenish = time.Second
	s := newTestServer(t, c)
	for i := 0; i < 60; i++ {
		response := request(t, s, "PUT", "/mytopic", fmt.Sprintf("message %d", i), nil)
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "message", nil)
	require.Equal(t, 429, response.Code)

	time.Sleep(1020 * time.Millisecond)
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
	t.Parallel()
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
	require.Equal(t, 40003, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PublishDelayedCall_Fail(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic", "fail", map[string]string{
		"Call":  "yes",
		"Delay": "20 min",
	})
	require.Equal(t, 40037, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PublishEmailNoMailer_Fail(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "fail", map[string]string{
		"E-Mail": "test@example.com",
	})
	require.Equal(t, 400, response.Code)
}

func TestServer_PublishAndExpungeTopicAfter16Hours(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	defer s.messageCache.Close()

	subFn := func(v *visitor, msg *message) error {
		return nil
	}

	// Publish and check last access
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"Cache": "no",
	})
	require.Equal(t, 200, response.Code)
	waitFor(t, func() bool {
		s.mu.Lock()
		tp, exists := s.topics["mytopic"]
		s.mu.Unlock()
		if !exists {
			return false
		}
		// .lastAccess set in t.Publish() -> t.Keepalive() in Goroutine
		tp.mu.RLock()
		defer tp.mu.RUnlock()
		return tp.lastAccess.Unix() >= time.Now().Unix()-2 &&
			tp.lastAccess.Unix() <= time.Now().Unix()+2
	})

	// Hack!
	time.Sleep(time.Second)

	// Topic won't get pruned
	s.execManager()
	require.NotNil(t, s.topics["mytopic"])

	// Fudge with last access, but subscribe, and see that it won't get pruned (because of subscriber)
	subID := s.topics["mytopic"].Subscribe(subFn, "", func() {})
	s.topics["mytopic"].mu.Lock()
	s.topics["mytopic"].lastAccess = time.Now().Add(-17 * time.Hour)
	s.topics["mytopic"].mu.Unlock()
	s.execManager()
	require.NotNil(t, s.topics["mytopic"])

	// It'll finally get pruned now that there are no subscribers and last access is 17 hours ago
	s.topics["mytopic"].Unsubscribe(subID)
	s.execManager()
	require.Nil(t, s.topics["mytopic"])
}

func TestServer_TopicKeepaliveOnPoll(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))

	// Create topic by polling once
	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	// Mess with last access time
	s.topics["mytopic"].lastAccess = time.Now().Add(-17 * time.Hour)

	// Poll again and check keepalive time
	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)
	require.True(t, s.topics["mytopic"].lastAccess.Unix() >= time.Now().Unix()-2)
	require.True(t, s.topics["mytopic"].lastAccess.Unix() <= time.Now().Unix()+2)
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

	// Register a UnifiedPush subscriber
	response := request(t, s, "GET", "/up123456789012/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	// Publish message to topic
	response = request(t, s, "PUT", "/up123456789012?up=1", string(b), nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "base64", m.Encoding)
	b2, err := base64.StdEncoding.DecodeString(m.Message)
	require.Nil(t, err)
	require.Equal(t, b, b2)

	// Retrieve and check published message
	response = request(t, s, "GET", "/up123456789012/json?poll=1", string(b), nil)
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

	// Register a UnifiedPush subscriber
	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	// Publish message to topic
	response = request(t, s, "PUT", "/mytopic?up=1", string(b), nil)
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

	// Register a UnifiedPush subscriber
	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	// Publish UnifiedPush text message
	response = request(t, s, "PUT", "/mytopic?up=1", "this is a unifiedpush text message", nil)
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

	response := request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)

	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":[]}`+"\n", response.Body.String())

	response = request(t, s, "GET", "/mytopic/json?poll=1", "", nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, notification, m.Message)
}

func TestServer_MatrixGateway_Push_Failure_NoSubscriber(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	require.Equal(t, 50701, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_MatrixGateway_Push_Failure_NoSubscriber_After13Hours(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`

	// No success if no rate visitor set (this also creates the topic in memory)
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 507, response.Code)
	require.Equal(t, 50701, toHTTPError(t, response.Body.String()).Code)
	require.Nil(t, s.topics["mytopic"].rateVisitor)

	// Fake: This topic has been around for 13 hours without a rate visitor
	s.topics["mytopic"].lastAccess = time.Now().Add(-13 * time.Hour)

	// Same request should now return HTTP 200 with a rejected pushkey
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"rejected":["http://127.0.0.1:12345/mytopic?up=1"]}`, strings.TrimSpace(response.Body.String()))

	// Slightly unrelated: Test that topic is pruned after 16 hours
	s.topics["mytopic"].lastAccess = time.Now().Add(-17 * time.Hour)
	s.execManager()
	require.Nil(t, s.topics["mytopic"])
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
	require.Equal(t, 40019, toHTTPError(t, response.Body.String()).Code)

	notification = `this isn't even JSON'`
	response = request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 400, response.Code)
	require.Equal(t, 40019, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_MatrixGateway_Push_Failure_Unconfigured(t *testing.T) {
	c := newTestConfig(t)
	c.BaseURL = ""
	s := newTestServer(t, c)
	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/mytopic?up=1"}]}}`
	response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 500, response.Code)
	require.Equal(t, 50003, toHTTPError(t, response.Body.String()).Code)
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

func TestServer_PublishMarkdown(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "**make this bold**", map[string]string{
		"Content-Type": "text/markdown",
	})
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "**make this bold**", m.Message)
	require.Equal(t, "text/markdown", m.ContentType)
}

func TestServer_PublishMarkdown_QueryParam(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic?md=1", "**make this bold**", nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "**make this bold**", m.Message)
	require.Equal(t, "text/markdown", m.ContentType)
}

func TestServer_PublishMarkdown_NotMarkdown(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", "**make this bold**", map[string]string{
		"Content-Type": "not-markdown",
	})
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "", m.ContentType)
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
	require.Equal(t, "", m.ContentType)

	require.Equal(t, 4, m.Priority)
	require.True(t, m.Time > time.Now().Unix()+29*60)
	require.True(t, m.Time < time.Now().Unix()+31*60)
}

func TestServer_PublishAsJSON_Markdown(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	body := `{"topic":"mytopic","message":"**This is bold**","markdown":true}`
	response := request(t, s, "PUT", "/", body, nil)
	require.Equal(t, 200, response.Code)

	m := toMessage(t, response.Body.String())
	require.Equal(t, "mytopic", m.Topic)
	require.Equal(t, "**This is bold**", m.Message)
	require.Equal(t, "text/markdown", m.ContentType)
}

func TestServer_PublishAsJSON_RateLimit_MessageDailyLimit(t *testing.T) {
	// Publishing as JSON follows a different path. This ensures that rate
	// limiting works for this endpoint as well
	c := newTestConfig(t)
	c.VisitorMessageDailyLimit = 3
	s := newTestServer(t, c)

	for i := 0; i < 3; i++ {
		response := request(t, s, "PUT", "/", `{"topic":"mytopic","message":"A message"}`, nil)
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/", `{"topic":"mytopic","message":"A message"}`, nil)
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42908, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_PublishAsJSON_WithEmail(t *testing.T) {
	t.Parallel()
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
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                  "test",
		MessageLimit:          5,
		MessageExpiryDuration: -5 * time.Second, // Second, what a hack!
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
	content := "text file!" + util.RandomString(4990) // > 4096
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

	response := request(t, s, "PUT", "/mytopic", "text file!"+util.RandomString(4990), nil)
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

func TestServer_PublishAttachmentAndExpire(t *testing.T) {
	t.Parallel()
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
	waitFor(t, func() bool {
		s.execManager() // May run many times
		return !util.FileExists(file)
	})
	response = request(t, s, "GET", path, "", nil)
	require.Equal(t, 404, response.Code)
}

func TestServer_PublishAttachmentWithTierBasedExpiry(t *testing.T) {
	t.Parallel()
	content := util.RandomString(5000) // > 4096

	c := newTestConfigWithAuthFile(t)
	c.AttachmentExpiryDuration = time.Millisecond // Hack
	s := newTestServer(t, c)

	// Create tier with certain limits
	sevenDays := time.Duration(604800) * time.Second
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                     "test",
		MessageLimit:             10,
		MessageExpiryDuration:    sevenDays,
		AttachmentFileSizeLimit:  50_000,
		AttachmentTotalSizeLimit: 200_000,
		AttachmentExpiryDuration: sevenDays, // 7 days
		AttachmentBandwidthLimit: 100000,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	// Publish and make sure we can retrieve it
	response := request(t, s, "PUT", "/mytopic", content, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
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

func TestServer_PublishAttachmentWithTierBasedBandwidthLimit(t *testing.T) {
	content := util.RandomString(5000) // > 4096

	c := newTestConfigWithAuthFile(t)
	c.VisitorAttachmentDailyBandwidthLimit = 1000 // Much lower than tier bandwidth!
	s := newTestServer(t, c)

	// Create tier with certain limits
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                     "test",
		MessageLimit:             10,
		MessageExpiryDuration:    time.Hour,
		AttachmentFileSizeLimit:  50_000,
		AttachmentTotalSizeLimit: 200_000,
		AttachmentExpiryDuration: time.Hour,
		AttachmentBandwidthLimit: 14000, // < 3x5000 bytes -> enough for one upload, one download
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))

	// Publish and make sure we can retrieve it
	rr := request(t, s, "PUT", "/mytopic", content, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	msg := toMessage(t, rr.Body.String())

	// Retrieve it (first time succeeds)
	rr = request(t, s, "GET", "/file/"+msg.ID, content, nil) // File downloads do not send auth headers!!
	require.Equal(t, 200, rr.Code)
	require.Equal(t, content, rr.Body.String())

	// Retrieve it AGAIN (fails, due to bandwidth limit)
	rr = request(t, s, "GET", "/file/"+msg.ID, content, nil)
	require.Equal(t, 429, rr.Code)
}

func TestServer_PublishAttachmentWithTierBasedLimits(t *testing.T) {
	smallFile := util.RandomString(20_000)
	largeFile := util.RandomString(50_000)

	c := newTestConfigWithAuthFile(t)
	c.AttachmentFileSizeLimit = 20_000
	c.VisitorAttachmentTotalSizeLimit = 40_000
	s := newTestServer(t, c)

	// Create tier with certain limits
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                     "test",
		MessageLimit:             100,
		AttachmentFileSizeLimit:  50_000,
		AttachmentTotalSizeLimit: 200_000,
		AttachmentExpiryDuration: 30 * time.Second,
		AttachmentBandwidthLimit: 1000000,
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

	// Value it 4 times successfully
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

func TestServer_PublishAttachmentAndImmediatelyGetItWithCacheTimeout(t *testing.T) {
	// This tests the awkward util.Retry in handleFile: Due to the async persisting of messages,
	// the message is not immediately available when attempting to download it.

	c := newTestConfig(t)
	c.CacheBatchTimeout = 500 * time.Millisecond
	c.CacheBatchSize = 10
	s := newTestServer(t, c)
	content := "this is an ATTACHMENT"
	rr := request(t, s, "PUT", "/mytopic?f=myfile.txt", content, nil)
	m := toMessage(t, rr.Body.String())
	require.Equal(t, "myfile.txt", m.Attachment.Name)

	path := strings.TrimPrefix(m.Attachment.URL, "http://127.0.0.1:12345")
	rr = request(t, s, "GET", path, "", nil)
	require.Equal(t, 200, rr.Code) // Not 404!
	require.Equal(t, content, rr.Body.String())
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
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(5000), account.Limits.AttachmentFileSize)
	require.Equal(t, int64(6000), account.Limits.AttachmentTotalSize)
	require.Equal(t, int64(4999), account.Stats.AttachmentTotalSize)
	require.Equal(t, int64(1001), account.Stats.AttachmentTotalSizeRemaining)
	require.Equal(t, int64(1), account.Stats.Messages)
}

func TestServer_Visitor_XForwardedFor_None(t *testing.T) {
	c := newTestConfig(t)
	c.BehindProxy = true
	s := newTestServer(t, c)
	r, _ := http.NewRequest("GET", "/bla", nil)
	r.RemoteAddr = "8.9.10.11"
	r.Header.Set("X-Forwarded-For", "  ") // Spaces, not empty!
	v, err := s.maybeAuthenticate(r)
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
	v, err := s.maybeAuthenticate(r)
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
	v, err := s.maybeAuthenticate(r)
	require.Nil(t, err)
	require.Equal(t, "234.5.2.1", v.ip.String())
}

func TestServer_PublishWhileUpdatingStatsWithLotsOfMessages(t *testing.T) {
	t.Parallel()
	count := 50000
	c := newTestConfig(t)
	c.TotalTopicLimit = 50001
	c.CacheStartupQueries = "pragma journal_mode = WAL; pragma synchronous = normal; pragma temp_store = memory;"
	s := newTestServer(t, c)

	// Add lots of messages
	log.Info("Adding %d messages", count)
	start := time.Now()
	messages := make([]*message, 0)
	for i := 0; i < count; i++ {
		topicID := fmt.Sprintf("topic%d", i)
		_, err := s.topicsFromIDs(topicID) // Add topic to internal s.topics array
		require.Nil(t, err)
		messages = append(messages, newDefaultMessage(topicID, "some message"))
	}
	require.Nil(t, s.messageCache.addMessages(messages))
	log.Info("Done: Adding %d messages; took %s", count, time.Since(start).Round(time.Millisecond))

	// Update stats
	statsChan := make(chan bool)
	go func() {
		log.Info("Updating stats")
		start := time.Now()
		s.execManager()
		log.Info("Done: Updating stats; took %s", time.Since(start).Round(time.Millisecond))
		statsChan <- true
	}()
	time.Sleep(50 * time.Millisecond) // Make sure it starts first

	// Publish message (during stats update)
	log.Info("Publishing message")
	start = time.Now()
	response := request(t, s, "PUT", "/mytopic", "some body", nil)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "some body", m.Message)
	require.True(t, time.Since(start) < 100*time.Millisecond)
	log.Info("Done: Publishing message; took %s", time.Since(start).Round(time.Millisecond))

	// Wait for all goroutines
	select {
	case <-statsChan:
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for Go routines")
	}
	log.Info("Done: Waiting for all locks")
}

func TestServer_AnonymousUser_And_NonTierUser_Are_Same_Visitor(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	s := newTestServer(t, conf)
	defer s.closeDatabases()

	// Create user without tier
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	// Publish a message (anonymous user)
	rr := request(t, s, "POST", "/mytopic", "hi", nil)
	require.Equal(t, 200, rr.Code)

	// Publish a message (non-tier user)
	rr = request(t, s, "POST", "/mytopic", "hi", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// User stats (anonymous user)
	rr = request(t, s, "GET", "/v1/account", "", nil)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(2), account.Stats.Messages)

	// User stats (non-tier user)
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(2), account.Stats.Messages)
}

func TestServer_SubscriberRateLimiting_Success(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.VisitorRequestLimitBurst = 3
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	// "Register" visitor 1.2.3.4 to topic "upAAAAAAAAAAAA" as a rate limit visitor
	subscriber1Fn := func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4"
	}
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, subscriber1Fn)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Body.String())
	require.Equal(t, "1.2.3.4", s.topics["upAAAAAAAAAAAA"].rateVisitor.ip.String())

	// "Register" visitor 8.7.7.1 to topic "up012345678912" as a rate limit visitor (implicitly via topic name)
	subscriber2Fn := func(r *http.Request) {
		r.RemoteAddr = "8.7.7.1"
	}
	rr = request(t, s, "GET", "/up012345678912/json?poll=1", "", nil, subscriber2Fn)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Body.String())
	require.Equal(t, "8.7.7.1", s.topics["up012345678912"].rateVisitor.ip.String())

	// Publish 2 messages to "subscriber1topic" as visitor 9.9.9.9. It'd be 3 normally, but the
	// GET request before is also counted towards the request limiter.
	for i := 0; i < 2; i++ {
		rr := request(t, s, "PUT", "/upAAAAAAAAAAAA", "some message", nil)
		require.Equal(t, 200, rr.Code)
	}
	rr = request(t, s, "PUT", "/upAAAAAAAAAAAA", "some message", nil)
	require.Equal(t, 429, rr.Code)

	// Publish another 2 messages to "up012345678912" as visitor 9.9.9.9
	for i := 0; i < 2; i++ {
		rr := request(t, s, "PUT", "/up012345678912", "some message", nil)
		require.Equal(t, 200, rr.Code) // If we fail here, handlePublish is using the wrong visitor!
	}
	rr = request(t, s, "PUT", "/up012345678912", "some message", nil)
	require.Equal(t, 429, rr.Code)

	// Hurray! At this point, visitor 9.9.9.9 has published 4 messages, even though
	// VisitorRequestLimitBurst is 3. That means it's working.

	// Now let's confirm that so far we haven't used up any of visitor 9.9.9.9's request limiter
	// by publishing another 3 requests from it.
	for i := 0; i < 3; i++ {
		rr := request(t, s, "PUT", "/some-other-topic", "some message", nil)
		require.Equal(t, 200, rr.Code)
	}
	rr = request(t, s, "PUT", "/some-other-topic", "some message", nil)
	require.Equal(t, 429, rr.Code)
}

func TestServer_SubscriberRateLimiting_NotWrongTopic(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	subscriberFn := func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4"
	}
	rr := request(t, s, "GET", "/alerts,upAAAAAAAAAAAA,upBBBBBBBBBBBB/json?poll=1", "", nil, subscriberFn)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Body.String())
	require.Nil(t, s.topics["alerts"].rateVisitor)
	require.Equal(t, "1.2.3.4", s.topics["upAAAAAAAAAAAA"].rateVisitor.ip.String())
	require.Equal(t, "1.2.3.4", s.topics["upBBBBBBBBBBBB"].rateVisitor.ip.String())
}

func TestServer_SubscriberRateLimiting_NotEnabled_Failed(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.VisitorRequestLimitBurst = 3
	c.VisitorSubscriberRateLimiting = false
	s := newTestServer(t, c)

	// Subscriber rate limiting is disabled!

	// Registering visitor 1.2.3.4 to topic has no effect
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4"
	})
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Body.String())
	require.Nil(t, s.topics["upAAAAAAAAAAAA"].rateVisitor)

	// Registering visitor 8.7.7.1 to topic has no effect
	rr = request(t, s, "GET", "/up012345678912/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "8.7.7.1"
	})
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Body.String())
	require.Nil(t, s.topics["up012345678912"].rateVisitor)

	// Publish 3 messages to "upAAAAAAAAAAAA" as visitor 9.9.9.9
	for i := 0; i < 3; i++ {
		rr := request(t, s, "PUT", "/subscriber1topic", "some message", nil)
		require.Equal(t, 200, rr.Code)
	}
	rr = request(t, s, "PUT", "/subscriber1topic", "some message", nil)
	require.Equal(t, 429, rr.Code)
	rr = request(t, s, "PUT", "/up012345678912", "some message", nil)
	require.Equal(t, 429, rr.Code)
}

func TestServer_SubscriberRateLimiting_UP_Only(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.VisitorRequestLimitBurst = 3
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	// "Register" 5 different UnifiedPush visitors
	for i := 0; i < 5; i++ {
		subscriberFn := func(r *http.Request) {
			r.RemoteAddr = fmt.Sprintf("1.2.3.%d", i+1)
		}
		rr := request(t, s, "GET", fmt.Sprintf("/up12345678901%d/json?poll=1", i), "", nil, subscriberFn)
		require.Equal(t, 200, rr.Code)
	}

	// Publish 2 messages per topic
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			rr := request(t, s, "PUT", fmt.Sprintf("/up12345678901%d?up=1", i), "some message", nil)
			require.Equal(t, 200, rr.Code)
		}
	}
}

func TestServer_Matrix_SubscriberRateLimiting_UP_Only(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorRequestLimitBurst = 3
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	// "Register" 5 different UnifiedPush visitors
	for i := 0; i < 5; i++ {
		rr := request(t, s, "GET", fmt.Sprintf("/up12345678901%d/json?poll=1", i), "", nil, func(r *http.Request) {
			r.RemoteAddr = fmt.Sprintf("1.2.3.%d", i+1)
		})
		require.Equal(t, 200, rr.Code)
	}

	// Publish 2 messages per topic
	for i := 0; i < 5; i++ {
		notification := fmt.Sprintf(`{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/up12345678901%d?up=1"}]}}`, i)
		for j := 0; j < 2; j++ {
			response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
			require.Equal(t, 200, response.Code)
			require.Equal(t, `{"rejected":[]}`+"\n", response.Body.String())
		}
		response := request(t, s, "POST", "/_matrix/push/v1/notify", notification, nil)
		require.Equal(t, 429, response.Code, notification)
		require.Equal(t, 42901, toHTTPError(t, response.Body.String()).Code)
	}
}

func TestServer_SubscriberRateLimiting_VisitorExpiration(t *testing.T) {
	c := newTestConfig(t)
	c.VisitorRequestLimitBurst = 3
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	// "Register" rate visitor
	subscriberFn := func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4"
	}
	rr := request(t, s, "GET", "/upAAAAAAAAAAAA/json?poll=1", "", nil, subscriberFn)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "1.2.3.4", s.topics["upAAAAAAAAAAAA"].rateVisitor.ip.String())
	require.Equal(t, s.visitors["ip:1.2.3.4"], s.topics["upAAAAAAAAAAAA"].rateVisitor)

	// Publish message, observe rate visitor tokens being decreased
	response := request(t, s, "POST", "/upAAAAAAAAAAAA", "some message", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, int64(0), s.visitors["ip:9.9.9.9"].messagesLimiter.Value())
	require.Equal(t, int64(1), s.topics["upAAAAAAAAAAAA"].rateVisitor.messagesLimiter.Value())
	require.Equal(t, s.visitors["ip:1.2.3.4"], s.topics["upAAAAAAAAAAAA"].rateVisitor)

	// Expire visitor
	s.visitors["ip:1.2.3.4"].seen = time.Now().Add(-1 * 25 * time.Hour)
	s.pruneVisitors()

	// Publish message again, observe that rateVisitor is not used anymore and is reset
	response = request(t, s, "POST", "/upAAAAAAAAAAAA", "some message", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, int64(1), s.visitors["ip:9.9.9.9"].messagesLimiter.Value())
	require.Nil(t, s.topics["upAAAAAAAAAAAA"].rateVisitor)
	require.Nil(t, s.visitors["ip:1.2.3.4"])
}

func TestServer_SubscriberRateLimiting_ProtectedTopics_WithDefaultReadWrite(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.AuthDefault = user.PermissionReadWrite
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)

	// Create some ACLs
	require.Nil(t, s.userManager.AllowAccess(user.Everyone, "announcements", user.PermissionRead))

	// Set rate visitor as ip:1.2.3.4 on topic
	// - "up123456789012": Allowed, because no ACLs and nobody owns the topic
	// - "announcements": NOT allowed, because it has read-only permissions for everyone
	rr := request(t, s, "GET", "/up123456789012,announcements/json?poll=1", "", nil, func(r *http.Request) {
		r.RemoteAddr = "1.2.3.4"
	})
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "1.2.3.4", s.topics["up123456789012"].rateVisitor.ip.String())
	require.Nil(t, s.topics["announcements"].rateVisitor)
}

func TestServer_MessageHistoryAndStatsEndpoint(t *testing.T) {
	c := newTestConfig(t)
	c.ManagerInterval = 2 * time.Second
	s := newTestServer(t, c)

	// Publish some messages, and get stats
	for i := 0; i < 5; i++ {
		response := request(t, s, "POST", "/mytopic", "some message", nil)
		require.Equal(t, 200, response.Code)
	}
	require.Equal(t, int64(5), s.messages)
	require.Equal(t, []int64{0}, s.messagesHistory)

	response := request(t, s, "GET", "/v1/stats", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"messages":5,"messages_rate":0}`+"\n", response.Body.String())

	// Run manager and see message history update
	s.execManager()
	require.Equal(t, []int64{0, 5}, s.messagesHistory)

	response = request(t, s, "GET", "/v1/stats", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"messages":5,"messages_rate":2.5}`+"\n", response.Body.String()) // 5 messages in 2 seconds = 2.5 messages per second

	// Publish some more messages
	for i := 0; i < 10; i++ {
		response := request(t, s, "POST", "/mytopic", "some message", nil)
		require.Equal(t, 200, response.Code)
	}
	require.Equal(t, int64(15), s.messages)
	require.Equal(t, []int64{0, 5}, s.messagesHistory)

	response = request(t, s, "GET", "/v1/stats", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"messages":15,"messages_rate":2.5}`+"\n", response.Body.String()) // Rate did not update yet

	// Run manager and see message history update
	s.execManager()
	require.Equal(t, []int64{0, 5, 15}, s.messagesHistory)

	response = request(t, s, "GET", "/v1/stats", "", nil)
	require.Equal(t, 200, response.Code)
	require.Equal(t, `{"messages":15,"messages_rate":3.75}`+"\n", response.Body.String()) // 15 messages in 4 seconds = 3.75 messages per second
}

func TestServer_MessageHistoryMaxSize(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	for i := 0; i < 20; i++ {
		s.messages = int64(i)
		s.execManager()
	}
	require.Equal(t, []int64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}, s.messagesHistory)
}

func TestServer_MessageCountPersistence(t *testing.T) {
	t.Parallel()
	c := newTestConfig(t)
	s := newTestServer(t, c)
	s.messages = 1234
	s.execManager()
	waitFor(t, func() bool {
		messages, err := s.messageCache.Stats()
		require.Nil(t, err)
		return messages == 1234
	})

	s = newTestServer(t, c)
	require.Equal(t, int64(1234), s.messages)
}

func TestServer_PublishWithUTF8MimeHeader(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))

	response := request(t, s, "POST", "/mytopic", "some attachment", map[string]string{
		"X-Filename": "some =?UTF-8?q?=C3=A4?=ttachment.txt",
		"X-Message":  "=?UTF-8?B?8J+HqfCfh6o=?=",
		"X-Title":    "=?UTF-8?B?bnRmeSDlvojmo5I=?=, no really I mean it! =?UTF-8?Q?This is q=C3=BC=C3=B6ted-print=C3=A4ble.?=",
		"X-Tags":     "=?UTF-8?B?8J+HqfCfh6o=?=, =?UTF-8?B?bnRmeSDlvojmo5I=?=",
		"X-Click":    "=?uTf-8?b?aHR0cHM6Ly/wn5KpLmxh?=",
		"X-Actions":  "http, \"=?utf-8?q?Mettre =C3=A0 jour?=\", \"https://my.tld/webhook/netbird-update\"; =?utf-8?b?aHR0cCwg6L+Z5piv5LiA5Liq5qCH562+LCBodHRwczovL/CfkqkubGE=?=",
	})
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "🇩🇪", m.Message)
	require.Equal(t, "ntfy 很棒, no really I mean it! This is qüöted-printäble.", m.Title)
	require.Equal(t, "some ättachment.txt", m.Attachment.Name)
	require.Equal(t, "🇩🇪", m.Tags[0])
	require.Equal(t, "ntfy 很棒", m.Tags[1])
	require.Equal(t, "https://💩.la", m.Click)
	require.Equal(t, "Mettre à jour", m.Actions[0].Label)
	require.Equal(t, "http", m.Actions[1].Action)
	require.Equal(t, "这是一个标签", m.Actions[1].Label)
	require.Equal(t, "https://💩.la", m.Actions[1].URL)
}

func TestServer_UpstreamBaseURL_Success(t *testing.T) {
	t.Parallel()
	var pollID atomic.Pointer[string]
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/87c9cddf7b0105f5fe849bf084c6e600be0fde99be3223335199b4965bd7b735", r.URL.Path)
		require.Equal(t, "", string(body))
		require.NotEmpty(t, r.Header.Get("X-Poll-ID"))
		pollID.Store(util.String(r.Header.Get("X-Poll-ID")))
	}))
	defer upstreamServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.BaseURL = "http://myserver.internal"
	c.UpstreamBaseURL = upstreamServer.URL
	s := newTestServer(t, c)

	// Send message, and wait for upstream server to receive it
	response := request(t, s, "PUT", "/mytopic", `hi there`, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.NotEmpty(t, m.ID)
	require.Equal(t, "hi there", m.Message)
	waitFor(t, func() bool {
		pID := pollID.Load()
		return pID != nil && *pID == m.ID
	})
}

func TestServer_UpstreamBaseURL_With_Access_Token_Success(t *testing.T) {
	t.Parallel()
	var pollID atomic.Pointer[string]
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/a1c72bcb4daf5af54d13ef86aea8f76c11e8b88320d55f1811d5d7b173bcc1df", r.URL.Path)
		require.Equal(t, "Bearer tk_1234567890", r.Header.Get("Authorization"))
		require.Equal(t, "", string(body))
		require.NotEmpty(t, r.Header.Get("X-Poll-ID"))
		pollID.Store(util.String(r.Header.Get("X-Poll-ID")))
	}))
	defer upstreamServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.BaseURL = "http://myserver.internal"
	c.UpstreamBaseURL = upstreamServer.URL
	c.UpstreamAccessToken = "tk_1234567890"
	s := newTestServer(t, c)

	// Send message, and wait for upstream server to receive it
	response := request(t, s, "PUT", "/mytopic1", `hi there`, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.NotEmpty(t, m.ID)
	require.Equal(t, "hi there", m.Message)
	waitFor(t, func() bool {
		pID := pollID.Load()
		return pID != nil && *pID == m.ID
	})
}

func TestServer_UpstreamBaseURL_DoNotForwardUnifiedPush(t *testing.T) {
	t.Parallel()
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("UnifiedPush messages should not be forwarded")
	}))
	defer upstreamServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.BaseURL = "http://myserver.internal"
	c.UpstreamBaseURL = upstreamServer.URL
	s := newTestServer(t, c)

	// Send UP message, this should not forward to upstream server
	response := request(t, s, "PUT", "/mytopic?up=1", `hi there`, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.NotEmpty(t, m.ID)
	require.Equal(t, "hi there", m.Message)

	// Forwarding is done asynchronously, so wait a bit.
	// This ensures that the t.Fatal above is actually not triggered.
	time.Sleep(500 * time.Millisecond)
}

func TestServer_MessageTemplate(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", `{"foo":"bar", "nested":{"title":"here"}}`, map[string]string{
		"X-Message":  "{{.foo}}",
		"X-Title":    "{{.nested.title}}",
		"X-Template": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "bar", m.Message)
	require.Equal(t, "here", m.Title)
}

func TestServer_MessageTemplate_RepeatPlaceholder(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", `{"foo":"bar", "nested":{"title":"here"}}`, map[string]string{
		"Message":  "{{.foo}} is {{.foo}}",
		"Title":    "{{.nested.title}} is {{.nested.title}}",
		"Template": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "bar is bar", m.Message)
	require.Equal(t, "here is here", m.Title)
}

func TestServer_MessageTemplate_JSONBody(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	body := `{"topic": "mytopic", "message": "{\"foo\":\"bar\",\"nested\":{\"title\":\"here\"}}"}`
	response := request(t, s, "PUT", "/", body, map[string]string{
		"m":   "{{.foo}}",
		"t":   "{{.nested.title}}",
		"tpl": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "bar", m.Message)
	require.Equal(t, "here", m.Title)
}

func TestServer_MessageTemplate_MalformedJSONBody(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	body := `{"topic": "mytopic", "message": "{\"foo\":\"bar\",\"nested\":{\"title\":\"here\"INVALID"}`
	response := request(t, s, "PUT", "/", body, map[string]string{
		"X-Message":  "{{.foo}}",
		"X-Title":    "{{.nested.title}}",
		"X-Template": "1",
	})

	require.Equal(t, 400, response.Code)
	require.Equal(t, 40042, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_MessageTemplate_PlaceholderTypo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", `{"foo":"bar", "nested":{"title":"here"}}`, map[string]string{
		"X-Message":  "{{.food}}",
		"X-Title":    "{{.neste.title}}",
		"X-Template": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "<no value>", m.Message)
	require.Equal(t, "<no value>", m.Title)
}

func TestServer_MessageTemplate_MultiplePlaceholders(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "PUT", "/mytopic", `{"foo":"bar", "nested":{"title":"here"}}`, map[string]string{
		"X-Message":  "{{.foo}} is {{.nested.title}}",
		"X-Template": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "bar is here", m.Message)
}

func TestServer_MessageTemplate_Range(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	jsonBody := `{"foo": "bar", "errors": [{"level": "severe", "url": "https://severe1.com"},{"level": "warning", "url": "https://warning.com"},{"level": "severe", "url": "https://severe2.com"}]}`
	response := request(t, s, "PUT", "/mytopic", jsonBody, map[string]string{
		"X-Message":  `Severe URLs:\n{{range .errors}}{{if eq .level "severe"}}- {{.url}}\n{{end}}{{end}}`,
		"X-Template": "1",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "Severe URLs:\n- https://severe1.com\n- https://severe2.com\n", m.Message)
}

func TestServer_MessageTemplate_ExceedMessageSize_TemplatedMessageOK(t *testing.T) {
	t.Parallel()
	c := newTestConfig(t)
	c.MessageSizeLimit = 25 // 25 < len(HTTP body) < 32k, and len(m.Message) < 25
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic", `{"foo":"bar", "nested":{"title":"here"}}`, map[string]string{
		"X-Message":  "{{.foo}}",
		"X-Title":    "{{.nested.title}}",
		"X-Template": "yes",
	})

	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "bar", m.Message)
	require.Equal(t, "here", m.Title)
}

func TestServer_MessageTemplate_ExceedMessageSize_TemplatedMessageTooLong(t *testing.T) {
	t.Parallel()
	c := newTestConfig(t)
	c.MessageSizeLimit = 21 // 21 < len(HTTP body) < 32k, but !len(m.Message) < 21
	s := newTestServer(t, c)
	response := request(t, s, "PUT", "/mytopic", `{"foo":"This is a long message"}`, map[string]string{
		"X-Message":  "{{.foo}}",
		"X-Template": "1",
	})

	require.Equal(t, 400, response.Code)
	require.Equal(t, 40041, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_MessageTemplate_Grafana(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	body := `{"receiver":"ntfy\\.example\\.com/alerts","status":"resolved","alerts":[{"status":"resolved","labels":{"alertname":"Load avg 15m too high","grafana_folder":"Node alerts","instance":"10.108.0.2:9100","job":"node-exporter"},"annotations":{"summary":"15m load average too high"},"startsAt":"2024-03-15T02:28:00Z","endsAt":"2024-03-15T02:42:00Z","generatorURL":"localhost:3000/alerting/grafana/NW9oDw-4z/view","fingerprint":"becbfb94bd81ef48","silenceURL":"localhost:3000/alerting/silence/new?alertmanager=grafana&matcher=alertname%3DLoad+avg+15m+too+high&matcher=grafana_folder%3DNode+alerts&matcher=instance%3D10.108.0.2%3A9100&matcher=job%3Dnode-exporter","dashboardURL":"","panelURL":"","values":{"B":18.98211314475876,"C":0},"valueString":"[ var='B' labels={__name__=node_load15, instance=10.108.0.2:9100, job=node-exporter} value=18.98211314475876 ], [ var='C' labels={__name__=node_load15, instance=10.108.0.2:9100, job=node-exporter} value=0 ]"}],"groupLabels":{"alertname":"Load avg 15m too high","grafana_folder":"Node alerts"},"commonLabels":{"alertname":"Load avg 15m too high","grafana_folder":"Node alerts","instance":"10.108.0.2:9100","job":"node-exporter"},"commonAnnotations":{"summary":"15m load average too high"},"externalURL":"localhost:3000/","version":"1","groupKey":"{}:{alertname=\"Load avg 15m too high\", grafana_folder=\"Node alerts\"}","truncatedAlerts":0,"orgId":1,"title":"[RESOLVED] Load avg 15m too high Node alerts (10.108.0.2:9100 node-exporter)","state":"ok","message":"**Resolved**\n\nValue: B=18.98211314475876, C=0\nLabels:\n - alertname = Load avg 15m too high\n - grafana_folder = Node alerts\n - instance = 10.108.0.2:9100\n - job = node-exporter\nAnnotations:\n - summary = 15m load average too high\nSource: localhost:3000/alerting/grafana/NW9oDw-4z/view\nSilence: localhost:3000/alerting/silence/new?alertmanager=grafana&matcher=alertname%3DLoad+avg+15m+too+high&matcher=grafana_folder%3DNode+alerts&matcher=instance%3D10.108.0.2%3A9100&matcher=job%3Dnode-exporter\n"}`
	response := request(t, s, "PUT", "/mytopic?tpl=yes&title=Grafana+alert:+{{.title}}&message={{.message}}", body, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "Grafana alert: [RESOLVED] Load avg 15m too high Node alerts (10.108.0.2:9100 node-exporter)", m.Title)
	require.Equal(t, `**Resolved**

Value: B=18.98211314475876, C=0
Labels:
 - alertname = Load avg 15m too high
 - grafana_folder = Node alerts
 - instance = 10.108.0.2:9100
 - job = node-exporter
Annotations:
 - summary = 15m load average too high
Source: localhost:3000/alerting/grafana/NW9oDw-4z/view
Silence: localhost:3000/alerting/silence/new?alertmanager=grafana&matcher=alertname%3DLoad+avg+15m+too+high&matcher=grafana_folder%3DNode+alerts&matcher=instance%3D10.108.0.2%3A9100&matcher=job%3Dnode-exporter
`, m.Message)
}

func TestServer_MessageTemplate_GitHub(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	body := `{"action":"opened","number":1,"pull_request":{"url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1","id":1783420972,"node_id":"PR_kwDOHAbdo85qTNgs","html_url":"https://github.com/binwiederhier/dabble/pull/1","diff_url":"https://github.com/binwiederhier/dabble/pull/1.diff","patch_url":"https://github.com/binwiederhier/dabble/pull/1.patch","issue_url":"https://api.github.com/repos/binwiederhier/dabble/issues/1","number":1,"state":"open","locked":false,"title":"A sample PR from Phil","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"body":null,"created_at":"2024-03-21T02:52:09Z","updated_at":"2024-03-21T02:52:09Z","closed_at":null,"merged_at":null,"merge_commit_sha":null,"assignee":null,"assignees":[],"requested_reviewers":[],"requested_teams":[],"labels":[],"milestone":null,"draft":false,"commits_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/commits","review_comments_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/comments","review_comment_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/comments{/number}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/issues/1/comments","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/5703842cc5715ed1e358d23ebb693db09747ae9b","head":{"label":"binwiederhier:aa","ref":"aa","sha":"5703842cc5715ed1e358d23ebb693db09747ae9b","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"repo":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main","allow_squash_merge":true,"allow_merge_commit":true,"allow_rebase_merge":true,"allow_auto_merge":false,"delete_branch_on_merge":false,"allow_update_branch":false,"use_squash_pr_title_as_default":false,"squash_merge_commit_message":"COMMIT_MESSAGES","squash_merge_commit_title":"COMMIT_OR_PR_TITLE","merge_commit_message":"PR_TITLE","merge_commit_title":"MERGE_MESSAGE"}},"base":{"label":"binwiederhier:main","ref":"main","sha":"72d931a20bb83d123ab45accaf761150c8b01211","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"repo":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main","allow_squash_merge":true,"allow_merge_commit":true,"allow_rebase_merge":true,"allow_auto_merge":false,"delete_branch_on_merge":false,"allow_update_branch":false,"use_squash_pr_title_as_default":false,"squash_merge_commit_message":"COMMIT_MESSAGES","squash_merge_commit_title":"COMMIT_OR_PR_TITLE","merge_commit_message":"PR_TITLE","merge_commit_title":"MERGE_MESSAGE"}},"_links":{"self":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1"},"html":{"href":"https://github.com/binwiederhier/dabble/pull/1"},"issue":{"href":"https://api.github.com/repos/binwiederhier/dabble/issues/1"},"comments":{"href":"https://api.github.com/repos/binwiederhier/dabble/issues/1/comments"},"review_comments":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/comments"},"review_comment":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/comments{/number}"},"commits":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/commits"},"statuses":{"href":"https://api.github.com/repos/binwiederhier/dabble/statuses/5703842cc5715ed1e358d23ebb693db09747ae9b"}},"author_association":"OWNER","auto_merge":null,"active_lock_reason":null,"merged":false,"mergeable":null,"rebaseable":null,"mergeable_state":"unknown","merged_by":null,"comments":0,"review_comments":0,"maintainer_can_modify":false,"commits":1,"additions":1,"deletions":1,"changed_files":1},"repository":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main"},"sender":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false}}`
	response := request(t, s, "PUT", `/mytopic?tpl=yes&message=[{{.pull_request.head.repo.full_name}}]+Pull+request+{{if+eq+.action+"opened"}}OPENED{{else}}CLOSED{{end}}:+{{.pull_request.title}}`, body, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, "", m.Title)
	require.Equal(t, `[binwiederhier/dabble] Pull request OPENED: A sample PR from Phil`, m.Message)
}

func TestServer_MessageTemplate_GitHub2(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	body := `{"action":"opened","number":1,"pull_request":{"url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1","id":1783420972,"node_id":"PR_kwDOHAbdo85qTNgs","html_url":"https://github.com/binwiederhier/dabble/pull/1","diff_url":"https://github.com/binwiederhier/dabble/pull/1.diff","patch_url":"https://github.com/binwiederhier/dabble/pull/1.patch","issue_url":"https://api.github.com/repos/binwiederhier/dabble/issues/1","number":1,"state":"open","locked":false,"title":"A sample PR from Phil","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"body":null,"created_at":"2024-03-21T02:52:09Z","updated_at":"2024-03-21T02:52:09Z","closed_at":null,"merged_at":null,"merge_commit_sha":null,"assignee":null,"assignees":[],"requested_reviewers":[],"requested_teams":[],"labels":[],"milestone":null,"draft":false,"commits_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/commits","review_comments_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/comments","review_comment_url":"https://api.github.com/repos/binwiederhier/dabble/pulls/comments{/number}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/issues/1/comments","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/5703842cc5715ed1e358d23ebb693db09747ae9b","head":{"label":"binwiederhier:aa","ref":"aa","sha":"5703842cc5715ed1e358d23ebb693db09747ae9b","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"repo":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main","allow_squash_merge":true,"allow_merge_commit":true,"allow_rebase_merge":true,"allow_auto_merge":false,"delete_branch_on_merge":false,"allow_update_branch":false,"use_squash_pr_title_as_default":false,"squash_merge_commit_message":"COMMIT_MESSAGES","squash_merge_commit_title":"COMMIT_OR_PR_TITLE","merge_commit_message":"PR_TITLE","merge_commit_title":"MERGE_MESSAGE"}},"base":{"label":"binwiederhier:main","ref":"main","sha":"72d931a20bb83d123ab45accaf761150c8b01211","user":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"repo":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main","allow_squash_merge":true,"allow_merge_commit":true,"allow_rebase_merge":true,"allow_auto_merge":false,"delete_branch_on_merge":false,"allow_update_branch":false,"use_squash_pr_title_as_default":false,"squash_merge_commit_message":"COMMIT_MESSAGES","squash_merge_commit_title":"COMMIT_OR_PR_TITLE","merge_commit_message":"PR_TITLE","merge_commit_title":"MERGE_MESSAGE"}},"_links":{"self":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1"},"html":{"href":"https://github.com/binwiederhier/dabble/pull/1"},"issue":{"href":"https://api.github.com/repos/binwiederhier/dabble/issues/1"},"comments":{"href":"https://api.github.com/repos/binwiederhier/dabble/issues/1/comments"},"review_comments":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/comments"},"review_comment":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/comments{/number}"},"commits":{"href":"https://api.github.com/repos/binwiederhier/dabble/pulls/1/commits"},"statuses":{"href":"https://api.github.com/repos/binwiederhier/dabble/statuses/5703842cc5715ed1e358d23ebb693db09747ae9b"}},"author_association":"OWNER","auto_merge":null,"active_lock_reason":null,"merged":false,"mergeable":null,"rebaseable":null,"mergeable_state":"unknown","merged_by":null,"comments":0,"review_comments":0,"maintainer_can_modify":false,"commits":1,"additions":1,"deletions":1,"changed_files":1},"repository":{"id":470212003,"node_id":"R_kgDOHAbdow","name":"dabble","full_name":"binwiederhier/dabble","private":false,"owner":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false},"html_url":"https://github.com/binwiederhier/dabble","description":"A repo for dabbling","fork":false,"url":"https://api.github.com/repos/binwiederhier/dabble","forks_url":"https://api.github.com/repos/binwiederhier/dabble/forks","keys_url":"https://api.github.com/repos/binwiederhier/dabble/keys{/key_id}","collaborators_url":"https://api.github.com/repos/binwiederhier/dabble/collaborators{/collaborator}","teams_url":"https://api.github.com/repos/binwiederhier/dabble/teams","hooks_url":"https://api.github.com/repos/binwiederhier/dabble/hooks","issue_events_url":"https://api.github.com/repos/binwiederhier/dabble/issues/events{/number}","events_url":"https://api.github.com/repos/binwiederhier/dabble/events","assignees_url":"https://api.github.com/repos/binwiederhier/dabble/assignees{/user}","branches_url":"https://api.github.com/repos/binwiederhier/dabble/branches{/branch}","tags_url":"https://api.github.com/repos/binwiederhier/dabble/tags","blobs_url":"https://api.github.com/repos/binwiederhier/dabble/git/blobs{/sha}","git_tags_url":"https://api.github.com/repos/binwiederhier/dabble/git/tags{/sha}","git_refs_url":"https://api.github.com/repos/binwiederhier/dabble/git/refs{/sha}","trees_url":"https://api.github.com/repos/binwiederhier/dabble/git/trees{/sha}","statuses_url":"https://api.github.com/repos/binwiederhier/dabble/statuses/{sha}","languages_url":"https://api.github.com/repos/binwiederhier/dabble/languages","stargazers_url":"https://api.github.com/repos/binwiederhier/dabble/stargazers","contributors_url":"https://api.github.com/repos/binwiederhier/dabble/contributors","subscribers_url":"https://api.github.com/repos/binwiederhier/dabble/subscribers","subscription_url":"https://api.github.com/repos/binwiederhier/dabble/subscription","commits_url":"https://api.github.com/repos/binwiederhier/dabble/commits{/sha}","git_commits_url":"https://api.github.com/repos/binwiederhier/dabble/git/commits{/sha}","comments_url":"https://api.github.com/repos/binwiederhier/dabble/comments{/number}","issue_comment_url":"https://api.github.com/repos/binwiederhier/dabble/issues/comments{/number}","contents_url":"https://api.github.com/repos/binwiederhier/dabble/contents/{+path}","compare_url":"https://api.github.com/repos/binwiederhier/dabble/compare/{base}...{head}","merges_url":"https://api.github.com/repos/binwiederhier/dabble/merges","archive_url":"https://api.github.com/repos/binwiederhier/dabble/{archive_format}{/ref}","downloads_url":"https://api.github.com/repos/binwiederhier/dabble/downloads","issues_url":"https://api.github.com/repos/binwiederhier/dabble/issues{/number}","pulls_url":"https://api.github.com/repos/binwiederhier/dabble/pulls{/number}","milestones_url":"https://api.github.com/repos/binwiederhier/dabble/milestones{/number}","notifications_url":"https://api.github.com/repos/binwiederhier/dabble/notifications{?since,all,participating}","labels_url":"https://api.github.com/repos/binwiederhier/dabble/labels{/name}","releases_url":"https://api.github.com/repos/binwiederhier/dabble/releases{/id}","deployments_url":"https://api.github.com/repos/binwiederhier/dabble/deployments","created_at":"2022-03-15T15:06:17Z","updated_at":"2022-03-15T15:06:17Z","pushed_at":"2024-03-21T02:52:10Z","git_url":"git://github.com/binwiederhier/dabble.git","ssh_url":"git@github.com:binwiederhier/dabble.git","clone_url":"https://github.com/binwiederhier/dabble.git","svn_url":"https://github.com/binwiederhier/dabble","homepage":null,"size":1,"stargazers_count":0,"watchers_count":0,"language":null,"has_issues":true,"has_projects":true,"has_downloads":true,"has_wiki":true,"has_pages":false,"has_discussions":false,"forks_count":0,"mirror_url":null,"archived":false,"disabled":false,"open_issues_count":1,"license":null,"allow_forking":true,"is_template":false,"web_commit_signoff_required":false,"topics":[],"visibility":"public","forks":0,"open_issues":1,"watchers":0,"default_branch":"main"},"sender":{"login":"binwiederhier","id":664597,"node_id":"MDQ6VXNlcjY2NDU5Nw==","avatar_url":"https://avatars.githubusercontent.com/u/664597?v=4","gravatar_id":"","url":"https://api.github.com/users/binwiederhier","html_url":"https://github.com/binwiederhier","followers_url":"https://api.github.com/users/binwiederhier/followers","following_url":"https://api.github.com/users/binwiederhier/following{/other_user}","gists_url":"https://api.github.com/users/binwiederhier/gists{/gist_id}","starred_url":"https://api.github.com/users/binwiederhier/starred{/owner}{/repo}","subscriptions_url":"https://api.github.com/users/binwiederhier/subscriptions","organizations_url":"https://api.github.com/users/binwiederhier/orgs","repos_url":"https://api.github.com/users/binwiederhier/repos","events_url":"https://api.github.com/users/binwiederhier/events{/privacy}","received_events_url":"https://api.github.com/users/binwiederhier/received_events","type":"User","site_admin":false}}`
	response := request(t, s, "PUT", `/mytopic?tpl=yes&title={{if+eq+.action+"opened"}}New+PR:+%23{{.number}}+by+{{.pull_request.user.login}}{{else}}[{{.action}}]+PR:+%23{{.number}}+by+{{.pull_request.user.login}}{{end}}&message={{.pull_request.title}}+in+{{.repository.full_name}}.+View+more+at+{{.pull_request.html_url}}`, body, nil)
	require.Equal(t, 200, response.Code)
	m := toMessage(t, response.Body.String())
	require.Equal(t, `New PR: #1 by binwiederhier`, m.Title)
	require.Equal(t, `A sample PR from Phil in binwiederhier/dabble. View more at https://github.com/binwiederhier/dabble/pull/1`, m.Message)
}

func TestServer_MessageTemplate_DisallowedCalls(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfig(t))
	disallowedTemplates := []string{
		`{{template ""}}`,
		`{{- template ""}}`,
		`{{-
template ""}}`,
		`{{      call abc}}`,
		`{{      define "aa"}}`,
		`We cannot {{define "aa"}}`,
		`We cannot {{ call "aa"}}`,
		`We cannot {{- template "aa"}}`,
	}
	for _, disallowedTemplate := range disallowedTemplates {
		messageTemplate := disallowedTemplate
		t.Run(disallowedTemplate, func(t *testing.T) {
			t.Parallel()
			response := request(t, s, "PUT", `/mytopic`, `{}`, map[string]string{
				"Template": "yes",
				"Message":  messageTemplate,
			})
			require.Equal(t, 400, response.Code)
			require.Equal(t, 40044, toHTTPError(t, response.Body.String()).Code)
		})
	}
}

func TestServer_MessageTemplate_Server(t *testing.T) {
	s := newTestServer(t, newTestConfigWithTemplates(t))
	// keep this in sync with the mock templates
	// generated by newTestConfigWithTemplates
	body := `{"foo":"bar", "nested":{"title":"here"}}`

	var testCases = map[string]struct {
		haveTitle   string
		haveMessage string
		wantCode    int
		wantTitle   string
		wantMessage string
	}{
		"empty title": {
			haveMessage: "foo_message",
			wantCode:    200,
			wantMessage: "bar",
		},
		"invalid title template": {
			haveTitle: "does_not_exist",
			wantCode:  400,
		},
		"invalid message template": {
			haveMessage: "does_not_exist",
			wantCode:    400,
		},
		"simple templates": {
			haveTitle:   "nested_title",
			haveMessage: "foo_message",
			wantCode:    200,
			wantTitle:   "here",
			wantMessage: "bar",
		},
		"repeat templates": {
			haveTitle:   "nested_repeat",
			haveMessage: "foo_repeat",
			wantCode:    200,
			wantTitle:   "here is here",
			wantMessage: "bar is bar",
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			endpoint := "/mytopic?tpl=server&title=" + test.haveTitle + "&message=" + test.haveMessage
			response := request(t, s, "PUT", endpoint, body, nil)
			require.Equal(t, test.wantCode, response.Code)

			if test.wantCode == 200 {
				m := toMessage(t, response.Body.String())
				require.Equal(t, test.wantTitle, m.Title, "message title")
				require.Equal(t, test.wantMessage, m.Message, "message body")
			}
		})
	}
}

func newTestConfig(t *testing.T) *Config {
	conf := NewConfig()
	conf.BaseURL = "http://127.0.0.1:12345"
	conf.CacheFile = filepath.Join(t.TempDir(), "cache.db")
	conf.CacheStartupQueries = "pragma journal_mode = WAL; pragma synchronous = normal; pragma temp_store = memory;"
	conf.AttachmentCacheDir = t.TempDir()
	return conf
}

func configureAuth(t *testing.T, conf *Config) *Config {
	conf.AuthFile = filepath.Join(t.TempDir(), "user.db")
	conf.AuthStartupQueries = "pragma journal_mode = WAL; pragma synchronous = normal; pragma temp_store = memory;"
	conf.AuthBcryptCost = bcrypt.MinCost // This speeds up tests a lot
	return conf
}

func newTestConfigWithAuthFile(t *testing.T) *Config {
	conf := newTestConfig(t)
	conf = configureAuth(t, conf)
	return conf
}

func newTestConfigWithWebPush(t *testing.T) *Config {
	conf := newTestConfig(t)
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	require.Nil(t, err)
	conf.WebPushFile = filepath.Join(t.TempDir(), "webpush.db")
	conf.WebPushEmailAddress = "testing@example.com"
	conf.WebPushPrivateKey = privateKey
	conf.WebPushPublicKey = publicKey
	return conf
}

func newTestConfigWithTemplates(t *testing.T) *Config {
	basedir, err := os.MkdirTemp(t.TempDir(), "templates")
	require.Nil(t, err)

	metadataFile1 := filepath.Join(basedir, "metadata.json")
	templateFile1 := filepath.Join(basedir, "foo_message"+templateExtension)
	templateFile2 := filepath.Join(basedir, "nested_title"+templateExtension)
	templateFile3 := filepath.Join(basedir, "foo_repeat"+templateExtension)
	templateFile4 := filepath.Join(basedir, "nested_repeat"+templateExtension)
	metadataData1 := []byte("{}")
	templateData1 := []byte("{{.foo}}")
	templateData2 := []byte("{{.nested.title}}")
	templateData3 := []byte("{{.foo}} is {{.foo}}")
	templateData4 := []byte("{{.nested.title}} is {{.nested.title}}")

	conf := newTestConfig(t)
	conf.TemplateDirectory = basedir

	err = os.WriteFile(metadataFile1, metadataData1, 0644)
	require.Nil(t, err)

	err = os.WriteFile(templateFile1, templateData1, 0644)
	require.Nil(t, err)

	err = os.WriteFile(templateFile2, templateData2, 0644)
	require.Nil(t, err)

	err = os.WriteFile(templateFile3, templateData3, 0644)
	require.Nil(t, err)

	err = os.WriteFile(templateFile4, templateData4, 0644)
	require.Nil(t, err)

	return conf
}

func newTestServer(t *testing.T, config *Config) *Server {
	server, err := New(config)
	require.Nil(t, err)
	return server
}

func request(t *testing.T, s *Server, method, url, body string, headers map[string]string, fn ...func(r *http.Request)) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	r, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.RemoteAddr = "9.9.9.9" // Used for tests
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	for _, f := range fn {
		f(r)
	}
	s.handle(rr, r)
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
		time.Sleep(200 * time.Millisecond)
		cancel()
		<-done
	}
	time.Sleep(200 * time.Millisecond)
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

func readAll(t *testing.T, rc io.ReadCloser) string {
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func waitFor(t *testing.T, f func() bool) {
	waitForWithMaxWait(t, 5*time.Second, f)
}

func waitForWithMaxWait(t *testing.T, maxWait time.Duration, f func() bool) {
	start := time.Now()
	for time.Since(start) < maxWait {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Function f did not succeed after %v: %v", maxWait, string(debug.Stack()))
}
