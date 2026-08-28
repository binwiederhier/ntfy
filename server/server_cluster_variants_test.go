package server

// Cluster variants of the single-node feature tests: each test takes an existing behavior
// (publish/subscribe fidelity, filters, polling, delayed messages, message actions, sequence
// IDs, attachments, auth, UnifiedPush, Matrix, upstream forwarding) and asserts that it holds
// when the publisher and the subscriber (or the actor and the observer) sit on DIFFERENT
// cluster nodes. All tests share the newTestCluster harness: full servers on one Postgres
// schema with real, mutually reachable fan-out listeners.

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

// newTestCluster starts n full servers sharing one Postgres schema, each serving its real
// cluster listener, so fan-out flows in every direction. configure may adjust each node's
// config before start (same index order as the returned slice).
func newTestCluster(t *testing.T, n int, configure func(i int, conf *Config)) []*Server {
	t.Helper()
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	listeners := make([]net.Listener, n)
	for i := range listeners {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.Nil(t, err)
		listeners[i] = listener
	}
	servers := make([]*Server, n)
	for i := range servers {
		conf := newTestConfig(t, schemaDSN)
		conf.ClusterNodeID = fmt.Sprintf("node-%d", i+1)
		conf.ClusterListen = listeners[i].Addr().String()
		conf.ClusterSecret = "s3cret"
		conf.ClusterAdvertiseURL = "http://" + listeners[i].Addr().String()
		conf.ClusterBatchLinger = 0 // Deliver immediately; the variants assert timely arrival
		conf.VisitorUsageFlushInterval = 150 * time.Millisecond
		if configure != nil {
			configure(i, conf)
		}
		s := newTestServer(t, conf)
		srv := &http.Server{Handler: s.clusterHandler()}
		listener := listeners[i]
		go srv.Serve(listener)
		t.Cleanup(func() { srv.Close() })
		servers[i] = s
	}
	// Let one heartbeat cycle pass: nodes created first primed their peer cache before the
	// later ones registered, and the cache only refreshes on the (3s) heartbeat tick
	time.Sleep(4 * time.Second)
	return servers
}

func TestServer_ClusterVariant_PublishSubscribe_FullMessageFidelity(t *testing.T) {
	// Variant of TestServer_PublishAndSubscribe/PublishPriority/PublishActions/PublishMarkdown:
	// every publisher-controlled field must survive the node-to-node hop unchanged.
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json", subscribeRR)

	response := request(t, sA, "PUT", "/mytopic", "cluster message with ünïcode", map[string]string{
		"Title":    "a title",
		"X-Tags":   "tag1,tag 2",
		"Priority": "high",
		"Click":    "https://example.com/click",
		"Actions":  "view, Open, https://example.com/action",
		"Markdown": "yes",
		"X-Icon":   "https://example.com/icon.png",
	})
	require.Equal(t, 200, response.Code)
	published := toMessage(t, response.Body.String())

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages)) // open + message
	m := messages[1]
	require.Equal(t, published.ID, m.ID)
	require.Equal(t, "cluster message with ünïcode", m.Message)
	require.Equal(t, "a title", m.Title)
	require.Equal(t, []string{"tag1", "tag 2"}, m.Tags)
	require.Equal(t, 4, m.Priority)
	require.Equal(t, "https://example.com/click", m.Click)
	require.Equal(t, "https://example.com/icon.png", m.Icon)
	require.Equal(t, "text/markdown", m.ContentType)
	require.Equal(t, 1, len(m.Actions))
	require.Equal(t, "view", m.Actions[0].Action)
	require.Equal(t, "Open", m.Actions[0].Label)
}

func TestServer_ClusterVariant_SubscribeWithQueryFilters(t *testing.T) {
	// Variant of TestServer_SubscribeWithQueryFilters: filters are evaluated on the
	// subscriber's node, so they must apply to peer-delivered messages too
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json?priority=high&tags=zfs-error", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "no match", nil).Code)
	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "match", map[string]string{
		"Priority": "high",
		"Tags":     "zfs-error,zfs",
	}).Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages)) // open + the one matching message
	require.Equal(t, "match", messages[1].Message)
}

func TestServer_ClusterVariant_PollFromPeerNode(t *testing.T) {
	// Variant of TestServer_PublishAndPoll and TestServer_PublishAndPollSince: the cache is
	// shared, so polling any node returns messages published on any other node, and since=<id>
	// markers from one node work on another
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "first", nil).Code)
	response := request(t, sA, "PUT", "/mytopic", "second", nil)
	require.Equal(t, 200, response.Code)
	second := toMessage(t, response.Body.String())

	messages := toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "first", messages[0].Message)
	require.Equal(t, "second", messages[1].Message)

	// since=<id of first> on the OTHER node returns only the second message
	messages = toMessages(t, request(t, sB, "GET", fmt.Sprintf("/mytopic/json?poll=1&since=%s", messages[0].ID), "", nil).Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, second.ID, messages[0].ID)
}

func TestServer_ClusterVariant_PublishNoCache(t *testing.T) {
	// Variant of TestServer_PublishNoCache: Cache:no messages are still delivered to live
	// subscribers on other nodes (fan-out is independent of the cache), but polls on any node
	// come up empty
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "ephemeral", map[string]string{"Cache": "no"}).Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "ephemeral", messages[1].Message)

	require.Empty(t, toMessages(t, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Body.String()))
	require.Empty(t, toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String()))
}

func TestServer_ClusterVariant_DelayedMessage_ClaimedByPeer(t *testing.T) {
	// Variant of TestServer_PublishAt: a delayed message published on node A can be claimed
	// and dispatched by node B's delayed sender (SKIP LOCKED), and must still reach a
	// subscriber on node A -- the claiming node acts as the origin
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sA, "/mytopic/json", subscribeRR)

	response := request(t, sA, "PUT", "/mytopic", "delayed hello", map[string]string{"In": "1h"})
	require.Equal(t, 200, response.Code)
	msg := toMessage(t, response.Body.String())

	// Not delivered yet
	require.Empty(t, toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String()))

	// Backdate and let NODE B claim and deliver it
	require.Nil(t, sA.messageCache.UpdateMessageTime(msg.ID, time.Now().Add(-10*time.Second).Unix()))
	require.Nil(t, sB.sendDelayedMessages())

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, "delayed hello", messages[1].Message)

	// And node A's own delayed sender finds nothing left to claim (exactly once)
	require.Nil(t, sA.sendDelayedMessages())
	polled := toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
	require.Equal(t, 1, len(polled))
}

func TestServer_ClusterVariant_UpdateScheduledMessage_ViaPeer(t *testing.T) {
	// Variant of TestServer_UpdateScheduledMessage: a scheduled message published on node A is
	// replaced via its sequence ID on node B before it is due; only the updated content fires
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	response := request(t, sA, "PUT", "/mytopic/sched-seq", "original scheduled", map[string]string{"In": "1h"})
	require.Equal(t, 200, response.Code)

	// Replace it on the OTHER node (same sequence ID, still scheduled)
	response = request(t, sB, "PUT", "/mytopic/sched-seq", "updated scheduled", map[string]string{"In": "1h"})
	require.Equal(t, 200, response.Code)
	updated := toMessage(t, response.Body.String())

	require.Nil(t, sA.messageCache.UpdateMessageTime(updated.ID, time.Now().Add(-10*time.Second).Unix()))
	require.Nil(t, sA.sendDelayedMessages())
	require.Nil(t, sB.sendDelayedMessages())

	messages := toMessages(t, request(t, sA, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "updated scheduled", messages[0].Message)
	require.Equal(t, "sched-seq", messages[0].SequenceID)
}

func TestServer_ClusterVariant_DeleteMessage_EventReachesPeerSubscriber(t *testing.T) {
	// Variant of TestServer_DeleteMessage: deleting a message by sequence ID on one node emits
	// a message_delete event that must reach subscribers on other nodes (they need it to
	// remove the notification), and the deletion is visible in polls everywhere
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic/seq123", "to be deleted", nil).Code)
	require.Equal(t, 200, request(t, sA, "DELETE", "/mytopic/seq123", "", nil).Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 3, len(messages)) // open + message + message_delete
	require.Equal(t, "message", messages[1].Event)
	require.Equal(t, "message_delete", messages[2].Event)
	require.Equal(t, "seq123", messages[2].SequenceID)

	// The peer node's poll sees the same two events from the shared cache
	polled := toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
	require.Equal(t, 2, len(polled))
	require.Equal(t, "message_delete", polled[1].Event)
}

func TestServer_ClusterVariant_UpdateMessage_AcrossNodes(t *testing.T) {
	// Variant of TestServer_UpdateMessage: publish a sequence ID on node A, update it on node
	// B; a subscriber on node A sees both versions live, and polls agree on both nodes
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sA, "/mytopic/json", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic/update-seq", "original", nil).Code)
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic/update-seq", "updated", nil).Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 3, len(messages)) // open + original (local) + updated (from peer)
	require.Equal(t, "original", messages[1].Message)
	require.Equal(t, "updated", messages[2].Message)
	require.Equal(t, "update-seq", messages[2].SequenceID)

	for _, s := range []*Server{sA, sB} {
		polled := toMessages(t, request(t, s, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
		require.Equal(t, 2, len(polled))
		require.Equal(t, "updated", polled[1].Message)
	}
}

func TestServer_ClusterVariant_UnifiedPushBinary(t *testing.T) {
	// Variant of TestServer_PublishUnifiedPushBinary_AndPoll: base64-encoded binary UP
	// messages must survive the node hop byte-for-byte (encoding travels in the envelope)
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/up123456789012/json", subscribeRR)

	body := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}
	response := request(t, sA, "PUT", "/up123456789012?up=1", string(body), nil)
	require.Equal(t, 200, response.Code)

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages))
	m := messages[1]
	require.Equal(t, "base64", m.Encoding)
	require.Equal(t, "AAEC//79", m.Message)
}

func TestServer_ClusterVariant_Attachment_UploadAndDownloadOnPeer(t *testing.T) {
	// Variant of TestServer_PublishAttachment: with shared attachment storage (S3 in prod), an
	// attachment uploaded via node A must be pollable and downloadable via node B
	sharedAttachmentDir := t.TempDir()
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.AttachmentCacheDir = sharedAttachmentDir
	})
	sA, sB := cluster[0], cluster[1]

	content := strings.Repeat("this is an attachment. ", 1000) // > message size limit -> stored as attachment
	response := request(t, sA, "PUT", "/mytopic", content, nil)
	require.Equal(t, 200, response.Code)
	published := toMessage(t, response.Body.String())
	require.NotNil(t, published.Attachment)
	require.Equal(t, "attachment.txt", published.Attachment.Name)

	// Poll from the peer node; the attachment metadata comes from the shared cache
	messages := toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1", "", nil).Body.String())
	require.Equal(t, 1, len(messages))
	require.NotNil(t, messages[0].Attachment)

	// Download through the peer node (path only; the URL host points at the origin's base-url)
	path := strings.TrimPrefix(messages[0].Attachment.URL, sA.config.BaseURL)
	download := request(t, sB, "GET", path, "", nil)
	require.Equal(t, 200, download.Code)
	require.Equal(t, content, download.Body.String())
}

func TestServer_ClusterVariant_Auth_TokenAndACLImmediatelyValidOnPeer(t *testing.T) {
	// Variant of the TestServer_Auth_* family: users, tokens, and ACL entries live in the
	// shared database, so credentials created via one node must work on another right away
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.AuthDefault = user.PermissionDenyAll
		conf.AuthBcryptCost = 4
		conf.EnableLogin = true
	})
	sA, sB := cluster[0], cluster[1]

	require.Nil(t, sA.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	require.Nil(t, sA.userManager.AllowAccess("phil", "mytopic", user.PermissionReadWrite))

	// Publish on node B with basic auth created via node A
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "authed", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil12345"),
	}).Code)
	// Anonymous publish on node B is still denied
	require.Equal(t, 403, request(t, sB, "PUT", "/mytopic", "anon", nil).Code)

	// A token created via node A works on node B immediately (tokens are never cached)
	u, err := sA.userManager.User("phil")
	require.Nil(t, err)
	token, err := sA.userManager.CreateToken(u.ID, "cli", time.Now().Add(time.Hour), netip.MustParseAddr("1.2.3.4"), false)
	require.Nil(t, err)
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "token-authed", map[string]string{
		"Authorization": "Bearer " + token.Value,
	}).Code)

	// Revoking access via node B takes effect on node A (no ACL cache in this config)
	require.Nil(t, sB.userManager.ResetAccess("phil", "mytopic"))
	require.Equal(t, 403, request(t, sA, "PUT", "/mytopic", "revoked", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil12345"),
	}).Code)
}

func TestServer_ClusterVariant_Matrix_PushOnOneNode_SubscriberOnAnother(t *testing.T) {
	// Variant of TestServer_MatrixGateway_Push_Success: a Matrix push accepted by node A must
	// reach the UnifiedPush subscriber connected to node B
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.BaseURL = "http://127.0.0.1:12345"
	})
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/up123456789012/json", subscribeRR)
	time.Sleep(300 * time.Millisecond) // Let the first-subscriber announcement reach node A

	notification := `{"notification":{"devices":[{"pushkey":"http://127.0.0.1:12345/up123456789012?up=1"}]}}`
	response := request(t, sA, "POST", "/_matrix/push/v1/notify", notification, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, `{"rejected":[]}`+"\n", response.Body.String())

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages))
	require.Equal(t, notification, messages[1].Message)
}

func TestServer_ClusterVariant_Upstream_ForwardedOnceByOriginOnly(t *testing.T) {
	// Variant of TestServer_UpstreamBaseURL_Success: with upstream-base-url set on every node
	// (as it would be in a real deployment), a publish must produce exactly ONE upstream poll
	// request (from the origin node), not one per node
	var upstreamRequests atomic.Int32
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		upstreamRequests.Add(1)
	}))
	defer upstreamServer.Close()

	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.BaseURL = "http://myserver.internal"
		conf.UpstreamBaseURL = upstreamServer.URL
	})
	sA, sB := cluster[0], cluster[1]

	// Subscriber on B guarantees the message actually crosses the node boundary
	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "hello upstream", nil).Code)

	waitFor(t, func() bool { return upstreamRequests.Load() >= 1 })
	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	require.Equal(t, 2, len(messages)) // Message did reach node B ...
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, int32(1), upstreamRequests.Load()) // ... but only the origin forwarded it
}

func TestServer_ClusterVariant_CancelScheduled_ViaPeer(t *testing.T) {
	// Variant of TestServer_DeleteScheduledMessage: canceling a scheduled message by sequence
	// ID on another node must prevent delivery on every node and emit message_delete
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sA, "/mytopic/json", subscribeRR)

	response := request(t, sA, "PUT", "/mytopic/cancel-seq", "never sent", map[string]string{"In": "1h"})
	require.Equal(t, 200, response.Code)
	msg := toMessage(t, response.Body.String())

	// Cancel via the OTHER node before it is due
	require.Equal(t, 200, request(t, sB, "DELETE", "/mytopic/cancel-seq", "", nil).Code)

	// Even if it were due now, no node delivers it
	_ = sA.messageCache.UpdateMessageTime(msg.ID, time.Now().Add(-10*time.Second).Unix())
	require.Nil(t, sA.sendDelayedMessages())
	require.Nil(t, sB.sendDelayedMessages())

	subscribeCancel()
	messages := toMessages(t, subscribeRR.Body.String())
	for _, m := range messages {
		require.NotEqual(t, "never sent", m.Message)
	}
	require.Equal(t, "message_delete", messages[len(messages)-1].Event)
}

func TestServer_ClusterVariant_SSEStream_FromPeerNode(t *testing.T) {
	// Variant of the SSE subscription: the /sse serializer sits on the same delivery seam, but
	// pin it anyway -- Android/web fallbacks rely on it
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/sse", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "sse hello", nil).Code)

	subscribeCancel()
	body := subscribeRR.Body.String()
	require.Contains(t, body, "event: open")
	require.Contains(t, body, "sse hello")
}

func TestServer_ClusterVariant_PollModes_OnPeer(t *testing.T) {
	// Variants of "fetch latest message", "fetch scheduled messages", and the iOS NSE's
	// fetch-by-ID (GET /topic/json?poll=1&id=<mid>): all read the shared cache, so they must
	// work on any node regardless of where the message was published
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "older", nil).Code)
	response := request(t, sA, "PUT", "/mytopic", "newest", nil)
	require.Equal(t, 200, response.Code)
	newest := toMessage(t, response.Body.String())
	response = request(t, sA, "PUT", "/mytopic", "scheduled", map[string]string{"In": "1h"})
	require.Equal(t, 200, response.Code)
	scheduled := toMessage(t, response.Body.String())

	// since=latest on the peer returns only the newest (non-scheduled) message
	messages := toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1&since=latest", "", nil).Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, newest.ID, messages[0].ID)

	// sched=1 on the peer includes the scheduled message
	messages = toMessages(t, request(t, sB, "GET", "/mytopic/json?poll=1&sched=1", "", nil).Body.String())
	ids := make([]string, 0)
	for _, m := range messages {
		ids = append(ids, m.ID)
	}
	require.Contains(t, ids, scheduled.ID)

	// Fetch-by-ID on the peer (iOS notification service extension pattern)
	messages = toMessages(t, request(t, sB, "GET", fmt.Sprintf("/mytopic/json?poll=1&id=%s", newest.ID), "", nil).Body.String())
	require.Equal(t, 1, len(messages))
	require.Equal(t, "newest", messages[0].Message)
}

func TestServer_ClusterVariant_TopicAuthEndpointAndQueryParamAuth(t *testing.T) {
	// Variants of the Android read-access probe (GET /<topic>/auth) and query-param auth
	// (?auth=<base64>, used where headers are impossible, e.g. WebSocket/EventSource):
	// credentials and grants live in the shared database and must work via any node
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.AuthDefault = user.PermissionDenyAll
		conf.AuthBcryptCost = 4
	})
	sA, sB := cluster[0], cluster[1]

	require.Nil(t, sA.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	require.Nil(t, sA.userManager.AllowAccess("phil", "mytopic", user.PermissionReadWrite))

	// Android subscription probe against the peer node
	require.Equal(t, 200, request(t, sB, "GET", "/mytopic/auth", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil12345"),
	}).Code)
	require.Equal(t, 403, request(t, sB, "GET", "/mytopic/auth", "", nil).Code)

	// Query-param auth against the peer node
	authParam := base64.RawURLEncoding.EncodeToString([]byte(util.BasicAuth("phil", "phil12345")))
	require.Equal(t, 200, request(t, sB, "GET", "/mytopic/json?poll=1&auth="+authParam, "", nil).Code)
}

func TestServer_ClusterVariant_WebPush_SinglePushFromOrigin(t *testing.T) {
	// Variant of TestServer_WebPush_Publish: web push subscriptions live in the shared
	// database, but only the ORIGIN node sends the push -- a peer receiving the message via
	// fan-out must not send a second one (double notifications on real devices)
	var pushes atomic.Int32
	pushService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		pushes.Add(1)
	}))
	defer pushService.Close()

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	require.Nil(t, err)
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.WebPushEmailAddress = "testing@example.com"
		conf.WebPushPrivateKey = privateKey
		conf.WebPushPublicKey = publicKey
	})
	sA, sB := cluster[0], cluster[1]

	// Subscription registered via node B; live subscriber on B forces the cross-node hop
	require.Nil(t, sB.webPush.UpsertSubscription(pushService.URL+"/push-receive", "kSC3T8aN1JCQxxPdrFLrZg", "BMKKbxdUU_xLS7G1Wh5AN8PvWOjCzkCuKZYb8apcqYrDxjOF_2piggBnoJLQYx9IeSD70fNuwawI3e9Y8m3S3PE", "u_123", netip.MustParseAddr("1.2.3.4"), []string{"mytopic"}))
	subscribeRR := httptest.NewRecorder()
	subscribeCancel := subscribe(t, sB, "/mytopic/json", subscribeRR)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "push once", nil).Code)

	waitFor(t, func() bool { return pushes.Load() >= 1 })
	subscribeCancel()
	require.Equal(t, 2, len(toMessages(t, subscribeRR.Body.String()))) // Message crossed nodes
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, int32(1), pushes.Load()) // ... but exactly one web push was sent
}

func TestServer_ClusterVariant_TierChange_PickedUpOnPeer(t *testing.T) {
	// Tier changes are written to the shared database via one node; other nodes re-read the
	// user per request and rebuild limiters when the tier changed, so new limits apply on the
	// peer within one request
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.AuthDefault = user.PermissionReadWrite
		conf.AuthBcryptCost = 4
		conf.VisitorRequestLimitBurst = 100
	})
	sA, sB := cluster[0], cluster[1]

	require.Nil(t, sA.userManager.AddTier(&user.Tier{Code: "tiny", MessageLimit: 2}))
	require.Nil(t, sA.userManager.AddTier(&user.Tier{Code: "big", MessageLimit: 100}))
	require.Nil(t, sA.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	require.Nil(t, sA.userManager.ChangeTier("phil", "tiny"))
	auth := map[string]string{"Authorization": util.BasicAuth("phil", "phil12345")}

	// Exhaust the tiny tier on node B
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "1", auth).Code)
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "2", auth).Code)
	require.Equal(t, 429, request(t, sB, "PUT", "/mytopic", "3", auth).Code)

	// Upgrade via node A; node B honors the new tier on the next request
	require.Nil(t, sA.userManager.ChangeTier("phil", "big"))
	require.Equal(t, 200, request(t, sB, "PUT", "/mytopic", "4", auth).Code)
}

func TestServer_ClusterVariant_WebSocket_SubscribeOnPeer(t *testing.T) {
	// Variant of the Android app's primary transport: a real WebSocket subscription
	// (GET /<topic>/ws?since=all) on node B receives a message published on node A.
	// There is no single-node WebSocket-client test to mirror; this doubles as one.
	cluster := newTestCluster(t, 2, nil)
	sA, sB := cluster[0], cluster[1]

	publicB := httptest.NewServer(http.HandlerFunc(sB.handle))
	defer publicB.Close()

	wsURL := "ws" + strings.TrimPrefix(publicB.URL, "http") + "/mytopic/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Nil(t, err)
	defer conn.Close()

	// First frame is the open event
	_, frame, err := conn.ReadMessage()
	require.Nil(t, err)
	require.Equal(t, "open", toMessage(t, string(frame)).Event)

	require.Equal(t, 200, request(t, sA, "PUT", "/mytopic", "over the wire", map[string]string{"Title": "ws title"}).Code)

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, frame, err = conn.ReadMessage()
	require.Nil(t, err)
	m := toMessage(t, string(frame))
	require.Equal(t, "message", m.Event)
	require.Equal(t, "over the wire", m.Message)
	require.Equal(t, "ws title", m.Title)
}

func TestServer_ClusterVariant_StopCompletesWithBlockedUsageFlush(t *testing.T) {
	// Regression: Stop used to hold s.mu while waiting for the usage tracker's flush loop; a
	// flush tick blocked in applyPeerUsage (which needs s.mu) then deadlocked shutdown (seen
	// as the full test suite hanging). Stop now closes the tracker before taking the lock, so
	// it completes as soon as whoever holds s.mu releases it.
	cluster := newTestCluster(t, 2, func(i int, conf *Config) {
		conf.VisitorUsageFlushInterval = 50 * time.Millisecond
	})
	sA, sB := cluster[0], cluster[1]

	// Continuous publishes on B ensure A's pulls keep reporting peer usage (each pull calls
	// back into applyPeerUsage on A)
	stopPublishing := make(chan struct{})
	publishingDone := make(chan struct{})
	go func() {
		defer close(publishingDone)
		for {
			select {
			case <-stopPublishing:
				return
			default:
				request(t, sB, "PUT", "/mytopic", "x", nil)
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()
	time.Sleep(400 * time.Millisecond) // A has pulled peer usage at least once

	// Hold the server lock so the next flush tick blocks inside applyPeerUsage, then stop the
	// server while the lock is still held
	sA.mu.Lock()
	time.Sleep(200 * time.Millisecond) // > flush interval: a tick is now waiting for s.mu
	stopDone := make(chan struct{})
	go func() {
		sA.Stop()
		close(stopDone)
	}()
	time.Sleep(200 * time.Millisecond) // Stop is now underway with the tick still blocked
	sA.mu.Unlock()
	select {
	case <-stopDone:
	case <-time.After(15 * time.Second):
		t.Fatal("Server.Stop deadlocked with the usage flush loop")
	}
	close(stopPublishing)
	<-publishingDone
}
