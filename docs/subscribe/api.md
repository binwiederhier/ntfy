# Subscribe via API
You can create and subscribe to a topic in the [web UI](web.md), via the [phone app](phone.md), via the [ntfy CLI](cli.md),
or in your own app or script by subscribing the API. This page describes how to subscribe via API. You may also want to 
check out the page that describes how to [publish messages](../publish.md).

You can consume the subscription API as either a **[simple HTTP stream (JSON, SSE or raw)](#http-stream)**, or 
**[via WebSockets](#websockets)**. Both are incredibly simple to use.

## HTTP stream
The HTTP stream-based API relies on a simple GET request with a streaming HTTP response, i.e **you open a GET request and
the connection stays open forever**, sending messages back as they come in. There are three different API endpoints, which 
only differ in the response format:

* [JSON stream](#subscribe-as-json-stream): `<topic>/json` returns a JSON stream, with one JSON message object per line
* [SSE stream](#subscribe-as-sse-stream): `<topic>/sse` returns messages as [Server-Sent Events (SSE)](https://en.wikipedia.org/wiki/Server-sent_events), which
  can be used with [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
* [Raw stream](#subscribe-as-raw-stream): `<topic>/raw` returns messages as raw text, with one line per message

### Subscribe as JSON stream
Here are a few examples of how to consume the JSON endpoint (`<topic>/json`). For almost all languages, **this is the 
recommended way to subscribe to a topic**. The notable exception is JavaScript, for which the 
[SSE/EventSource stream](#subscribe-as-sse-stream) is much easier to work with.

=== "Command line (curl)"
    ```
    $ curl -s ntfy.sh/disk-alerts/json
    {"id":"SLiKI64DOt","time":1635528757,"event":"open","topic":"mytopic"}
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    {"id":"DGUDShMCsc","time":1635528787,"event":"keepalive","topic":"mytopic"}
    ...
    ```

=== "ntfy CLI"
    ```
    $ ntfy subcribe disk-alerts
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    ...
    ```

=== "HTTP"
    ``` http
    GET /disk-alerts/json HTTP/1.1
    Host: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: application/x-ndjson; charset=utf-8
    Transfer-Encoding: chunked
    
    {"id":"SLiKI64DOt","time":1635528757,"event":"open","topic":"mytopic"}
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    {"id":"DGUDShMCsc","time":1635528787,"event":"keepalive","topic":"mytopic"}
    ...
    ```

=== "Go"
    ``` go
    resp, err := http.Get("https://ntfy.sh/disk-alerts/json")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        println(scanner.Text())
    }
    ```

=== "Python"
    ``` python
    resp = requests.get("https://ntfy.sh/disk-alerts/json", stream=True)
    for line in resp.iter_lines():
      if line:
        print(line)
    ```

=== "PHP"
    ``` php-inline
    $fp = fopen('https://ntfy.sh/disk-alerts/json', 'r');
    if (!$fp) die('cannot open stream');
    while (!feof($fp)) {
        echo fgets($fp, 2048);
        flush();
    }
    fclose($fp);
    ```

### Subscribe as SSE stream
Using [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) in JavaScript, you can consume
notifications via a [Server-Sent Events (SSE)](https://en.wikipedia.org/wiki/Server-sent_events) stream. It's incredibly 
easy to use. Here's what it looks like. You may also want to check out the [full example on GitHub](https://github.com/binwiederhier/ntfy/tree/main/examples/web-example-eventsource).

=== "Command line (curl)"
    ```
    $ curl -s ntfy.sh/mytopic/sse
    event: open
    data: {"id":"weSj9RtNkj","time":1635528898,"event":"open","topic":"mytopic"}
    
    data: {"id":"p0M5y6gcCY","time":1635528909,"event":"message","topic":"mytopic","message":"Hi!"}
    
    event: keepalive
    data: {"id":"VNxNIg5fpt","time":1635528928,"event":"keepalive","topic":"test"}
    ...
    ```

=== "HTTP"
    ``` http
    GET /mytopic/sse HTTP/1.1
    Host: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: text/event-stream; charset=utf-8
    Transfer-Encoding: chunked

    event: open
    data: {"id":"weSj9RtNkj","time":1635528898,"event":"open","topic":"mytopic"}
    
    data: {"id":"p0M5y6gcCY","time":1635528909,"event":"message","topic":"mytopic","message":"Hi!"}
    
    event: keepalive
    data: {"id":"VNxNIg5fpt","time":1635528928,"event":"keepalive","topic":"test"}
    ...
    ```

=== "JavaScript"
    ``` javascript
    const eventSource = new EventSource('https://ntfy.sh/mytopic/sse');
    eventSource.onmessage = (e) => {
      console.log(e.data);
    };
    ```

### Subscribe as raw stream
The `/raw` endpoint will output one line per message, and **will only include the message body**. It's useful for extremely
simple scripts, and doesn't include all the data. Additional fields such as [priority](../publish.md#message-priority), 
[tags](../publish.md#tags--emojis--) or [message title](../publish.md#message-title) are not included in this output 
format. Keepalive messages are sent as empty lines.

=== "Command line (curl)"
    ```
    $ curl -s ntfy.sh/disk-alerts/raw
    
    Disk full
    ...
    ```

=== "HTTP"
    ``` http
    GET /disk-alerts/raw HTTP/1.1
    Host: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: text/plain; charset=utf-8
    Transfer-Encoding: chunked

    Disk full
    ...
    ```

=== "Go"
    ``` go
    resp, err := http.Get("https://ntfy.sh/disk-alerts/raw")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        println(scanner.Text())
    }
    ```

=== "Python"
    ``` python 
    resp = requests.get("https://ntfy.sh/disk-alerts/raw", stream=True)
    for line in resp.iter_lines():
      if line:
        print(line)
    ```

=== "PHP"
    ``` php-inline
    $fp = fopen('https://ntfy.sh/disk-alerts/raw', 'r');
    if (!$fp) die('cannot open stream');
    while (!feof($fp)) {
        echo fgets($fp, 2048);
        flush();
    }
    fclose($fp);
    ```

## WebSockets
You may also subscribe to topics via [WebSockets](https://en.wikipedia.org/wiki/WebSocket), which is also widely 
supported in many languages. Most notably, WebSockets are natively supported in JavaScript. On the command line, 
I recommend [websocat](https://github.com/vi/websocat), a fantastic tool similar to `socat` or `curl`, but specifically
for WebSockets.  

The WebSockets endpoint is available at `<topic>/ws` and returns messages as JSON objects similar to the 
[JSON stream endpoint](#subscribe-as-json-stream). 

=== "Command line (websocat)"
    ```
    $ websocat wss://ntfy.sh/mytopic/ws
    {"id":"qRHUCCvjj8","time":1642307388,"event":"open","topic":"mytopic"}
    {"id":"eOWoUBJ14x","time":1642307754,"event":"message","topic":"mytopic","message":"hi there"}
    ```

=== "HTTP"
    ``` http
    GET /disk-alerts/ws HTTP/1.1
    Host: ntfy.sh
    Upgrade: websocket
    Connection: Upgrade

    HTTP/1.1 101 Switching Protocols
    Upgrade: websocket
    Connection: Upgrade
    ...
    ```

=== "Go"
    ``` go
    import "github.com/gorilla/websocket"
	ws, _, _ := websocket.DefaultDialer.Dial("wss://ntfy.sh/mytopic/ws", nil)
	messageType, data, err := ws.ReadMessage()
    ...
    ```

=== "JavaScript"
    ``` javascript
    const socket = new WebSocket('wss://ntfy.sh/mytopic/ws');
    socket.addEventListener('message', function (event) {
        console.log(event.data);
    });
    ```

## Advanced features

### Poll for messages
You can also just poll for messages if you don't like the long-standing connection using the `poll=1`
query parameter. The connection will end after all available messages have been read. This parameter can be
combined with `since=` (defaults to `since=all`).

```
curl -s "ntfy.sh/mytopic/json?poll=1"
```

### Fetch cached messages
Messages may be cached for a couple of hours (see [message caching](../config.md#message-cache)) to account for network
interruptions of subscribers. If the server has configured message caching, you can read back what you missed by using 
the `since=` query parameter. It takes a duration (e.g. `10m` or `30s`), a Unix timestamp (e.g. `1635528757`),
a message ID (e.g. `nFS3knfcQ1xe`), or `all` (all cached messages).

```
curl -s "ntfy.sh/mytopic/json?since=10m"
curl -s "ntfy.sh/mytopic/json?since=1645970742"
curl -s "ntfy.sh/mytopic/json?since=nFS3knfcQ1xe"
```

### Fetch scheduled messages
Messages that are [scheduled to be delivered](../publish.md#scheduled-delivery) at a later date are not typically 
returned when subscribing via the API, which makes sense, because after all, the messages have technically not been 
delivered yet. To also return scheduled messages from the API, you can use the `scheduled=1` (alias: `sched=1`) 
parameter (makes most sense with the `poll=1` parameter):

```
curl -s "ntfy.sh/mytopic/json?poll=1&sched=1"
```

### Filter messages
You can filter which messages are returned based on the well-known message fields `id`, `message`, `title`, `priority` and
`tags`. Here's an example that only returns messages of high or urgent priority that contains the both tags 
"zfs-error" and "error". Note that the `priority` filter is a logical OR and the `tags` filter is a logical AND. 

```
$ curl "ntfy.sh/alerts/json?priority=high&tags=zfs-error"
{"id":"0TIkJpBcxR","time":1640122627,"event":"open","topic":"alerts"}
{"id":"X3Uzz9O1sM","time":1640122674,"event":"message","topic":"alerts","priority":4,
  "tags":["error", "zfs-error"], "message":"ZFS pool corruption detected"}
```

Available filters (all case-insensitive):

| Filter variable | Alias                     | Example                                       | Description                                                             |
|-----------------|---------------------------|-----------------------------------------------|-------------------------------------------------------------------------|
| `id`            | `X-ID`                    | `ntfy.sh/mytopic/json?poll=1&id=pbkiz8SD7ZxG` | Only return messages that match this exact message ID                   |
| `message`       | `X-Message`, `m`          | `ntfy.sh/mytopic/json?message=lalala`         | Only return messages that match this exact message string               |
| `title`         | `X-Title`, `t`            | `ntfy.sh/mytopic/json?title=some+title`       | Only return messages that match this exact title string                 |
| `priority`      | `X-Priority`, `prio`, `p` | `ntfy.sh/mytopic/json?p=high,urgent`          | Only return messages that match *any priority listed* (comma-separated) |
| `tags`          | `X-Tags`, `tag`, `ta`     | `ntfy.sh/mytopic?/jsontags=error,alert`       | Only return messages that match *all listed tags* (comma-separated)     |

### Subscribe to multiple topics
It's possible to subscribe to multiple topics in one HTTP call by providing a comma-separated list of topics 
in the URL. This allows you to reduce the number of connections you have to maintain:

```
$ curl -s ntfy.sh/mytopic1,mytopic2/json
{"id":"0OkXIryH3H","time":1637182619,"event":"open","topic":"mytopic1,mytopic2,mytopic3"}
{"id":"dzJJm7BCWs","time":1637182634,"event":"message","topic":"mytopic1","message":"for topic 1"}
{"id":"Cm02DsxUHb","time":1637182643,"event":"message","topic":"mytopic2","message":"for topic 2"}
```

### Authentication
Depending on whether the server is configured to support [access control](../config.md#access-control), some topics
may be read/write protected so that only users with the correct credentials can subscribe or publish to them.
To publish/subscribe to protected topics, you can:

* Use [basic auth](../publish.md#basic-auth), e.g. `Authorization: Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk`
* or use the [`auth` query parameter](../publish.md#query-param), e.g. `?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw`

Please refer to the [publishing documentation](../publish.md#authentication) for additional details.

## JSON message format
Both the [`/json` endpoint](#subscribe-as-json-stream) and the [`/sse` endpoint](#subscribe-as-sse-stream) return a JSON
format of the message. It's very straight forward:

**Message**:

| Field        | Required | Type                                              | Example                                               | Description                                                                                                                          |
|--------------|----------|---------------------------------------------------|-------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `id`         | ✔️       | *string*                                          | `hwQ2YpKdmg`                                          | Randomly chosen message identifier                                                                                                   |
| `time`       | ✔️       | *number*                                          | `1635528741`                                          | Message date time, as Unix time stamp                                                                                                |  
| `expires`    | ✔️       | *number*                                          | `1673542291`                                          | Unix time stamp indicating when the message will be deleted                                                                          |  
| `event`      | ✔️       | `open`, `keepalive`, `message`, or `poll_request` | `message`                                             | Message type, typically you'd be only interested in `message`                                                                        |
| `topic`      | ✔️       | *string*                                          | `topic1,topic2`                                       | Comma-separated list of topics the message is associated with; only one for all `message` events, but may be a list in `open` events |
| `message`    | -        | *string*                                          | `Some message`                                        | Message body; always present in `message` events                                                                                     |
| `title`      | -        | *string*                                          | `Some title`                                          | Message [title](../publish.md#message-title); if not set defaults to `ntfy.sh/<topic>`                                               |
| `tags`       | -        | *string array*                                    | `["tag1","tag2"]`                                     | List of [tags](../publish.md#tags-emojis) that may or not map to emojis                                                              |
| `priority`   | -        | *1, 2, 3, 4, or 5*                                | `4`                                                   | Message [priority](../publish.md#message-priority) with 1=min, 3=default and 5=max                                                   |
| `click`      | -        | *URL*                                             | `https://example.com`                                 | Website opened when notification is [clicked](../publish.md#click-action)                                                            |
| `actions`    | -        | *JSON array*                                      | *see [actions buttons](../publish.md#action-buttons)* | [Action buttons](../publish.md#action-buttons) that can be displayed in the notification                                             |
| `attachment` | -        | *JSON object*                                     | *see below*                                           | Details about an attachment (name, URL, size, ...)                                                                                   |

**Attachment** (part of the message, see [attachments](../publish.md#attachments) for details):

| Field     | Required | Type        | Example                        | Description                                                                                               |
|-----------|----------|-------------|--------------------------------|-----------------------------------------------------------------------------------------------------------|
| `name`    | ✔️       | *string*    | `attachment.jpg`               | Name of the attachment, can be overridden with `X-Filename`, see [attachments](../publish.md#attachments) |
| `url`     | ✔️       | *URL*       | `https://example.com/file.jpg` | URL of the attachment                                                                                     |  
| `type`    | -️       | *mime type* | `image/jpeg`                   | Mime type of the attachment, only defined if attachment was uploaded to ntfy server                       |
| `size`    | -️       | *number*    | `33848`                        | Size of the attachment in bytes, only defined if attachment was uploaded to ntfy server                   |
| `expires` | -️       | *number*    | `1635528741`                   | Attachment expiry date as Unix time stamp, only defined if attachment was uploaded to ntfy server         |

Here's an example for each message type:

=== "Notification message"
    ``` json
    {
        "id": "sPs71M8A2T",
        "time": 1643935928,
        "expires": 1643936928,
        "event": "message",
        "topic": "mytopic",
        "priority": 5,
        "tags": [
            "warning",
            "skull"
        ],
        "click": "https://homecam.mynet.lan/incident/1234",
        "attachment": {
            "name": "camera.jpg",
            "type": "image/png",
            "size": 33848,
            "expires": 1643946728,
            "url": "https://ntfy.sh/file/sPs71M8A2T.png"
        },
        "title": "Unauthorized access detected",
        "message": "Movement detected in the yard. You better go check"
    }
    ```


=== "Notification message (minimal)"
    ``` json
    {
        "id": "wze9zgqK41",
        "time": 1638542110,
        "expires": 1638543112,
        "event": "message",
        "topic": "phil_alerts",
        "message": "Remote access to phils-laptop detected. Act right away."
    }
    ```

=== "Open message"
    ``` json
    {
        "id": "2pgIAaGrQ8",
        "time": 1638542215,
        "event": "open",
        "topic": "phil_alerts"
    }
    ```

=== "Keepalive message"
    ``` json
    {
        "id": "371sevb0pD",
        "time": 1638542275,
        "event": "keepalive",
        "topic": "phil_alerts"
    }
    ```    

=== "Poll request message"
    ``` json
    {
        "id": "371sevb0pD",
        "time": 1638542275,
        "event": "poll_request",
        "topic": "phil_alerts"
    }
    ```

## List of all parameters
The following is a list of all parameters that can be passed **when subscribing to a message**. Parameter names are **case-insensitive**,
and can be passed as **HTTP headers** or **query parameters in the URL**. They are listed in the table in their canonical form.

| Parameter   | Aliases (case-insensitive) | Description                                                                     |
|-------------|----------------------------|---------------------------------------------------------------------------------|
| `poll`      | `X-Poll`, `po`             | Return cached messages and close connection                                     |
| `since`     | `X-Since`, `si`            | Return cached messages since timestamp, duration or message ID                  |
| `scheduled` | `X-Scheduled`, `sched`     | Include scheduled/delayed messages in message list                              |
| `id`        | `X-ID`                     | Filter: Only return messages that match this exact message ID                   |
| `message`   | `X-Message`, `m`           | Filter: Only return messages that match this exact message string               |
| `title`     | `X-Title`, `t`             | Filter: Only return messages that match this exact title string                 |
| `priority`  | `X-Priority`, `prio`, `p`  | Filter: Only return messages that match *any priority listed* (comma-separated) |
| `tags`      | `X-Tags`, `tag`, `ta`      | Filter: Only return messages that match *all listed tags* (comma-separated)     |
