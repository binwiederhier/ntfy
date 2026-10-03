# Topic visibility & discovery
Topics in ntfy are implicit, global strings: there is no notion of who "owns" a topic, and a user on a
shared instance cannot discover topics that other users have created unless they are told about them
out of band. **Topic visibility** adds an opt-in way for an instance's users to publish the existence
of a reserved topic to everyone else.

A reserved topic is **private** by default (unchanged behavior) and can be switched to **shared**.
Shared topics are listed by a discovery API, and switching a topic to shared publishes a notification
on the reserved [`~directory`](#the-directory-feed) topic so clients can react immediately.

!!! note
    Visibility only applies to **reserved topics** (topics tied to a user through
    [reservations](config.md#reservations)). Unreserved topics are unaffected: they remain exactly as
    before, with no owner and no visibility.

## The private/shared model
- **`private`** (default): the topic behaves exactly as it always has. Nobody learns about it by
  browsing; only the owner (and admins, and anyone matching an ACL grant) knows it exists.
- **`shared`**: the topic is listed by `GET /v1/topics?visibility=shared` for all authenticated users,
  and a one-line announcement is published to the `~directory` feed.

Visibility is a property of the reservation, not of the ACL. Setting a topic back to `private` stops
new discovery announcements and removes it from the listing, but it does **not** automatically narrow
the topic's `everyone` access rule (see [Sharing and access](#sharing-and-access) below).

## Prerequisites
- An auth backend must be configured (`auth-file` or `database-url`), so that users, reservations and
  ACLs exist. Without it, the discovery endpoints are not available.
- The topic must be **reserved** by a user. Reserving topics requires the reservation feature to be
  enabled (`enable-reservations: true`) and, for non-admin users, a [tier](config.md#tiers) with a
  non-zero reservation limit.

## API reference

### List shared topics
```
GET /v1/topics?visibility=shared
```

Returns the reserved topics that their owners have marked as shared. Readable by **any authenticated
user** (regular users as well as admins).

| | |
|---|---|
| **Auth** | Bearer token or Basic auth of any user (admin or regular). Anonymous requests are rejected. |
| **Query** | `visibility=shared` (required; any other value is a `400`) |
| **Response** | `200 OK` |

``` json
{
  "topics": [
    { "topic": "announcements", "owner": "phil" },
    { "topic": "home-automation", "owner": "ben" }
  ]
}
```

Error responses:

| HTTP | Code | When |
|---|---|---|
| `400` | `40000` | `visibility` is missing or not `shared` |
| `401` | `40101` | Not authenticated |
| `404` | `40401` | No auth backend configured on the server |

!!! note
    ntfy does not track a reservation's creation time, so the response contains only the topic name and
    the owner's username.

### Change a topic's visibility
```
PATCH /v1/topics/<topic>
```

``` json
{ "visibility": "shared" }
```

The body value is either `"shared"` or `"private"`. The response echoes the applied value:

``` json
{ "topic": "announcements", "visibility": "shared" }
```

Only the reservation's **owner** may change its visibility; admins may also change it on behalf of the
owner. Setting a topic to `shared` also applies the [access upgrade](#sharing-and-access)
described below, and publishes an announcement on `~directory`.

| | |
|---|---|
| **Auth** | The owner of the reservation, or an admin |
| **Body** | `{"visibility": "private"\|"shared"}` |
| **Response** | `200 OK` |

Error responses:

| HTTP | Code | When |
|---|---|---|
| `400` | `40000` | Invalid body / unknown `visibility` value |
| `400` | `40009` | Invalid topic name |
| `401` | `40101` | Not authenticated |
| `403` | `40301` | Authenticated, but not the owner and not an admin |
| `404` | `40401` | The topic is not reserved by anyone (nothing to change) |

## The `~directory` feed
When a topic is marked as shared — either at reservation creation or via the `PATCH` endpoint — the
server publishes a short message to the reserved system topic `~directory`:

```
topic `announcements` was shared by `phil`
```

`~directory` follows ntfy's existing convention for internal topics (alongside `~control` and `~poll`):
the `~` prefix is **not** part of the normal topic character set, so it can never be reserved or
created by a regular account. Access is controlled by the server: any **authenticated** user may
subscribe to `~directory` (anonymous users are not granted access), and only the server itself
publishes to it.

Clients that want live discovery can subscribe to `~directory`; the web app does this automatically for
logged-in users and surfaces each announcement as a toast.

=== "Command line (curl)"
    ```
    curl -s "https://ntfy.example.com/~directory/json" -H "Authorization: Bearer tk_..."
    ```

## Web UI
In the web app, the feature surfaces in three places, all available only to logged-in users:

- **Discover** (left-hand navigation): lists the topics returned by
  `GET /v1/topics?visibility=shared` **that you are not already subscribed to**, showing the topic name
  and its owner. Each entry has a **Subscribe** button that subscribes you using the normal subscribe
  flow; subscribing removes the entry immediately, without a reload (the list is filtered against your
  live subscription state). If every shared topic is already subscribed, Discover shows a "No new shared
  topics" message. The list refreshes when a new shared topic is announced on `~directory`.
- **Subscribe dialog** (the **+** → *Subscribe to topic* flow): when you turn on **Reserve topic and
  configure access**, the same dialog also offers **Make discoverable (shared)**. The access level and
  discoverability are independent: choosing a wider access level does not turn discoverability on, and
  the discoverable switch is **off by default** (matching the private default). Reserving with it on
  marks the topic shared as part of the same action, so no separate trip through the per-topic menu is
  needed. Setting it off leaves the reservation private, exactly as before.
- **Per-topic menu** (the ⋮ menu on a subscribed topic you own): **Share topic (discoverable)** marks
  the topic as shared; **Make topic private** switches it back. This remains the way to change a
  topic's visibility **after** it has been reserved (e.g. reserve privately now, share it later). The
  toggle only appears for topics you have reserved.

## Sharing and access
A reserved topic carries an `everyone` access rule that controls what other users may do with it. A
`deny-all` rule would make a "shared" topic technically discoverable but useless: subscribers could see
it and subscribe, yet receive nothing. To avoid that silent dead end, **marking a topic as shared
automatically upgrades a `deny-all` `everyone` rule to `read-only`**, in the same transaction.

- `read-only` and `read-write` rules are left as they are — only `deny-all` is upgraded.
- Switching a topic back to `private` does **not** revert the access rule. Visibility and ACL are
  related but distinct concepts, and narrowing access that an owner deliberately widened for other
  reasons could be surprising. The change is intentionally one-way.

## Backfilling existing topics
If a topic was marked shared before this upgrade logic existed, and its `everyone` rule is still
`deny-all`, it can be repaired with the server-side command:

```
ntfy topics fix-shared-read
```

The command upgrades the `everyone` rule of every affected shared topic from `deny-all` to `read-only`,
prints each change (`topic <name> (owner <user>): everyone deny-all -> read-only`) and a count, and is
idempotent. It is a one-off data repair, not a schema migration.

## Migration and compatibility
This feature changes the auth/user database schema from **version 9 to version 10**. The migration is
additive:

- A new **`topics`** table is created (`topic` primary key, `owner_user_id`, `visibility`), storing the
  reservation entity and its visibility.
- Existing reservations are backfilled into `topics` as `private`.
- The `user_access` table is **unchanged** — it continues to hold ACL grants only.

Because the default visibility is `private`, the migration preserves all existing behavior:

- **Anonymous and unreserved topics** are completely unaffected; there is no new restriction, listing,
  or notification for them.
- **Existing reservations** become `private` and are not listed until their owner opts in.
- **Existing ACLs** (`user_access`) are untouched, so current access rules keep working exactly as
  before.

## Design rationale
Two implementation choices are worth calling out for reviewers:

**A dedicated `topics` table rather than a column on `user_access`.** A reservation is represented in
ntfy as ACL rows (the owner's row plus an `everyone` row). Visibility is topic metadata, not an ACL
grant, so overloading `user_access` with it would mix concerns and make `user_access` diverge from
upstream. A separate `topics` table keeps `user_access` byte-for-byte upstream-compatible, gives the
reservation a real entity to hang future metadata on (creation time, descriptions, …), and keeps the
read/write ACL path unchanged. The `Reservations()` read joins the two.

**Reserving `~directory` with the `~` prefix.** The announcement feed needs a well-known name that a
regular account can never claim. ntfy already reserves `~control` and `~poll` for internal topics, and
the `~` character is deliberately outside the topic character set, so an account physically cannot
reserve or subscribe to a `~`-prefixed name through the normal paths. Reusing that convention gives us
a non-claimable name for free. The only supporting change is that the subscribe/auth path matchers
accept the `~` prefix so that authenticated clients can read `~directory`; the publish/delete paths do
not, so `~` topics stay read-only to clients. Access remains gated by the authorization layer, which
exposes only `~directory`.

## Open questions for maintainers
- **PostgreSQL not exercised in development.** The schema and queries are implemented for both SQLite
  and PostgreSQL, but only SQLite was run against a live database during development. PostgreSQL
  should be validated in CI before merge.
- **Extending the subscribe-path character set to `~`.** Accepting `~` in the subscribe/auth path
  matchers is a (small) change to the public topic syntax. An alternative would be to expose
  `~directory` through a dedicated route instead. We would welcome guidance on which is preferred.
- **Stored topic representation.** `topics.topic` currently reuses the same SQL-escaped representation
  as `user_access.topic` (underscores escaped) so the reservation join is a plain equality. Storing the
  raw topic name would be cleaner and could be revisited.
