# Protocol and data model

What goes over the wire and what lands in the database. The wire side is one JSON message struct
(`model.Message`) and a deliberately generous publish surface: every option can be set as an HTTP
header, a query parameter or a JSON body field, with short aliases, so a publish fits in a bare
`curl` or a browser address bar. The storage side is a few independent SQL stores (the message
cache in `message/`, the user database in `user/`, and the smaller stores that share them), each
with a SQLite and a Postgres dialect whose names do not entirely agree. All paths are relative to
the root of this repo; the request path itself is described in
[server-architecture.md](server-architecture.md).

## Where the types live

The wire types are not in `server/`: they live in `model/` so the CLI, the mail formatter and the
message cache can use them without importing the server.

| Concept | Package | File |
|---|---|---|
| `Message`, `Attachment`, `Action`, `SinceMarker`, event constants | `model` | `model/model.go` |
| Action parsing and validation | `action` | `action/action.go` |
| JSON publish body, query filters, `api*` request/response types | `server` | `server/types.go` |
| Message cache (store, queries, schema, migrations, tail) | `message` | `message/cache*.go`, `message/tail.go` |
| User database (manager, queries, schema, migrations) | `user` | `user/manager*.go`, `user/types.go` |
| Daily usage counts (`usage` table) | `limit` | `limit/store*.go` |
| UnifiedPush topic state (`topic` table) | `topic` | `topic/store*.go` |
| Web push subscriptions | `webpush` | `webpush/store*.go` |
| Schema versioning | `db/schema` | `db/schema/schema.go` |

## The wire message

`model.Message` is emitted by every subscribe endpoint and returned as the body of a successful
publish (`handlePublish` writes `m.ForJSON()`).

| Field | JSON | Type | Meaning |
|---|---|---|---|
| `ID` | `id` | string | Random 12-char message ID; always present |
| `SequenceID` | `sequence_id,omitempty` | string | Identity across updates; omitted when equal to `id` (except on `/ws`, see below) |
| `Time` | `time` | int64 | Unix seconds; for a scheduled message, the **delivery** time |
| `Expires` | `expires,omitempty` | int64 | Unix seconds; not set for `open`/`keepalive` or with `cache=no` |
| `Event` | `event` | string | One of the event constants below |
| `Topic` | `topic` | string | For `open`/`keepalive` on a multi-topic subscription, the comma-joined list |
| `Title` | `title,omitempty` | string | At most 1024 bytes (40057) |
| `Message` | `message,omitempty` | string | Body; `triggered` if empty (`emptyMessageBody`) |
| `Priority` | `priority,omitempty` | int | 1 to 5; 0 means unset and is treated as 3 |
| `Tags` | `tags,omitempty` | []string | At most 512 bytes in total (40058) |
| `Click` | `click,omitempty` | string | URL opened on tap |
| `Icon` | `icon,omitempty` | string | Icon URL |
| `Actions` | `actions,omitempty` | []*Action | At most 3 |
| `Attachment` | `attachment,omitempty` | *Attachment | Attachment metadata |
| `PollID` | `poll_id,omitempty` | string | On `poll_request` events: the message to fetch |
| `ContentType` | `content_type,omitempty` | string | Empty for `text/plain`, or `text/markdown` |
| `Encoding` | `encoding,omitempty` | string | Empty for UTF-8, or `base64` for binary UnifiedPush payloads |
| `Sender` | `-` | netip.Addr | Publisher IP; **never serialized** |
| `User` | `-` | string | Publisher user ID; **never serialized** |

`Sender` and `User` are persisted (columns `sender` and `user` / `user_id`) for rate limiting and
attachment attribution but never leave the server. Logging goes through `Message.Context()`, which
picks fields deliberately.

`ForJSON()` clones the message and blanks `SequenceID` when it equals `ID`. Clients must treat a
missing `sequence_id` as "equal to `id`". The `/ws` writer serializes the message without
`ForJSON()`, so there `sequence_id` is always present.

`SanitizeUTF8()` runs once in the publish path, after the body is read: it replaces invalid UTF-8
and strips NUL bytes from every publisher-supplied string, including tags and attachment metadata.

### Event types

| Event | Constant | When | Body |
|---|---|---|---|
| `open` | `OpenEvent` | First frame of every streaming subscription; not sent with `poll=1` | empty |
| `keepalive` | `KeepaliveEvent` | Every `keepalive-interval` (45s) on `/json`, `/sse`, `/raw`; also the Firebase `~control` keepalive | empty |
| `message` | `MessageEvent` | A notification | full |
| `message_delete` | `MessageDeleteEvent` | `DELETE /<topic>/<sequence-id>` or `GET /<topic>/<sequence-id>/delete` | `sequence_id` |
| `message_clear` | `MessageClearEvent` | `GET`/`PUT /<topic>/<sequence-id>/read` or `/clear` | `sequence_id` |
| `poll_request` | `PollRequestEvent` | Sent to Firebase / upstream so a client fetches a message it cannot receive inline | `poll_id` |

`/ws` sends no `keepalive` frames; it uses WebSocket ping/pong with a read deadline of
`keepalive-interval + 15s` and a 64-byte read limit, so clients must answer pings.

Query filters apply only to `message`, `message_delete` and `message_clear`
(`queryFilter.Pass` in `server/types.go`); other events always pass.

### Attachment and Action

`Attachment`: `name`, `type` (MIME), `size` (bytes), `expires` (Unix seconds), `url`. An uploaded
attachment's URL is `<base-url>/file/<message-id><ext>`, and its expiry never exceeds the message's.

`Action` has `id` (random), `action`, `label`, `clear`, and type-specific fields: `url` (view, http),
`method`/`headers`/`body` (http), `intent`/`extras` (broadcast), `value` (copy). `action.Parse`
validates:

- At most 3 actions (`actionsMax`).
- `action` is one of `view`, `broadcast`, `http`, `copy` (lowercased first); `label` is required.
- `url` is required for `view` and `http`, `value` for `copy`.
- `method` is uppercased; `body` is not allowed for `GET`/`HEAD`. The http action defaults to
  **POST**.

Input starting with `[` is parsed as JSON, anything else with the hand-written "simple" format
parser.

## Publishing

### Every way to publish

| Form | Notes |
|---|---|
| `PUT`/`POST /<topic>` | Canonical; the body is the message or an attachment |
| `PUT`/`POST /` with a JSON body | `transformBodyJSON`; the topic comes from the body |
| `GET /<topic>/publish`, `/send`, `/trigger` | Everything in query params |
| `PUT`/`POST /<topic>/<sequence-id>` | Update; the sequence ID comes from the path |
| `DELETE /<topic>/<sequence-id>`, `GET /<topic>/<sequence-id>/delete` | Emits `message_delete` |
| `GET`/`PUT /<topic>/<sequence-id>/read` or `/clear` | Emits `message_clear` |
| `POST /_matrix/push/v1/notify` | Matrix push gateway, via `transformMatrixJSON` (`server/server_matrix.go`) |
| Email to the SMTP server | `smtp-server-listen`; `server/smtp_server.go` turns a mail into a publish through `s.handle` |

There is no special form-encoded handling. `readParam` falls back to query parameters, so
`?title=x&priority=5` works on any method.

### The JSON body path

`transformBodyJSON` (`server/server.go`) does not build a message. It unmarshals `publishMessage`
(`server/types.go`) with a body limit of twice `message-size-limit`, validates the topic, rewrites
`r.URL.Path` to `/<topic>`, replaces the body with the message text, and sets the equivalent
`X-*` header for every field. There is only one implementation of publish options, so the JSON and
header APIs cannot drift.

- `cache` and `firebase` are strings, not bools, because they default to true and a zero `bool`
  would be indistinguishable from "not set".
- `template` and `unifiedpush` are not in `publishMessage`; they can only be sent as a header or
  query parameter.

### Publish parameters

`readParam` (`server/util.go`) checks the names as headers first, in order (first non-empty wins),
then the same names lowercased as query parameters. Header values are MIME word-decoded
(`maybeDecodeHeader`), so `=?utf-8?q?...?=` works for non-ASCII titles. Parsing is in
`parsePublishParams` (`server/server.go`).

| Header | Aliases | Field | Validation and behavior |
|---|---|---|---|
| `X-Title` | `Title`, `t` | `Title` | At most 1024 bytes, else 40057 |
| `X-Message` | `Message`, `m` | `Message` | `\n` becomes a newline unless inline-template mode |
| `X-Priority` | `Priority`, `prio`, `p` | `Priority` | `util.ParsePriority`, else 40007; deferred to the template if templating is on |
| `X-Tags` | `Tags`, `tag`, `ta` | `Tags` | Comma-separated and trimmed; at most 512 bytes in total |
| `X-Click` | `Click` | `Click` | none |
| `X-Icon` | `Icon` | `Icon` | Must match `^https?://`, else 40021 |
| `X-Attach` | `Attach`, `a` | `Attachment.URL` | Must match `^https?://`, else 40013; name taken from the URL path, falling back to `attachment` |
| `X-Filename` | `Filename`, `file`, `f` | `Attachment.Name` | Forces the body to be treated as an attachment |
| `X-Delay` | `Delay`, `X-At`, `at`, `X-In`, `in` | `Time` | See scheduled delivery |
| `X-Timezone` | `Timezone` | (delay) | IANA zone for natural-language delays, else 40059 |
| `X-Actions` | `Actions`, `action` | `Actions` | `action.Parse`, else 40018 |
| `X-Email` | `X-E-Mail`, `Email`, `E-Mail`, `mail`, `e` | (not stored) | An address matching `^[^\s,;]+@[^\s,;]+$` or a bool, else 40050; 40001 without a mailer |
| `X-Call` | `Call` | (not stored) | `+<digits>` or a bool, else 40033; 40032 without Twilio and a user manager |
| `X-Cache` | `Cache` | (control) | Bool, default **true**; false means the message is never persisted |
| `X-Firebase` | `Firebase` | (control) | Bool, default **true** |
| `X-UnifiedPush` | `UnifiedPush`, `up` | (control) | Bool; forces `firebase=false`; also implied by `Content-Encoding: aes128gcm` |
| `X-Markdown` | `Markdown`, `md` | `ContentType` | Bool; also set by `Content-Type: text/markdown` |
| `X-Template` | `Template`, `tpl` | (control) | Empty = off, a bool = inline mode, anything else = template file name (`templateMode`) |
| `X-Sequence-ID` | `Sequence-ID`, `sid` | `SequenceID` | Must match the topic charset, else 40049; on update routes the path wins |
| `X-Poll-ID` | `Poll-ID` | `PollID` | If set: `unifiedpush=false`, `cache=false`, no email |

Bools: `1`, `yes`, `true` are true; `0`, `no`, `false` are false; anything else is false but still
"set" (`readBoolParam`, `toBool`).

**The `Priority` header trap.** RFC 9218 defines an HTTP `Priority` header that some proxies and
CDNs (Cloudflare, for example) add to forwarded requests. `maybeIgnoreSpecialHeader` drops a
`Priority` header matching `^u=\d,\s*(i|\d)$|^u=\d$` (`priorityHeaderIgnoreRegex`), so the lookup
falls through to `X-Priority`, `prio`, `p` or the query string. Without this, every such request
would fail with 40007.

### How the body is interpreted

`handlePublishBody` (`server/server.go`) picks the first matching case:

1. `poll_request` event: the body is discarded.
2. UnifiedPush: the body is the message verbatim, base64 with `encoding=base64` if not valid UTF-8.
   Not trimmed.
3. `Attach` set (external URL): the body is the message text.
4. `Filename` set: the body is an attachment.
5. Templating on: the body is JSON rendered through the template (`server/server_template.go`).
6. Body within `message-size-limit` (4096 bytes by default) and valid UTF-8: the body is the
   message, trimmed.
7. Otherwise: the body is an attachment.

Case 6 versus 7 is the surprising one: whether `curl -T file.txt` becomes a message or an
attachment depends only on size and encoding. Attachments need an attachment store and
`base-url` (40014), and are limited by the visitor's file size, total size and bandwidth limits
(41301).

### Priorities

`util.ParsePriority` (`util/util.go`):

| Value | Spellings |
|---|---|
| 1 | `1`, `min` |
| 2 | `2`, `low` |
| 3 | `3`, `default` |
| 4 | `4`, `high` |
| 5 | `5`, `max`, `urgent` |

Empty parses to 0, which is "not set" and treated as 3 in query filtering and in email formatting.

### Tags and emoji

Tags are stored as one comma-joined string. The server does not interpret them for notifications;
emoji mapping is a client concern. The exception is email: `mail/format.go` embeds an emoji map and
puts mapped emoji in the subject, listing unmapped tags separately.

### Scheduled delivery

`X-Delay` and its aliases go through `util.ParseFutureTime` (`util/time.go`), which tries a Unix
timestamp, a duration (`30m`, `3h`, `2d`; `d` for days is an ntfy extension, `util.ParseDuration`),
an RFC 3339 time, and finally natural language ("tomorrow, 10am"), interpreted in `X-Timezone` if
given.

| Condition | Error |
|---|---|
| `cache=no` | 40002: a delayed message has nowhere to live |
| `X-Email` also set | 40003: the address cannot be persisted |
| `X-Call` also set | 40037: the number cannot be persisted |
| Unparseable | 40004 |
| Earlier than now + `MessageDelayMin` (10s) | 40005 |
| Later than now + `message-delay-limit` (default 3 days) | 40006 |

A delayed message is written with `published = false`, skipped by normal reads, and sent later by
the delayed sender (`MessagesDue`, then `MarkPublished`). Publishing again with the same
`sequence_id` replaces a still-scheduled message, and `message_delete` removes it.

## Subscribing

Four streaming endpoints, all `GET`, all taking a comma-separated list of topics (each
`[-_A-Za-z0-9]{1,64}`, at most 150 distinct topics per request, 40061):

| Path | Content type | Framing |
|---|---|---|
| `/<topics>/json` | `application/x-ndjson` | One JSON object per line |
| `/<topics>/sse` | `text/event-stream` | `data: <json>`; non-message events get an `event:` line, so the browser's `onmessage` does not fire for them |
| `/<topics>/raw` | `text/plain` | Message body only, newlines replaced by spaces; other events are empty lines |
| `/<topics>/ws` | WebSocket | JSON frames; ping/pong instead of `keepalive` |

`/<topics>/auth` is not a subscription: it runs the same read check and returns `{"success":true}`.

All of them go through `authorizeTopicRead` and `parseSubscribeParams`:

| Parameter | Aliases | Meaning |
|---|---|---|
| `poll` | `X-Poll`, `po` | Return cached messages and close |
| `since` | `X-Since`, `si` | Replay start, see below |
| `scheduled` | `X-Scheduled`, `sched` | Include not-yet-published (delayed) messages |
| `id` | `X-ID` | Exact match on message ID |
| `message` | `X-Message`, `m` | Exact match on body |
| `title` | `X-Title`, `t` | Exact match on title |
| `priority` | `X-Priority`, `prio`, `p` | Comma-separated; matches any |
| `tags` | `X-Tags`, `tag`, `ta` | Comma-separated; the message must have **all** of them |

Filters are applied in-process on every frame after the cache query, so they save bandwidth, not
database work. Credentials can also be passed as `?auth=`, see below.

### Replay size

A replay keeps the newest messages per topic that fit `Config.MessagePollSizeLimit` (10 MiB of
`Message.Size()`, always at least one message). If any topic was cut, the response carries
`X-Messages-Truncated: 1` (set before the first write; a no-op on `/ws`). A poll replay is also
charged to the visitor's daily bandwidth budget (429 with 42905 when exhausted). Results from all
topics are merged and **stable**-sorted by `time`, so each topic's own same-second messages keep
their order (#1297), while the order between topics within one second is not defined.

### `since` semantics

`parseSince` (`server/server.go`) returns a `model.SinceMarker`:

| Input | Marker | Effect |
|---|---|---|
| absent, `poll=1` | `SinceAllMessages` | Everything cached |
| absent, streaming | `SinceNoMessages` | Live only |
| `all` | `SinceAllMessages` | Everything cached |
| `none` | `SinceNoMessages` | Nothing cached |
| `latest` | `SinceLatestMessage` | The single newest message |
| a valid message ID (12 alphanumerics) | `NewSinceID` | What came after that message |
| an integer | `NewSinceTime` | Everything at or after that Unix time |
| a Go duration (`10m`, `3h`) | `NewSinceTime(now - d)` | Relative window |
| anything else | 40008 | |

The sentinels are values: `SinceAllMessages` is `{Unix(0), ""}`, `SinceNoMessages` is
`{Unix(1), ""}`, `SinceLatestMessage` is `{Unix(0), "latest"}`. "All" is simply a time-based query
with `time >= 0`.

**since=<id>.** Message IDs are random, not sortable, so the cache resolves the ID to its row id
and time once (`Cache.ResolveMarker`, which asks a replica and then the primary), and
`MessagesAfterMarker` returns rows matching
`id > marker row id OR (time >= marker time AND id <> marker row id)`. The time branch catches
messages published in the marker's second or later but written before it (batches and cluster
nodes write independently); the cost is an occasional duplicate, which clients drop by ID. Times
are whole seconds on purpose, to absorb small clock skew between nodes.

If the marker is unknown (not written yet, or expired), `sinceForTopic`
(`server/server_tail.go`) replays from shortly before the moment the message went out on this
node, if it did; otherwise the replay is the topic's **entire retained history**. A client that
stores a since-ID across a long offline period can therefore get a full replay rather than an
empty result, so clients must de-duplicate.

## Message IDs

`model.GenerateMessageID()` is `util.RandomString(12)` over the 62-character alphanumeric charset.
`randomStringPrefixWithCharset` (`util/util.go`) uses `crypto/rand` with rejection sampling
(`limit := 256 - (256 % len(charset))`), so the distribution is unbiased.

- **Not sortable**: no time component. Ordering comes from the autoincrement `id` column and
  `time`.
- **No uniqueness constraint**: `mid` has a non-unique index in both backends. Uniqueness rests on
  62^12 (about 3.2e21) values of CSPRNG output; there is no collision check.
- `ValidMessageID` checks only length and charset, which is how `parseSince` tells an ID from a
  timestamp.
- `SequenceID` uses the topic charset (`sequenceIDRegex = topicRegex`, `^[-_A-Za-z0-9]{1,64}$`) and
  defaults to the message's own ID.

## Schema versioning

Every store migrates through `schema.Migrate` (`db/schema/schema.go`) in one transaction. On
Postgres all stores share one database and one `schema_version(store, version, min_version)`
table keyed by store name, and setup is serialized across nodes with a transaction-scoped advisory
lock. On SQLite each store is its own file with `schemaVersion(id = 1, version, min_version)`.
A missing version row means a fresh database: the store's create function runs and records the
target version. Migrations are `{From, To}` steps that must be contiguous; an older binary on a
newer schema warns and continues, so **migrations must be additive** (see the package comment).

| Store (`schema_version.store`) | Package | SQLite file | Current version | Oldest migratable |
|---|---|---|---|---|
| `message` | `message` | `cache-file` | 17 | SQLite 1, Postgres 14 |
| `user` (also holds `usage` and `topic`) | `user` | `auth-file` | 11 | SQLite 1, Postgres 6 |
| `webpush` | `webpush` | `web-push-file` | 1 | n/a |
| `node_registry` | `cluster/registry` | Postgres only | 2 | 1 |

Postgres support arrived at message cache version 14 and user version 6, so the Postgres migration
maps start there; an older Postgres database fails with "cannot find ... migration step".

## The message cache schema

The two dialects come from separate DDL constants (`message/cache_sqlite_schema.go`,
`message/cache_postgres_schema.go`). **Table names and one column name differ**, which is why
every query is written per backend.

| | SQLite | Postgres |
|---|---|---|
| Messages table | `messages` | `message` |
| Stats table | `stats` | `message_stats` |
| Version table | `schemaVersion` | `schema_version` (shared) |
| User column | `user` | `user_id` |

### Columns

| Column | SQLite | Postgres |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` |
| `mid` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `sequence_id` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `time` | `INT NOT NULL` | `BIGINT NOT NULL` |
| `event` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `expires` | `INT NOT NULL` | `BIGINT NOT NULL` |
| `topic`, `message`, `title` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `priority` | `INT NOT NULL` | `INT NOT NULL` |
| `tags`, `click`, `icon`, `actions` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `attachment_name`, `attachment_type` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `attachment_size`, `attachment_expires` | `INT NOT NULL` | `BIGINT NOT NULL` |
| `attachment_url` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `attachment_deleted` | `INT NOT NULL` | `BOOLEAN NOT NULL DEFAULT FALSE` |
| `sender` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| user | `user TEXT NOT NULL` | `user_id TEXT NOT NULL` |
| `content_type`, `encoding` | `TEXT NOT NULL` | `TEXT NOT NULL` |
| `published` | `INT NOT NULL` | `BOOLEAN NOT NULL DEFAULT FALSE` |
| `claimed_at` | (none) | `BIGINT NOT NULL DEFAULT 0` |

Booleans are `INT` in SQLite (`= 0` / `= 1`) and real `BOOLEAN` in Postgres; Go passes native
`bool` either way. Tags are comma-joined, actions JSON-marshalled (`insertMessageArgs` in
`message/cache.go`). `published` is computed at insert as `m.Time <= now`, which is how a delayed
message is marked; `attachment_deleted` is always inserted false. `claimed_at` exists only on
Postgres: the delayed sender's claim query (`FOR UPDATE SKIP LOCKED`) stamps it so several nodes
can send due messages without overlap.

The stats table holds one row, `('messages', <count>)`, incremented by deltas (`AddStats`).

### Indexes

| SQLite | Postgres | Serves |
|---|---|---|
| `idx_mid (mid)` | `idx_message_mid (mid)` | Lookup by ID, `ResolveMarker` |
| `idx_sequence_id (sequence_id)` | `idx_message_sequence_id (sequence_id)` | Scheduled message replace/delete by sequence ID |
| `idx_topic_published_time (topic, published, time)`, `idx_topic`, `idx_time` | `idx_message_topic_published_time (topic, published, time, id)` | Time-based replays, `latest`, the time branch of since-ID |
| (rowid) | `idx_message_topic_published_id (topic, published, id)` | The id branch of since-ID |
| `idx_expires (expires)` | `idx_message_published_expires (published, expires)` | Expiry deletion |
| `idx_attachment_expires (attachment_expires)` | `idx_message_attachment_expires (attachment_expires) WHERE attachment_deleted = FALSE` | Attachment expiry sweep |
| `idx_sender (sender)` | `idx_message_sender_attachment_expires (sender, attachment_expires) WHERE user_id = ''` | Anonymous attachment quota |
| `idx_user (user)` | `idx_message_user_id_attachment_expires (user_id, attachment_expires)` | Per-user attachment quota |

### Key queries

- **Insert**: all 24 columns per row (`insertMessageColumns`), in one transaction per batch. SQLite
  uses multi-row `INSERT ... VALUES` of up to 1000 rows; Postgres one
  `INSERT ... SELECT * FROM unnest($1::TEXT[], ...)` per batch, a fixed statement so pgx's statement
  cache does not grow.
- **Since time**: `WHERE topic = ? AND time >= ? AND published = 1 ORDER BY time DESC, id DESC`;
  the reader keeps the newest rows within the size budget and reverses them. With
  `scheduled=1` the published filter is dropped.
- **Since ID**: a `UNION ALL` of `time >= marker time AND id <> marker id` and
  `id > marker id AND time + 0 < marker time` (the `+ 0` keeps the planner from using the time
  index for the second branch). With `scheduled=1`, a third branch adds all unpublished rows.
- **Due**: SQLite selects `time <= now AND published = 0`; Postgres claims up to 1000 such rows
  whose `claimed_at` is older than the claim timeout.
- **Tail**: `selectMessagesAfterRowID` reads rows after the cursor plus the listed hole ids, in id
  order, from the primary (`message/tail.go`).
- **Expiry**: `DELETE ... WHERE mid IN (SELECT mid ... WHERE expires <= ? AND published = 1 LIMIT ?)`.

The select list has 22 columns (`id`, `attachment_deleted`, `published` and `claimed_at` are not
read back, except where the tail appends `id, published`). It must stay in sync with
`readMessage`'s scan order; there is no column-name binding.

### Message cache migrations

SQLite (`sqliteMigrations`, versions 1 to 17; databases older than version 1, from before
v1.10.0, cannot be migrated). Selected steps:

| Step | Change |
|---|---|
| 1 -> 2 | Add `published` |
| 4 -> 5 | **Table rebuild**: the string message ID moves out of the primary key into `mid`; `id` becomes autoincrement |
| 6 -> 7 | Rename `attachment_owner` to `sender` |
| 9 -> 10 | Add `user`, `attachment_deleted`, `expires`; backfill `expires = time + cache-duration` |
| 10 -> 11 | Add the `stats` table |
| 13 -> 14 | Add `sequence_id` and `event` (default `message`): updates, deletes, clears |
| 14 -> 15, 15 -> 16 | No-ops on SQLite (Postgres index, Postgres claim column) |
| 16 -> 17 | Add `idx_topic_published_time` |

Postgres (`postgresMigrations`): 14 -> 15 adds the partial `idx_message_attachment_expires`,
15 -> 16 adds `claimed_at`, 16 -> 17 adds `idx_message_topic_published_id`.

Startup path: `server.New` -> `createMessageCache` -> `message.NewPostgresStore` or
`message.NewSQLiteStore` -> `schema.Migrate`. The in-memory stores use the same SQLite path with a
`file:<random>?mode=memory&cache=shared` DSN, so tests run the real migrations.

## The user database schema

SQLite DDL is in `user/manager_sqlite_schema.go`, Postgres in `user/manager_postgres_schema.go`.

**Portability gotcha:** the username column is `user` in SQLite and `user_name` in Postgres, and
the table is `user` in SQLite but `"user"` (quoted, a reserved word) in Postgres.

### `user`

| Column | SQLite | Postgres |
|---|---|---|
| `id` | `TEXT PRIMARY KEY` (`u_` + 12 chars) | same |
| `tier_id` | `TEXT`, FK to `tier(id)` | `TEXT REFERENCES tier(id)` |
| username | `user TEXT NOT NULL`, unique index `idx_user` | `user_name TEXT NOT NULL UNIQUE` |
| `pass` | `TEXT NOT NULL` (bcrypt) | same |
| `role` | `TEXT CHECK (role IN ('anonymous','admin','user')) NOT NULL` | same |
| `prefs` | `JSON NOT NULL DEFAULT '{}'` | `JSONB NOT NULL DEFAULT '{}'` |
| `sync_topic` | `TEXT NOT NULL` (`st_` + 16 chars) | same |
| `provisioned` | `INT NOT NULL` | `BOOLEAN NOT NULL` |
| `stats_messages`, `stats_emails`, `stats_calls` | `INT NOT NULL DEFAULT (0)` | `BIGINT NOT NULL DEFAULT 0` |
| `stripe_customer_id`, `stripe_subscription_id` | `TEXT`, unique indexes | `TEXT UNIQUE` |
| `stripe_subscription_status`, `stripe_subscription_interval` | `TEXT` | `TEXT` |
| `stripe_subscription_paid_until`, `stripe_subscription_cancel_at` | `INT` | `BIGINT` |
| `created` | `INT NOT NULL` | `BIGINT NOT NULL` |
| `deleted` | `INT` nullable | `BIGINT` nullable |

The `stats_*` columns are legacy: since user schema version 10 the daily counts live in the
`usage` table, and the columns are zeroed but kept (migrations are additive). `deleted` is a
soft-delete timestamp; soft-deleted users are hard-deleted after 7 days. The anonymous user
`u_everyone` (username `*`) is seeded at creation.

### `tier`

`id` (PK, `ti_` prefix), `code`, `name`, then `messages_limit`, `messages_expiry_duration`,
`emails_limit`, `calls_limit`, `reservations_limit`, `attachment_file_size_limit`,
`attachment_total_size_limit`, `attachment_expiry_duration`, `attachment_bandwidth_limit` (all
`INT NOT NULL` in SQLite, `BIGINT NOT NULL` in Postgres), plus nullable `stripe_monthly_price_id`
and `stripe_yearly_price_id`. Durations are stored in seconds. `code` and both price IDs are
unique: named indexes in SQLite, inline `UNIQUE` in Postgres.

### `user_access`

`user_id`, `topic`, `read`, `write`, `owner_user_id`, `provisioned`; primary key
`(user_id, topic)`, also the upsert conflict target. Both foreign keys cascade on delete. `topic`
is stored as a `LIKE` pattern: `*` becomes `%`, and `_` is escaped as `\_`.

A latent wart: SQLite declares `owner_user_id INT` while Postgres declares
`TEXT REFERENCES "user"(id)`. User IDs are text, so the SQLite type is wrong and only works through
SQLite's type affinity; it has been carried through table rebuilds and is best left alone.

### `user_token`

`user_id` (FK, cascade), `token`, `label`, `last_access`, `last_origin`, `expires`, `provisioned`.
Primary key `(user_id, token)`, plus a **unique** index on `token` alone (`idx_user_token` in
SQLite, inline `UNIQUE` in Postgres). That index is the hot path of every `Bearer` request.

### `user_phone`, `user_email`, `user_magic_link`

- `user_phone(user_id, phone_number)`, PK on both. (SQLite step 8 -> 9 rebuilds it to repair a
  foreign key that an earlier rename had pointed at a dropped table.)
- `user_email(user_id, email, is_primary)`, PK `(user_id, email)`. Two **partial unique** indexes:
  `idx_user_email_primary_user` (one primary address per user) and `idx_user_email_primary_addr`
  (an address is primary on at most one account). The predicates are `WHERE is_primary = 1` in
  SQLite and `WHERE is_primary` in Postgres because the column is `INT` versus `BOOLEAN`.
- `user_magic_link(token_hash PK, kind, user_id, email, expires, created)`, index
  `idx_magic_link_user_kind (user_id, kind)`; email verification and password reset links.

### `usage` (package `limit`)

`key` (PK: the visitor key, `ip:<prefix>` or `user:<id>`), `period_start`, `messages`, `emails`,
`calls`, `updated_at`, index on `updated_at`. One row per key, reused across the key's 24-hour
periods. Nodes upsert deltas (same period: add; newer period: replace; older: drop), read back rows
changed since their watermark, and prune rows not written for 25 hours. See `limit/store_*.go`.

### `topic` (package `topic`)

`topic` (PK), `rate_visitor_user_id`, `rate_visitor_ip`, `first_seen_at`, `subscribed_at`,
`updated_at`, index on `updated_at`. State of UnifiedPush topics: who pays for their pushes and when
a subscriber was last active, merged per field (earliest `first_seen_at`, latest `subscribed_at`,
last written rate visitor). Rows are pruned after 7 days. See `topic/store_*.go`.

### Cluster tables

With clustering, the Postgres database also holds `node_registry` (`node_id`, `advertise_url`,
`last_heartbeat`, `role`), store `node_registry` in `schema_version`. Cluster internals are
documented in [cluster.md](cluster.md).

### User DB migrations

Current version 11 in both dialects. SQLite steps go from 1 to 11, Postgres from 6 to 11. Notable
steps:

| Step | Change |
|---|---|
| 1 -> 2 | Renames the old `user` table aside, regenerates IDs and sync topics in Go, folds the legacy `access` table into `user_access` |
| 5 -> 6 | Adds the `provisioned` columns by full table rebuild (relies on foreign keys being off, so migrations run before the startup queries) |
| 6 -> 7 | `user_email` |
| 7 -> 8 | `is_primary`, its partial unique indexes, `user_magic_link` |
| 8 -> 9 | SQLite only: rebuild `user_phone` to repair its foreign key; no-op on Postgres |
| 9 -> 10 | `usage` table; zero the `stats_*` columns |
| 10 -> 11 | `topic` table |

Every step runs inside the single `schema.Migrate` transaction, on Postgres too.

## Auth on the wire

`maybeAuthenticate` (`server/server_auth.go`) always returns a visitor, even on failure. Without a
user manager it returns an IP visitor immediately.

### Header formats

Only `Basic` and `Bearer` are recognized (`supportedAuthHeader`, case-insensitive; an empty value is
not supported). Other schemes such as `WebPush` or `vapid` arrive on the same requests and are
treated as anonymous rather than as failed auth.

- **Bearer**: the token goes to `AuthenticateToken`, which length-checks and looks it up. An
  asynchronous `EnqueueTokenUpdate` records last access and origin IP, batched by the manager.
- **Basic**: `username:password`, where the username may also be a verified primary email
  (`Authenticate` uses `userByNameOrEmail`). **If the username is empty, the password is treated
  as a token**, which is what makes `curl -u :tk_...` work.

### The `auth` query parameter

Browsers cannot set headers on `EventSource` or the WebSocket handshake, so `readAuthHeader` also
accepts `?auth=` or `?authorization=`: **base64url without padding** of the entire header value,
which for Basic auth is itself base64:

```
?auth=base64url("Basic " + base64("user:pass"))
```

When present, the query parameter overrides the real header. It puts a credential into the URL,
where it can end up in access logs and browser history; treat it as a browser workaround.

The auth-failure bucket is checked before the database is touched (42909), and a failure calls
`vip.AuthFailed()`.

### Tokens

| Property | Value | Where |
|---|---|---|
| Prefix | `tk_` | `tokenPrefix` (`user/manager.go`) |
| Length | 32 including the prefix, so 29 random chars | `tokenLength` |
| Alphabet | lowercase alphanumeric | `GenerateToken` -> `util.RandomLowerStringPrefix` |
| Max per user | 60, excess trimmed at creation | `tokenMaxCount`, `createTokenTx` |
| Validation | `^tk_[-_A-Za-z0-9]{29}$` | `allowedTokenRegex` (`user/util.go`) |

Lowercase only because tokens appear in `<topic>+<token>@<domain>` email addresses, which are
case-insensitive. The validation regex is looser than the generator.

**Access tokens are stored in plaintext**: the lookup is `WHERE tk.token = ? AND (tk.expires = 0
OR tk.expires >= ?)`. This is a legacy trade-off (changing it is a breaking migration), but it means
a database read yields working credentials. `expires = 0` means "never"; provisioned tokens are
stored that way and cannot be changed through the API (40906).

**Magic-link tokens are hashed**: 48 base62 chars (`linkTokenLength`), and only the hex SHA-256
digest is persisted (`hashToken`); the comment there gives the rationale. Expired links are reaped
hourly.

### Passwords

bcrypt via `HashPassword` with `Config.AuthBcryptCost` (default `DefaultUserPasswordBcryptCost = 10`).
Pre-hashed passwords are validated by `ValidPasswordHash`: prefix `$2a$`, `$2b$` or `$2y$` and a
cost at least the minimum, else `ErrPasswordHashInvalid` / `ErrPasswordHashWeak`.

`Authenticate` compares against `userAuthIntentionalSlowDownHash` when the user does not exist or
is soft-deleted, to flatten the timing signal. That hash's cost must track the default cost, and
nothing enforces the coupling.

### Permissions and roles

`Permission` (`user/types.go`) is a bit set declared with `iota`: `PermissionDenyAll = 0`,
`PermissionRead = 1`, `PermissionWrite = 2`, `PermissionReadWrite = 3`. `ParsePermission` accepts
`read-write`/`rw`, `read-only`/`read`/`ro`, `write-only`/`write`/`wo`, and `deny-all`/`deny`/`none`.

`Role` matches the SQL `CHECK` constraint: `admin`, `user`, `anonymous`. `AllowedRole` permits only
`user` and `admin` for new accounts, reserving `anonymous` for `u_everyone`.
