# Server architecture

How the ntfy Go server is put together: startup, the request path, the in-memory topic graph,
visitors and limits, the message cache, the database layer, the user package, the background
loops and the subscribe transports. All paths are relative to the root of this repo. The wire
format and the database schemas are in [protocol-and-data-model.md](protocol-and-data-model.md);
clustering (peer fan-out, leader election, the node registry, draining) is in
[cluster.md](cluster.md) and only touched on here.

The server is a single process with no web framework. Routing is a hand-written if/else chain in
`handleInternal`, middleware is plain closure wrapping over a custom `handleFunc` signature, and
errors are values of type `*errHTTP` that carry both an HTTP status and a stable five-digit ntfy
error code.

## Package map

| Package | Role |
|---|---|
| `main.go`, `cmd/` | CLI entry point; `cmd/serve.go` turns flags and `server.yml` into a `*server.Config` |
| `server/` | HTTP handlers, routing, live topics, per-request visitors, publish/subscribe, SMTP, Firebase, web push, Matrix |
| `model/` | `Message`, `Attachment`, `Action`, `SinceMarker`: the wire and storage types |
| `message/` | The message cache (SQLite or Postgres), and the tail that reads back what was written |
| `user/` | Users, tiers, tokens, ACLs, the access cache |
| `db/`, `db/pg/`, `db/schema/` | Primary/replica wrapper, Postgres pool, schema versioning and migrations |
| `limit/` | Rate and quota decisions per visitor key (token buckets and daily counts) |
| `visitor/` | Visitor keys (`ip:<prefix>` / `user:<id>`) and limit resolution from config or tier |
| `topic/` | Shared state of UnifiedPush topics (subscriber activity, rate visitor) |
| `ban/` | Abuse ban feed (weighted strikes per IP prefix, appended to a ban file) |
| `attachment/` | Attachment store (file system or S3) |
| `webpush/` | Web push subscription store |
| `cluster/` | Peer-to-peer fan-out and node registry; a no-op without clustering |

## 1. Startup and the request lifecycle

### main.go, cmd/serve.go, server.New

`main.go` only stamps build vars (`version`, `commit`, `date`) into the urfave/cli app and runs it.
`cmd/serve.go` defines `cmdServe`, whose action is `execServe`: read every flag into a local,
validate, build a `*server.Config`, call `server.New(conf)`, start the signal handlers, then
`s.Run()`. Config file values are merged in by the `Before:` hook
`initConfigFileInputSourceFunc("config", flagsServe, initLogFunc)`, so `server.yml` and CLI flags
share one flag set. Provisioning strings (`auth-users`, `auth-access`, `auth-tokens`) are parsed by
`parseUsers`, `parseAccess` and `parseTokens`; they produce `Provisioned: true` records that the
user manager reconciles on every boot.

On Unix, `sigHandlerShutdown` (`cmd/serve_unix.go`) handles SIGTERM/SIGINT: it calls `s.Drain(ctx)`
(a no-op unless clustered, see `server/server_drain.go`) and then `s.Stop()`. A second signal skips
the rest of the drain. SIGHUP (`sigHandlerConfigReload`) only reloads the log level.

`server.New` (`server/server.go`) does the wiring in a fixed order:

| Step | Notes |
|---|---|
| Mailer | `mail.NewSender`, nil unless `smtp-sender-addr` is set |
| Stripe | nil unless `payments.Available` and a secret key is set |
| Postgres pool | Only with `database-url`: `pg.Open` for the primary, `pg.OpenReplica` per replica, wrapped in `db.New` |
| Message cache | `createMessageCache`: Nop store (cache disabled), Postgres, SQLite file, or SQLite in memory |
| Web push store | Postgres or SQLite, only if `web-push-public-key` is set |
| Topic map | `messageCache.Topics()` pre-creates a `*topic` for every topic with cached messages |
| Message counter | `messageCache.Stats()` seeds `s.messages` |
| Attachment store | `createAttachmentStore`: `s3://` prefix selects S3, a directory the file store, else nil |
| Twilio, ban feed | `ban.NewService` only if `ban-file` is set |
| User manager | Only if `auth-file` is set or a Postgres pool exists; on SQLite, `user.OpenSQLite` opens the auth file |
| Firebase | Note the `user.Auther` nil-interface dance (issue #641) |
| Limit service | `limit.New` with a usage store on the user database (`createUsageStore`), or memory only |
| Visitor env, topic service | `newVisitorEnv`; `tp.New` with a topic store on the user database (`createTopicStore`) |
| Cluster | `cluster.New`; a no-op implementation unless `experimental-cluster-listen` is set |
| Tail | Only with a message cache (`cache-duration` > 0); starts `runTail` |
| Start | `s.limiter.Start` and `s.topicService.Start` load their state |

`Run` creates one `http.ServeMux` with a single catch-all route, `mux.HandleFunc("/", s.handle)`,
and attaches it to every public listener (HTTP, HTTPS, unix socket). Other listeners are separate:
metrics (`metrics-listen-http`), pprof (`profile-listen-http`), SMTP (`smtp-server-listen`), the
cluster peer listener (served by `s.cluster`), and the experimental admin socket (see below). All
listeners push their terminal error into one `errChan`, so `Run` returns on the first failure. The
background loops `runManager`, `runDelayedSender` and `runFirebaseKeepaliver` start last.

`Stop` is idempotent (`stopOnce`) and bounded by `stopTimeout` (10s). `stop` closes `closeChan`
first, so the background loops end before what they use, then the listeners, the attachment
store, the tail, the cluster, the limit and topic services (which write their unflushed state),
and finally `closeDatabases`. The message cache closes first there, because on Postgres every
store shares one pool and the cache may still be writing its last batch.

### handle, handleFunc, handleInternal

The entry point is `handle` (`server/server.go`), a normal `http.HandlerFunc`:

1. `maybeAuthenticate(r)` (`server/server_auth.go`) extracts the client IP, stores it in the request
   context under `contextVisitorIP`, and returns a visitor. It **always** returns a visitor, even on
   error, so downstream logging has visitor context.
2. Logs request start at trace/debug.
3. Calls `handleInternal` inside `log.Timing(...)` and routes a returned error to `handleError`.

Everything below `handle` uses the custom signature:

```go
type handleFunc func(http.ResponseWriter, *http.Request, *visitor) error
```

Middleware are functions `func(next handleFunc) handleFunc` (`server/server_middleware.go`),
composed inline in the routing chain:

```go
return s.transformBodyJSON(s.limitRequestsWithTopic(s.authorizeTopicWrite(s.handlePublish)))(w, r, v)
```

| Middleware | Does |
|---|---|
| `limitRequests` | Takes a token from the visitor's request bucket, else 42901 (skipped for `visitor-request-limit-exempt-hosts`) |
| `limitRequestsWithTopic` | Resolves the topic from the path, picks the rate visitor (the UnifiedPush subscriber, if any), stores both in the context (`contextTopic`, `contextRateVisitor`), and charges the request to the rate visitor |
| `authorizeTopicRead` / `authorizeTopicWrite` | `userManager.Authorize` for every topic in the path, else 40301; a no-op without a user manager |
| `ensureUserManager`, `ensureUser`, `ensureAdmin` | 404 without a user manager, 401 without a user / admin |
| `ensureWebEnabled`, `ensureWebPushEnabled`, `ensureEmailsEnabled`, `ensureCallsEnabled`, `ensurePaymentsEnabled`, `ensureStripeCustomer` | Feature gates, mostly 404 when the feature is off |
| `withAccountSync` | On success, `publishSyncEventAsync` publishes to the user's sync topic so other devices refresh |
| `transformBodyJSON`, `transformMatrixJSON` | Rewrite a JSON or Matrix body into a normal publish request (see the protocol doc) |

Handlers never write error responses themselves; they return an error and `handleError` renders
it. A handler that has already written body bytes must not return a non-nil error, because
`handleError` would write headers on top; the subscribe handlers return `nil` on normal stream
termination for that reason.

### handleError and errHTTP

`handleError` (`server/server.go`):

- Type-asserts `err.(*errHTTP)`; anything else becomes `errHTTPInternalError` (50001). This is a
  plain type assertion, not `errors.As`, so an `*errHTTP` wrapped with `fmt.Errorf("%w")` degrades
  to a 500. Use the cloning combinators `Wrap`, `With`, `Fields` and `WithLimitDecision`
  (`server/errors.go`) instead of wrapping.
- Increments `metrics.HTTPRequests` with (status, ntfy code, method).
- Logs "normal" errors (`normalErrorCodes` in `server/log.go`: 400, 401, 403, 404, 413, 429, 507,
  plus `i/o timeout` and the draining 503) at debug, everything else at info.
- Sets `Retry-After` and the `RateLimit-Policy` / `RateLimit` headers if the error carries a
  `limit.Decision` (`setRateLimitHeaders`).
- For WebSocket requests, skips `WriteHeader` if the error is an `*errWebSocketPostUpgrade`: the
  connection is already hijacked, and writing headers would only produce log spam.
- If the error is rate-limiting (429/413), Stripe is configured and the user has no tier, appends
  an upsell hint via `Wrap`.
- Writes JSON with the CORS header, then feeds the ban feed via `s.ban.Record(ip, ...)` with the IP
  from the request context.

`errHTTP` (`server/errors.go`):

```go
type errHTTP struct {
    Code     int    `json:"code,omitempty"` // five-digit ntfy code
    HTTPCode int    `json:"http"`
    Message  string `json:"error"`
    Link     string `json:"link,omitempty"`
    context  log.Context     // log-only, never sent to the client
    decision *limit.Decision // behind a 429: Retry-After and RateLimit headers
}
```

The code is `<http status><two-digit serial>`, so the status is recoverable from the code alone.
The full table is the `errHTTP*` var block in `server/errors.go`.

| Range | HTTP | Examples |
|---|---|---|
| 400xx | 400 | 40001 email disabled, 40009 topic invalid, 40016 not a WebSocket upgrade, 40041 to 40048 and 40055/40056 templating, 40049 sequence ID invalid, 40061 too many topics |
| 40101 | 401 | `errHTTPUnauthorized` |
| 403xx | 403 | 40301 ACL denial, 40302 operation only allowed over the admin socket |
| 40401 | 404 | Fallthrough of `handleInternal`, and most disabled-feature gates |
| 409xx | 409 | 40901 user exists, 40905/40906 provisioned user/token change |
| 410xx | 410 | 41001 phone verification expired, 41002 abandoned UnifiedPush topic |
| 413xx | 413 | 41301 attachment too large or bandwidth exhausted, 41303 JSON body too large |
| 429xx | 429 | 42901 requests, 42903 subscriptions, 42908 daily messages, 42909 auth failures, 42911 topic creation |
| 500xx | 500 | 50001 generic, 50002 invalid path, 50003 missing base-url |
| 50301 | 503 | Node is draining (cluster) |
| 50701 | 507 | UnifiedPush publish without a prior subscriber; deliberately 5xx so app servers like Mastodon do not drop the subscription (comment in `handlePublishInternal`) |

## 2. Routing table

All routing is the if/else chain in `handleInternal` (`server/server.go`). The first match wins; the
fallthrough is `errHTTPNotFound`. Path shapes are package-level regexes and path constants at the
top of `server/server.go`.

### Publish

| Method | Path | Chain |
|---|---|---|
| PUT/POST | `/` (topic in JSON body) | `transformBodyJSON` -> `limitRequestsWithTopic` -> `authorizeTopicWrite` -> `handlePublish` |
| POST | `/_matrix/push/v1/notify` | `transformMatrixJSON` -> `limitRequestsWithTopic` -> `authorizeTopicWrite` -> `handlePublishMatrix` |
| PUT/POST | `/<topic>` or `/<topic>/<sequence-id>` | `limitRequestsWithTopic` -> `authorizeTopicWrite` -> `handlePublish` |
| DELETE `/<topic>/<sequence-id>`, GET `/<topic>/<sequence-id>/delete` | | same guards -> `handleDelete` -> `handleActionMessage(MessageDeleteEvent)` |
| GET/PUT | `/<topic>/<sequence-id>/(read\|clear)` | same guards -> `handleClear` -> `handleActionMessage(MessageClearEvent)` |
| GET | `/<topic>/(publish\|send\|trigger)` | same guards -> `handlePublish` (`publishPathRegex`) |

### Subscribe

| Method | Path | Handler (all behind `limitRequests` -> `authorizeTopicRead`) |
|---|---|---|
| GET | `/<t1,t2,...>/json` | `handleSubscribeJSON` |
| GET | `/<t1,...>/sse` | `handleSubscribeSSE` |
| GET | `/<t1,...>/raw` | `handleSubscribeRaw` |
| GET | `/<t1,...>/ws` | `handleSubscribeWS` |
| GET | `/<t1,...>/auth` | `handleTopicAuth`: `{"success":true}` if the read check passed |

### Account (`server/server_account.go`, `server/server_payments.go`)

| Method | Path | Guards | Handler |
|---|---|---|---|
| POST | `/v1/account` | `ensureUserManager` | `handleAccountCreate` |
| GET | `/v1/account` | none (anonymous allowed) | `handleAccountGet` |
| DELETE | `/v1/account` | `ensureUser`, `withAccountSync` | `handleAccountDelete` |
| POST | `/v1/account/password` | `ensureUser` | `handleAccountPasswordChange` |
| POST | `/v1/account/login` | `ensureUser`, `withAccountSync` | `handleAccountLogin` |
| POST/PATCH/DELETE | `/v1/account/token` | `ensureUser`, `withAccountSync` | `handleAccountToken{Create,Update,Delete}` |
| PATCH | `/v1/account/settings` | `ensureUser`, `withAccountSync` | `handleAccountSettingsChange` |
| POST/PATCH/DELETE | `/v1/account/subscription` | `ensureUser`, `withAccountSync` | `handleAccountSubscription{Add,Change,Delete}` |
| POST | `/v1/account/reservation` | `ensureUser`, `withAccountSync` | `handleAccountReservationAdd` |
| DELETE | `/v1/account/reservation/<topic>` | `ensureUser`, `withAccountSync` | `handleAccountReservationDelete` |
| PUT/DELETE | `/v1/account/phone`, PUT `/v1/account/phone/verify` | `ensureUser`, `ensureCallsEnabled`, `withAccountSync` | `handleAccountPhoneNumber{Add,Delete,Verify}` |
| PUT/DELETE | `/v1/account/email` | `ensureUser`, `ensureEmailsEnabled`, `withAccountSync` | `handleAccountEmail{Add,Delete}` |
| POST | `/v1/account/email/primary` | `ensureUser`, `withAccountSync` | `handleAccountEmailSetPrimary` |
| POST | `/v1/account/email/resend` | `ensureUser`, `ensureEmailsEnabled` | `handleAccountEmailResend` |
| POST | `/v1/account/email/verify` | `ensureEmailsEnabled`, `limitRequests` | `handleAccountEmailVerify`; deliberately no `ensureUser`, the link is clicked from a mail client, possibly logged out |
| POST | `/v1/account/password/reset/request`, `/v1/account/password/reset` | `ensureEmailsEnabled`, `limitRequests` | `handleAccountPasswordReset{Request,}`; unauthenticated |
| POST/PUT/DELETE | `/v1/account/billing/subscription` | `ensurePaymentsEnabled` + `ensureUser` (POST) or `ensureStripeCustomer` | `handleAccountBillingSubscription{Create,Update,Delete}` |
| GET | `/v1/account/billing/subscription/success/<id>` | `ensurePaymentsEnabled`, `ensureUserManager` | `handleAccountBillingSubscriptionCreateSuccess`; no user context |
| POST | `/v1/account/billing/portal` | `ensurePaymentsEnabled`, `ensureStripeCustomer` | `handleAccountBillingPortalSessionCreate` |
| POST | `/v1/account/billing/webhook` | `ensurePaymentsEnabled`, `ensureUserManager` | `handleAccountBillingWebhook`; the request comes from Stripe |
| GET | `/v1/account/billing/tiers` | `ensurePaymentsEnabled` | `handleBillingTiersGet` |

Billing endpoints skip `withAccountSync` because the Stripe webhook drives the sync instead.

### Admin (`server/server_admin.go`, all `ensureAdmin`)

| Method | Path | Handler |
|---|---|---|
| GET | `/v1/version` | `handleVersion` |
| GET/POST/PUT/DELETE | `/v1/users` | `handleUsers{Get,Add,Update,Delete}` |
| PUT/POST, DELETE | `/v1/users/access` | `handleAccessAllow`, `handleAccessReset` |
| GET/POST/DELETE | `/v1/users/tokens` | `handleUsersTokens{Get,Add,Delete}` |
| POST | `/v1/users/password/reset` | `handleUsersPasswordReset` |
| GET (admins), POST, PUT, DELETE | `/v1/tiers` | `handleAdminTiers{Get,Add,Update,Delete}`; a non-admin GET falls through to `handleBillingTiersGet` (compatibility for older web apps) |

The admin socket (`experimental-listen-admin-unix`, `listenAdminSocket` / `handleAdminSocket`) serves
only the paths in `adminSocketPaths` through the same `handleInternal`. Requests on it run as the
synthetic `adminSocketUser` from 127.0.0.1, bypass rate limiting and the ban feed, and carry
`contextAdminSocket`, which some handlers check to allow what the network API refuses (for
example role changes, 40302 otherwise). Whoever can open the socket file (mode 0660) is an admin.

### Health, config, misc

| Method | Path | Handler |
|---|---|---|
| GET | `/v1/health` | `handleHealth`: `{"healthy":..., "role":...}`; 503 if the cluster heartbeat went stale (always healthy single-node) |
| GET | `/v1/ready` | `handleReady`: whether a load balancer should send clients here (cluster role, draining) |
| GET | `/v1/config`, `/config.js` | `handleConfig` (JSON), `handleWebConfig` (same payload as JS) in `server/server_web.go` |
| GET | `/manifest.webmanifest` | `handleWebManifest`, gated on web push |
| GET | `/v1/stats` | `handleStats`: total messages and a publish rate derived from `messagesHistory` |
| POST/DELETE | `/v1/webpush` | `ensureWebPushEnabled`, `limitRequests` -> `handleWebPushUpdate` / `handleWebPushDelete` |
| GET | `/metrics` | `handleMetrics`, only if `enable-metrics` is set and `metrics-listen-http` is not |
| GET | `/_matrix/push/v1/notify` | `handleMatrixDiscovery` |
| GET/HEAD | `/file/<message-id>[.ext]` | `limitRequests` -> `handleFile`, only with an attachment store |
| OPTIONS | any | `limitRequests` -> `handleOptions` (CORS); works even with the web app disabled (#598) |
| HEAD | `/` | `handleEmpty` |

`handleFile` looks the message up in the cache before streaming and bills the download bandwidth
to the **uploader's** visitor (`m.User`, else `m.Sender`), which stops the one-uploader,
many-downloaders pattern and lets a paying uploader's higher limit apply. It retries the cache
lookup for up to `cache-batch-timeout`, because the web app fetches an image immediately after
upload while batched persistence may still be in flight.

### Static site and docs (go:embed)

Both trees are compiled into the binary (`server/server.go`):

```go
//go:embed site
webFs       embed.FS
webFsCached = &util.CachingEmbedFS{ModTime: time.Now(), FS: webFs}

//go:embed docs
docsStaticFs     embed.FS
docsStaticCached = &util.CachingEmbedFS{ModTime: time.Now(), FS: docsStaticFs}
```

`util.CachingEmbedFS` (`util/embedfs.go`) gives every file the process start as `ModTime`, because
`embed.FS` reports a zero mtime and `http.FileServer` then cannot answer `304 Not Modified`.

- `staticRegex` (`/static/...`, `/app.html`, `/sw.js`, `/sw.js.map`) -> `handleStatic`; `docsRegex`
  (`/docs...`) -> `handleDocs`.
- `GET /` (when `web-root` is `/`) and any bare `/<topic>` or `/<domain>/<topic>` go to
  `handleWebApp` via `handleTopic`, which serves `/app.html` for SPA deep links. `handleTopic`
  answers `GET /<topic>?up=1` with the UnifiedPush discovery JSON instead.
- The magic-link landing routes `/account/email/verify/<token>` and
  `/account/password/reset/<token>` go to `handleWebAppNoIndex`, which also sets
  `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex`, so the one-time token does not leak
  via `Referer` or get indexed.

`go run` fails without the embedded trees: run `make cli-deps-static-sites` for placeholders, or
`make web docs` for the real thing.

## 3. Topics, subscribers, visitors

### Server-level state

`Server` (`server/server.go`) holds, under `s.mu` (RWMutex): `topics map[string]*topic`, the
`messages` counter and its `messagesHistory` ring, and the listener fields. Visitors are **not**
stored on the server any more; see below.

`topicsFromIDs` takes the write lock because it may insert. Before inserting it enforces
`disallowed-topics`, the global `global-topic-limit` (`TotalTopicLimit`, default 15000) and the
visitor's topic-creation bucket (42911); passing `v == nil` bypasses the latter and is for internal
callers only. `topicsFromPath` splits a comma list, de-duplicates it, and caps it at
`subscribeTopicsLimit` (150, 40061). `allTopics` copies the map under the read lock, so scans
(pattern matching, user deletion) run without `s.mu`.

### topic

```go
type topic struct {
    ID          string
    subscribers map[int]*topicSubscriber
    lastAccess  time.Time
    delivered   map[string]time.Time // message ID -> when it went out here
    mu          sync.RWMutex
}
```

(`server/topic.go`.) A `topicSubscriber` holds the `user` as of subscribing (nil if anonymous),
the `subscriber` callback `func(v *visitor, msg *model.Message) error`, the `cancel` func of the
stream's context, `subscribedAt`, and a `sent` channel used to chain sends in order.

Subscriber IDs are `rand.Int()` with a best-effort 5-attempt collision retry. `Stale()` is true
when there are no subscribers and `lastAccess` is older than `topicExpungeAfter` (6 hours); the
manager then deletes the topic from the map. State that must outlive the in-memory topic (a
UnifiedPush topic's subscriber activity and rate visitor) is kept by the `topic` package instead.

### Fan-out

`topic.Publish` calls `deliver`, which, under `t.mu`:

1. For each message, picks the subscribers that have not had it: all of them if the message never
   went out on this topic, otherwise only those that subscribed after it went out.
2. Records the message in `delivered` with the current time.
3. For each subscriber, starts one goroutine that waits for that subscriber's previous batch
   (`sent` channel) and then sends its messages in order.

Invariants:

- **Delivery never happens under `t.mu`.** Only the selection does; a wedged client cannot block
  publishes to the topic.
- **One goroutine per subscriber per batch**, chained per subscriber, so a slow client never
  head-of-line blocks others, and messages to one subscriber never overtake each other (a
  reconnect asks for what came after the last message the client got).
- **`Publish` is asynchronous and returns nil.** The publisher's response is written before any
  subscriber has necessarily received anything; send errors are logged and the rest of that
  subscriber's batch is dropped.
- **At most once per subscriber.** The `delivered` map is what lets the tail (below) and the live
  fan-out both call `deliver` without duplicates. Entries are kept at least
  `topicDeliveredRetain` (2 minutes) and dropped by `ExpireDelivered` on the manager pass.

### The publish path

`handlePublish` -> `handlePublishInternal` (`server/server.go`):

1. Returns 503 if the node is read-only (draining).
2. Pulls the `*topic` and rate visitor out of the request context.
3. `parsePublishParams`, then the daily message check `vrate.MessageAllowed()` (42908), the email
   and call checks, and builds the `*model.Message` (`Sender`, `User`, `Expires` from the
   visitor's `MessageExpiryDuration`).
4. `handlePublishBody` decides message versus attachment; `SanitizeUTF8()`.
5. If not delayed: `s.dispatch(v, t, m, opts)`.
6. If `cache` is on: delete any scheduled message with the same sequence ID (and its attachment
   files), then `s.messageCache.AddMessage(m)`.
7. `s.messages++`.

`dispatch` is the single choke point every published message passes through, including delete
and clear events and delayed messages:

- `s.cluster.ForwardMessage(m)`: hand to peer nodes (no-op single-node, see `cluster.md`)
- `t.Publish(v, m)`: local fan-out (in a goroutine for the delayed sender)
- `go s.sendToFirebase(v, m)` if Firebase is configured and requested
- `go s.sendEmail(...)`, `go s.callPhone(...)` if requested
- `go s.forwardPollRequest(v, m)` if `upstream-base-url` is set (not for UnifiedPush)
- `go s.publishToWebPushEndpoints(v, m)` if web push is enabled

**Fan-out happens before persistence.** A live subscriber can see a message that a `?poll=1`
issued a moment later does not return yet, especially with batched writes. Delayed messages
(`m.Time` in the future) skip `dispatch` entirely and are only written to the cache, unpublished;
`runDelayedSender` picks them up later.

### The tail

`runTail` (`server/server_tail.go`) runs once per second when a message cache exists. It calls
`message.Tail.Next()` (`message/tail.go`), which reads every row written to the cache since the
last call, in row-id order, from the **primary**, and hands the messages to `topic.PublishMissed`
for topics that exist locally. Thanks to the `delivered` map, only subscribers that missed a
message get it, for example a client that reconnected while the message was published but not yet
written, so its `since=` read missed it. In a cluster this also covers fan-out to this node that
was lost.

Row ids are assigned on insert but become visible on commit, and concurrent writers commit out of
order, so the tail tracks "holes" (skipped ids) and re-checks them until the row appears or
`tailHoleTimeout` (1 minute) passes.

`sinceForTopic` uses the same `delivered` map from the other side: if a client reconnects with
`since=<id>` for a message the cache does not know yet (still in a write batch) but which went out
on this node, the replay starts shortly before that delivery time instead of replaying the whole
topic.

### Connection teardown

Every stream registers its `cancel` func in two places: with each topic (`Subscribe`) and in
`s.streams` (`streamRegistry`, used by `Drain` to close streams gradually). Cancellation paths:

| Function | Used by |
|---|---|
| `topic.CancelSubscriberUser(userID)` | `killUserSubscriber` (ACL revocation), `handleUsersDeleted` (user deletion, also from peers) |
| `topic.CancelSubscribersWhere(func(u) bool)` | `reauthorizeSubscribers`, after an access-cache reload found changed entries |
| `topic.CancelSubscribersExceptUser(userID)` | Reservation takeover |
| `topic.CancelAllSubscribers` | Closing all local subscribers (cluster isolation) |

`subscribersMatching` copies the map under the read lock and runs the match and cancels without
`t.mu`, because canceling logs the topic's context (which takes `t.mu`) and the canceled stream's
`Unsubscribe` needs the write lock.

`authorizeSubscribed` closes a race: after a stream subscribed, it authorizes once more against the
access cache, so an access change whose re-check ran between the request's authorization and the
subscribe cannot miss the stream.

### visitor

A `visitor` (`server/visitor.go`) is built **per request** and never changed afterwards, so
concurrent requests from the same IP cannot see each other's user (#1051). It carries:

| Field | Meaning |
|---|---|
| `key` | `vis.Key` (`visitor/key.go`): `user:<id>` for a user **with a tier**, else `ip:<prefix>` with the IP masked to `visitor-prefix-bits-ipv4/ipv6` |
| `ip`, `user` | Request IP and authenticated user (nil if anonymous) |
| `limits` | `vis.LimitsFor`: config-based limits, or the tier's limits for a user with a tier |
| `env` | `visitorEnv`, shared by all visitors: config, the limit service, the per-node subscription gauge, the message cache, the user manager |

A logged-in user without a tier is keyed by IP and shares the IP's counts with anonymous visitors
behind the same prefix. A user with a tier shares one key across all of their source IPs, which is
why the ban feed uses the request IP from the context rather than the visitor.

All counting lives in `limit.Service` (`limit/service.go`), keyed by visitor key. A visitor method
builds a `limit.Rule` from its limits per call (`requestRule`, `messageRule`, `emailRule`,
`bandwidthRule`, `topicCreationRule`, `authRule`, `accountRule`, `firebaseRule`, ...) and asks the
service; the service keeps the state, so a tier change applies on the next request without
rebuilding anything.

| Kind (`limit.Kind`) | Shape | Checked by |
|---|---|---|
| requests | token bucket, per node | `limitRequests`, `limitRequestsWithTopic` |
| messages, emails, calls | daily count per key (plus a bucket for emails), shared through the usage table | `MessageAllowed`, `EmailAllowed`, `CallAllowed` |
| bandwidth | bucket of daily bytes | attachment upload/download, poll replay bytes |
| topics | bucket | `TopicCreationAllowed` |
| account, auth | buckets, anonymous visitors only | signups and password-reset requests; failed logins (42909) |
| firebase | single token with a long refill | pauses Firebase sends after "quota exceeded" |

Daily periods are per key, starting at an offset hashed from the key, so resets are spread over
the day rather than happening at midnight. With a store (`limit.NewSQLiteStore` /
`NewPostgresStore` on the user database) a sync worker writes this node's admitted counts to the
`usage` table every 5 seconds and reads back what other nodes wrote; store failures never reach
the request path. Anonymous keys are only written once they reach a tenth of their daily cap
(`promoteRate`), which keeps most keys out of the table. Concurrent open subscriptions are counted
per node only (`subscriptionGauge`, `server/visitor_subscriptions.go`), 42903 when full.

`v.Info()` is the expensive variant (it queries attachment bytes and reservation counts); use
`infoLight` on hot paths. Idle keys are forgotten by `limit.Service.Prune` from the manager loop.

### UnifiedPush rate visitors

For UnifiedPush topics (`up` prefix, exactly 14 chars, `isUnifiedPushTopic`) and only with
`visitor-subscriber-rate-limiting`, `maybeSetRateVisitors` makes the **subscriber** the topic's
rate visitor, so pushes from an app server are billed to the subscriber's quota rather than the
publisher's. It requires no ACLs, ownership of a reserved topic, or write access to an unreserved
one. The state (rate visitor user ID and masked IP, subscriber activity) is kept by
`topic.Service` (`topic/service.go`), synced through the `topic` table every 5 seconds.
`rateVisitor` (`server/server_rate_visitor.go`) rebuilds the visitor per push; it stays live for
`rateVisitorLiveFor` (1 day) after the subscriber was last active.

A UnifiedPush publish without a live rate visitor gets 50701, or 41002 (410 Gone) once the topic
had no subscriber activity for `unifiedPushAbandonedAfter` (4 days), so app servers delete the
subscription (#664).

### The ban feed

With `ban-file` set, `handleError` calls `ban.Service.Record(ip, httpCode, ntfyCode)`
(`ban/service.go`). Strikes are counted per IP **prefix** (the same prefix bits as the visitor key)
in a weighted token bucket sized `ban-threshold` per `ban-window`; per-code weights
(`ban-weights`, `DefaultBanWeights` in `server/config.go`) let quota errors count zero. A breaching
prefix is appended to the ban file, which a tool like fail2ban can tail. The line format is a
contract with such parsers; do not change it casually.

## 4. Messages and the message cache

### model.Message

`model.Message` (`model/model.go`) is the single wire and storage struct; see the protocol doc for
the field table. For architecture purposes:

- `Sender netip.Addr` and `User string` are `json:"-"`: they exist for rate limiting and
  attachment attribution and never leave the server.
- `SequenceID` defaults to `m.ID`. Always serialize via `ForJSON()`, which blanks it when equal.
- `Expires` is set per message from the visitor's tier at publish time, so retention is per
  message, not global.
- `SanitizeUTF8()` runs once in the publish path; every consumer sees clean UTF-8 without NULs.
- `Size()` approximates the publisher-controlled bytes and is used to budget replays.

### message.Cache

`message.Cache` (`message/cache.go`) is one struct for both backends. The dialect is a `queries`
struct of SQL strings plus an `insertMessages` func, filled by `sqliteQueries`
(`message/cache_sqlite.go`) or `postgresQueries` (`message/cache_postgres.go`).

```go
mu *sync.Mutex // nil for PostgreSQL (concurrent writes supported), set for SQLite (single writer)
```

`maybeLock` / `maybeUnlock` wrap the write paths.

| Constructor | Backing |
|---|---|
| `NewPostgresStore(d, batchSize, batchTimeout)` | Shared `*db.DB` pool, no mutex |
| `NewSQLiteStore(file, startupQueries, cacheDuration, batchSize, batchTimeout, nop)` | Own `sql.DB`, wrapped in `db.New(&db.Host{DB: d}, nil)` |
| `NewMemStore()` | SQLite `file:<random>?mode=memory&cache=shared` (a bare `:memory:` would give each pooled connection its own database; see `createMemoryFilename`) |
| `NewNopStore()` | Memory store with `nop: true`; `addMessages` returns early |

Only `message`, `message_delete` and `message_clear` events are persistable; `insertMessageArgs`
rejects anything else with `ErrUnexpectedMessageType`. Inserts are multi-row: SQLite uses
multi-row `INSERT` statements of up to 1000 rows, Postgres one `INSERT ... SELECT FROM unnest(...)`
per batch (fixed SQL, so pgx's statement cache does not grow).

Reads:

| Method | Notes |
|---|---|
| `MessagesCapped(topic, since, scheduled, maxBytes)` | Time-based or `latest` replay. Queries run newest first, keep the newest messages within `maxBytes` (`Config.MessagePollSizeLimit`, default 10 MiB) and reverse; the bool reports truncation (sent as `X-Messages-Truncated: 1`) |
| `ResolveMarker(mid)` + `MessagesAfterMarker` | `since=<id>` replay; see the protocol doc |
| `Message(id)` | Single lookup, used by `handleFile` |
| `MessagesDue()` | Due delayed messages; on Postgres it **claims** them (see below) |
| `Tail()` | Starts a `message.Tail` after the newest row |

### Batching

If `cache-batch-size` or `cache-batch-timeout` is set, `newCache` creates a
`util.BatchingQueue[*model.Message]` and `AddMessage` becomes an enqueue. One goroutine,
`processMessageBatches`, drains it and calls `addMessages` per batch in one transaction. The queue
buffers up to `queueBufferedBatches` (100) full batches before `Enqueue` blocks, and `Close` waits
up to `closeFlushTimeout` (5s) for the last batch. Batching is why `handleFile` retries and why a
just-published message may not be visible to `?poll=1` yet.

### Delayed messages

A delayed message is written with `published = false`. `runDelayedSender` ticks every
`DelayedSenderInterval` (10s) and calls `sendDelayedMessages`, which reads `MessagesDue()`,
rebuilds each message's visitor from `m.Sender` and `m.User`, calls `dispatch` (rate limits are not
re-applied; they were applied at publish), then `MarkPublished`. On Postgres, `MessagesDue` uses
`postgresClaimMessagesDueQuery` (`FOR UPDATE SKIP LOCKED`, stamps `claimed_at`, 1000 rows per
claim), so concurrent senders on several nodes get disjoint sets; a claim older than
`claimTimeout` (2 minutes) is claimable again. In a cluster only the leader runs it.

### Retention and pruning

Retention is per message (`expires`, written at publish time from the visitor's tier). Pruning runs
from the manager loop in bounded batches of `ManagerBatchSize` (default 30000), never as one
unbounded `DELETE`:

- `pruneMessages` -> `DeleteExpiredMessages(limit)`: `DELETE ... WHERE mid IN (SELECT mid ... LIMIT ?)`.
- `pruneAttachments` -> `MarkExpiredAttachmentsDeleted(limit)`: only flips `attachment_deleted`.
  The bytes are removed by the attachment store's own sync loop, which diffs storage against
  `AttachmentsWithSizes()` (with `Config.AttachmentOrphanGracePeriod`, 1 hour, for fresh uploads). A crash
  between the phases leaves an orphaned file that the next sync reclaims, never a row pointing at a
  missing file.
- `ExpireMessages(topics...)` sets `expires` for whole topics, used when a reservation is removed
  and its messages must go.
- `DeleteScheduledBySequenceID` returns the deleted IDs so the caller can remove attachment files.

### Message counter

`s.messages` counts publishes on this node. `updateAndWriteStats` (manager loop) writes the delta
since the last flush with `AddStats` and folds the stored total back in, so in a cluster every
node's counter and `/v1/stats` converge on the shared sum.

## 5. The db/ abstraction

### Structure

`db.DB` (`db/db.go`) wraps a primary `*db.Host` and replica `*db.Host`s (`db/types.go`). `Query`,
`QueryRow`, `Exec`, the `*Context` variants and `Begin` always go to the **primary**; replica use
is opt-in per call site via `ReadOnly()`.

`ReadOnly()` picks the healthy replica with the fewest connections in use (round-robin among
equally busy ones) and falls back to the primary when none is configured or healthy. It does not
fall back when replica pools are merely full; the read waits. `Primary()` returns the primary
`*sql.DB` for schema setup and for reads that must see this process's own writes.

`New` marks the primary healthy and the replicas unhealthy, and starts `healthCheckLoop`: first
check after 5s, then every 30s, 10s ping timeout. For the first seconds every `ReadOnly()` call
therefore hits the primary. `MarkReplicasHealthyForTest` exists so tests need not wait. `Stats()`
feeds the `ntfy_db_*` metrics.

`pg.Open` pings; `pg.OpenReplica` does not (health is the loop's job). Pool settings are DSN query
params that `db/pg` strips before handing the DSN to pgx: `pool_max_conns` (default 10),
`pool_max_idle_conns`, `pool_conn_max_lifetime`, `pool_conn_max_idle_time`.

`db.ExecTx` / `db.QueryTx` (`db/util.go`) are the transaction helpers (`defer tx.Rollback()`,
commit on success) over the `db.Beginner` interface.

### Schema versioning (db/schema)

Every store migrates through `schema.Migrate(db, dialect, store, create, migrations)`
(`db/schema/schema.go`), which runs in **one transaction**:

- Postgres: all stores share one database and one `schema_version(store, version, min_version)`
  table, keyed by store name (`message`, `user`, `webpush`, `node_registry`). Setup is serialized
  across nodes with `pg_advisory_xact_lock(pg.SchemaLockKey)`.
- SQLite: each store is its own file, with `schemaVersion(id = 1, version, min_version)`.
- A fresh database (no version row) runs `create` and records the target version.
- `Migrations` is a map of `{From, To}` steps; the target version is the last `To`, and gaps or
  multi-version steps fail at startup.
- An older binary on a newer schema logs a warning and continues: **migrations must be additive**
  (new tables, indexes, nullable or defaulted columns), because rollbacks and rolling cluster
  upgrades run old code on new schemas. A step marked `Breaking` raises `min_version`, and code
  older than that refuses to start.

### Which reads use replicas

| Call | Backend |
|---|---|
| `Cache.Message`, `messagesLatest`, `Topics`, `Stats`, attachment size queries | `ReadOnly()` |
| `Cache.messagesSinceTime` | `ReadOnly()`, except a replay starting less than `recentReplayWindow` (1 minute) ago, which reads the primary |
| `Cache.resolveMarker` | Replica first; if the marker is unknown there, the primary, and the replay is then read from the primary too |
| `Cache.MessagesDue`, `Tail.Next` | Primary (claims must not race, a lagging replica would create false holes) |
| `Manager.authorizeTopicAccess` without the access cache | `ReadOnly()` |
| `accessCache` reloads | Primary, so a reload after a write sees it |

### Replication-lag hazards

1. **Publish then poll.** With batching and lag, a message can be missing from a poll for
   batch-timeout plus lag. Recent time-based replays read the primary to shrink this, and the tail
   delivers to live subscribers what their replay missed.
2. **Attachment upload then download.** `handleFile` reads `Cache.Message` from a replica; its
   retry only covers the batching delay.
3. **ACL change then authorize.** Without `auth-access-cache`, `authorizeTopicAccess` reads a
   replica, so a fresh grant may be denied briefly. The access cache reloads from the primary.
4. **Attachment quota.** `AttachmentBytesUsedBy*` read a replica, so concurrent uploads can
   transiently under-count the total-size limit.

Rule of thumb: a value that drives an authorization or quota decision immediately after a write by
this process should come from the primary.

## 6. The user/ package

### Manager

`user.Manager` (`user/manager.go`) is the store for users, tiers, tokens, ACLs, phone numbers,
emails and magic links. It holds the `*db.DB`, a dialect `queries` struct (`user/manager_sqlite.go`,
`user/manager_postgres.go`), the token write-behind queue, an optional `accessCache`, and a `quit`
channel.

- `user.OpenSQLite(file)` opens the auth file with `_case_sensitive_like=on` (ACL matching uses
  `LIKE`, which is case-insensitive in SQLite by default, so a rule for `secret` would otherwise
  match `SECRET`).
- `NewSQLiteManager` migrates **before** running the startup queries (the 5 -> 6 table rebuilds rely
  on foreign keys being off), then `newManager`. `NewPostgresManager` migrates and calls
  `newManager`.
- `newManager` provisions config-declared users, grants and tokens
  (`maybeProvisionUsersAccessAndTokens`), starts the access cache if enabled, and starts
  `asyncQueueWriteLoop` (token last-access/last-origin, every `Config.AuthStatsQueueWriterInterval`,
  default 33s) and `asyncExpiredMagicLinkReapLoop` (hourly).

The user schema also hosts the `usage` table (package `limit`) and the `topic` table (package
`topic`), so they share its migrations; on SQLite they live in the auth file. Without a user
database, both services run from memory only.

### Authentication

`maybeAuthenticate` (`server/server_auth.go`) is the only entry point:

1. Extract the client IP honoring `behind-proxy`, `proxy-forwarded-header` and
   `proxy-trusted-hosts`, and store it in the context.
2. Build an IP visitor; without a user manager, return it.
3. `readAuthHeader` reads `Authorization`, or the `?auth=` / `?authorization=` query param (base64url
   of the whole header value; it exists because the browser `WebSocket` constructor cannot set
   headers). The query param overrides the header.
4. `supportedAuthHeader` accepts only `basic ` / `bearer ` prefixes; anything else (`WebPush`,
   `vapid`, ...) is treated as anonymous, not as an error (#629).
5. The auth-failure bucket is checked **before** authenticating (42909).
6. On failure: `vip.AuthFailed()`, then 401. A visitor is returned in every branch.

Basic auth with an empty username is treated as a token (`authenticateBasicAuth`), which makes
`curl -u :tk_...` work. `Manager.Authenticate` accepts a username or a verified primary email
(`userByNameOrEmail`) and burns a bcrypt comparison against `userAuthIntentionalSlowDownHash` when
the user does not exist or is soft-deleted, so timing does not reveal existence. Bearer auth fires
`EnqueueTokenUpdate` in a goroutine to record last access and origin.

### Tokens

`tk_` + 29 lowercase alphanumerics (32 total, `GenerateToken`), at most `tokenMaxCount` (60) per
user; `createTokenTx` trims the excess with `deleteExcessTokens`. Expired tokens and soft-deleted
users past `userHardDeleteAfterDuration` (7 days) are removed by `pruneTokens` in the manager loop.
Provisioned tokens cannot be changed or deleted through the API (`canChangeToken`, 40906).

### ACLs

`Authorize(user, topic, perm)` resolves in this order:

1. Admins are always allowed.
2. The user's own sync topic is always allowed (`subtle.ConstantTimeCompare`); without this,
   `auth-default-access: deny-all` would lock users out of their device-sync topic (#733).
3. Otherwise look up `(username or Everyone, topic)` via `authorizeTopicAccess`; if no entry
   matched, fall back to `auth-default-access`.

Match priority, in `selectTopicPerms` (`ORDER BY u.user DESC, LENGTH(a.topic) DESC, a.write DESC`)
and mirrored in the cache:

1. A rule for the specific user beats a rule for `Everyone` (`*`).
2. A longer pattern beats a shorter one (`test*` beats `*`).
3. At equal length, write beats read.

`accessCache` (`user/access_cache.go`, enabled by `auth-access-cache`) is an in-memory index over the
whole `user_access` table: `exact[username][escapedTopic]` for O(1) hits plus `pattern[username]`
for `%` rules. Keys are stored escaped (`\_`), so `Lookup` escapes the incoming topic first.

- After a write by this process, `accessWritten` calls `AccessWrittenFunc` (the server broadcasts
  to peers) and then reloads the cache synchronously, so the change applies here before the write
  returns.
- `RequestReload` asks the loop for a coalesced reload (used when a peer changed entries).
- The loop also reloads every `DefaultAccessCacheReloadInterval` (67s) to pick up writes by another
  process, most typically the `ntfy access` CLI against the same database.
- A reload that found changed entries calls `AccessCacheChangedFunc`, which is
  `Server.reauthorizeSubscribers`: it re-authorizes the matching live streams (from memory) and
  cancels those that lost read access.

### Roles, permissions, tiers

`Role` (`user/types.go`) is `admin` / `user` / `anonymous`; some SQL hardcodes these literals.
`Permission` is a two-bit set: `PermissionDenyAll = 0`, `PermissionRead = 1`, `PermissionWrite = 2`,
`PermissionReadWrite = 3`. `Everyone` is the literal username `*` with ID `u_everyone`.

`Tier` carries every per-account limit (daily messages, emails, calls; message and attachment
expiry; attachment file and total size; daily bandwidth; reservations) plus the Stripe price IDs.
Tiers reach the request path only through `vis.LimitsFor`, and the visitor key switches to
`user:<id>` only for users with a tier.

## 7. Concurrency model

### Goroutine inventory

| Goroutine | Started in | Lifetime |
|---|---|---|
| One per listener (HTTP, HTTPS, unix, admin socket, metrics, pprof, SMTP, cluster) | `Run` | process |
| `runManager`, `runDelayedSender`, `runFirebaseKeepaliver` | `Run` | until `closeChan` |
| `runTail` | `New` | until `Tail.Close` |
| `db.healthCheckLoop` | `db.New` | until `DB.Close` |
| `Cache.processMessageBatches` | `newCache` | until the queue is closed |
| `BatchingQueue` timeout ticker | `util.NewBatchingQueue` | until `Close` |
| `limit.Service` sync worker, `topic.Service` sync worker | `Start` | until `Close` |
| `Manager.asyncQueueWriteLoop`, `asyncExpiredMagicLinkReapLoop`, access cache reload loop | `newManager` | until `quit` |
| Attachment store sync loop | `attachment` package | until `Close` |
| Per request | `net/http` | request |
| Per delivery: one per subscriber per batch | `topic.deliver` | message delivery |
| Per publish side effect: Firebase, email, call, upstream, web push | `dispatch` | fire and forget |
| Per WebSocket: reader and writer via `errgroup` | `handleSubscribeWS` | connection |

Under load the dominant terms are one handler goroutine per open stream (three for WebSockets)
plus transient delivery goroutines, roughly in-flight messages times subscribers. A high
goroutine count under heavy publish load is expected, not necessarily a leak.

### The manager loop

`runManager` ticks every `ManagerInterval` (default 1 minute) and calls `execManager`
(`server/server_manager.go`), which warns to "only selectively lock with the mutex":

1. `pruneVisitors`: `limit.Service.Prune` forgets settled keys (every node).
2. Leader only (always true single-node): `pruneTokens`, `pruneAttachments`, `pruneMessages`,
   `pruneAndNotifyWebPushSubscriptions`.
3. Count cached messages, then sweep `s.topics` under the write lock: delete stale topics, call
   `ExpireDelivered`, and `topicService.Touch` UnifiedPush topics with open streams.
4. Collect mail and user stats, log the `Server stats` line, set Prometheus gauges.
5. `updateAndWriteStats` appends to the 10-entry `messagesHistory` ring and flushes the counter;
   `/v1/stats` derives the rate from that ring and `ManagerInterval`.

There is no midnight stats reset loop any more: daily counts reset per key in `limit` (see above).

### Other loops

- `runDelayedSender`: every 10s, leader only, see "Delayed messages".
- `runFirebaseKeepaliver`: every `FirebaseKeepaliveInterval` (3 hours), leader only, sends a
  keepalive to the `~control` topic with a synthetic visitor on `0.0.0.0`. The iOS `~poll` variant
  is commented out because of the thundering-herd problem (#677).

### Locking rules

- `s.mu` (RWMutex) guards the topic map, the message counter and history, and the listeners. Do not
  block on I/O while holding it; scans copy via `allTopics` first.
- `t.mu` guards a topic's subscriber and delivered maps, never held across delivery or cancel.
- `limit.Service` is sharded (64 shards, each with its own lock); visitor methods take no lock.
- `Cache.mu` is nil on Postgres and serializes writers on SQLite.
- `Manager.mu` guards the token write-behind queue.
- `accessCache.mu` guards the index; `reloadMu` serializes reloads so an older scan never replaces
  a newer one.

## 8. Transports: WebSocket, SSE, JSON stream, raw, long-poll

The HTTP streaming transports share `handleSubscribeHTTP`, parameterized by a content type and a
`messageEncoder`:

| Transport | Content-Type | Encoder |
|---|---|---|
| JSON stream | `application/x-ndjson` | One `ForJSON()` object per line |
| SSE | `text/event-stream` | `data: {...}`; non-message events get an `event: <type>` line, so the browser's `.onmessage()` does not fire for `open` / `keepalive` |
| raw | `text/plain` | Message body with newlines replaced by spaces; an empty line for other events |

Shared mechanics of `handleSubscribeHTTP`:

1. `v.SubscriptionAllowed()` up front (42903), `defer v.RemoveSubscription()`.
2. `topicsFromPath` (comma list, de-duplicated, at most 150 topics), `parseSubscribeParams`; 503
   for a non-poll stream on a draining node.
3. A `wlock` mutex plus `closed` flag guard the response writer. The deferred func takes the lock,
   sets `closed` and releases it, which waits for any in-flight `sub()` and turns later ones into
   no-ops. Without this, a delivery goroutine can write to a `ResponseWriter` after the handler
   returned and panic (#338, PR #1598). Do not "simplify" this away.
4. `maybeSetRateVisitors` for UnifiedPush topics.
5. `?poll=1`: keepalive the topics, replay via `sendOldMessages` (charged to the visitor's
   bandwidth bucket), return. No subscription is registered, so long-polls are not counted as
   subscribers.
6. Streaming: set `Incremental: ?1`, register in `s.streams`, `Subscribe` to every topic,
   `authorizeSubscribed`, send `open`, replay history with `sendOldMessages`, then loop on the
   cancel context, the request context, and a `KeepaliveInterval` timer (45s default) that sends a
   `keepalive` event. Subscribing **before** the replay, together with the topic's `delivered`
   map, is what makes the hand-over from replay to live delivery gap-free.

`sendOldMessages` resolves a `since=<id>` marker once, queries each topic (capped by
`Config.MessagePollSizeLimit`), merges and **stable**-sorts by `Time` (an unstable sort reorders a
topic's own same-second messages, #1297), sets `X-Messages-Truncated` if any topic was capped, and
writes them.

`handleSubscribeWS` differs:

- Requires `Upgrade: websocket` or returns 40016, and checks the draining state before the upgrade
  so a load balancer can retry elsewhere.
- `CheckOrigin` always returns true; ntfy is intentionally open cross-origin.
- An `errgroup` runs a reader and a writer. The reader only drives pong handling and disconnect
  detection; the read limit is `wsReadLimit` (64 bytes) because clients only send pongs.
- Liveness is protocol ping/pong: the writer pings every `KeepaliveInterval`; the read deadline is
  `KeepaliveInterval + wsPongWait` (15s), refreshed in the pong handler. No `keepalive` events.
- Writes go through `conn.WriteJSON(msg)` under `wlock` with a 2s write deadline. This serializes
  the raw `*model.Message`, not `ForJSON()`, so `sequence_id` is always present on `/ws`.
- An external cancel closes the connection with a normal-closure error.
- Close codes 1000, 1001, 1005 and 1006 are treated as normal, as is any error after the server
  closed the connection itself; other errors are wrapped in `*errWebSocketPostUpgrade`.

| | Long-poll | JSON/SSE/raw stream | WebSocket |
|---|---|---|---|
| Counts as a subscription | no | yes | yes |
| History replay | default `since=all` | only with `since=` | only with `since=` |
| Liveness | n/a | `keepalive` event | ping/pong |
| Goroutines per connection | handler only | handler | handler, reader, writer |
| Externally cancellable | no | yes (context) | yes (context and close) |
| `?auth=` query param | works | works (needed for `EventSource`) | works (needed for browser `WebSocket`) |

## 9. Quick reference

| Question | Where |
|---|---|
| What handles this URL? | `handleInternal` in `server/server.go` |
| What does error code NNNNN mean? | `errHTTP*` vars in `server/errors.go` |
| How is a message fanned out? | `Server.dispatch` (`server/server.go`), `topic.deliver` (`server/topic.go`) |
| How do missed messages reach live subscribers? | `runTail` (`server/server_tail.go`), `message.Tail` (`message/tail.go`) |
| Why is my client rate limited? | `server/visitor.go` (rules), `limit/service.go` (decisions), `server/server_middleware.go` |
| What is persisted, and when? | `Cache.AddMessage` (`message/cache.go`), `util/batching_queue.go` |
| Primary or replica? | `db.DB.ReadOnly` / `Primary` (`db/db.go`) and the call site |
| How are schemas migrated? | `schema.Migrate` (`db/schema/schema.go`) and each store's `*_schema.go` |
| Who can read or write this topic? | `Manager.Authorize` (`user/manager.go`), `accessCache.Lookup` (`user/access_cache.go`) |
| What runs periodically? | `execManager` (`server/server_manager.go`), `runDelayedSender`, `runFirebaseKeepaliver`, `runTail` |
| What happens on shutdown? | `sigHandlerShutdown` (`cmd/serve_unix.go`), `Server.Drain` (`server/server_drain.go`), `Server.Stop` |
| How does clustering change this? | [cluster.md](cluster.md) |
