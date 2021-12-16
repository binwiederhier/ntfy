# Subscribe via API
You can create and subscribe to a topic either in the [web UI](web.md), via the [phone app](phone.md), or in your own 
app or script by subscribing the API. This page describes how to subscribe via API. You may also want to check out the 
page that describes how to [publish messages](../publish.md).

The subscription API relies on a simple HTTP GET request with a streaming HTTP response, i.e **you open a GET request and
the connection stays open forever**, sending messages back as they come in. There are three different API endpoints, which 
only differ in the response format:

* [JSON stream](#subscribe-as-json-stream): `<topic>/json` returns a JSON stream, with one JSON message object per line
* [SSE stream](#subscribe-as-sse-stream): `<topic>/sse` returns messages as [Server-Sent Events (SSE)](https://en.wikipedia.org/wiki/Server-sent_events), which
  can be used with [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
* [Raw stream](#subscribe-as-raw-stream): `<topic>/raw` returns messages as raw text, with one line per message

## Subscribe as JSON stream
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

## Subscribe as SSE stream
Using [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) in JavaScript, you can consume
notifications via a [Server-Sent Events (SSE)](https://en.wikipedia.org/wiki/Server-sent_events) stream. It's incredibly 
easy to use. Here's what it looks like. You may also want to check out the [live example](/example.html).

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

## Subscribe as raw stream
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

## JSON message format
Both the [`/json` endpoint](#subscribe-as-json-stream) and the [`/sse` endpoint](#subscribe-as-sse-stream) return a JSON
format of the message. It's very straight forward:

| Field | Required | Type | Example | Description |
|---|---|---|---|---|
| `id` | ✔️ | *string* | `hwQ2YpKdmg` | Randomly chosen message identifier |
| `time` | ✔️ | *int* | `1635528741` | Message date time, as Unix time stamp |  
| `event` | ✔️ | `open`, `keepalive` or `message` | `message` | Message type, typically you'd be only interested in `message` |
| `topic` | ✔️ | *string* | `topic1,topic2` | Comma-separated list of topics the message is associated with; only one for all `message` events, but may be a list in `open` events |
| `message` | - | *string* | `Some message` | Message body; always present in `message` events |
| `title` | - | *string* | `Some title` | Message [title](../publish.md#message-title); if not set defaults to `ntfy.sh/<topic>` |
| `tags` | - | *string array* | `["tag1","tag2"]` | List of [tags](../publish.md#tags-emojis) that may or not map to emojis |
| `priority` | - | *1, 2, 3, 4, or 5* | `4` | Message [priority](../publish.md#message-priority) with 1=min, 3=default and 5=max |

Here's an example for each message type:

=== "Notification message"
    ``` json
    {
      "id": "wze9zgqK41",
      "time": 1638542110,
      "event": "message",
      "topic": "phil_alerts",
      "priority": 5,
      "tags": [
        "warning",
        "skull"
      ],
      "title": "Unauthorized access detected",
      "message": "Remote access to phils-laptop detected. Act right away."
    }
    ```


=== "Notification message (minimal)"
    ``` json
    {
      "id": "wze9zgqK41",
      "time": 1638542110,
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

## Advanced features

### Fetching cached messages
Messages may be cached for a couple of hours (see [message caching](../config.md#message-cache)) to account for network
interruptions of subscribers. If the server has configured message caching, you can read back what you missed by using 
the `since=` query parameter. It takes either a duration (e.g. `10m` or `30s`), a Unix timestamp (e.g. `1635528757`) 
or `all` (all cached messages).

```
curl -s "ntfy.sh/mytopic/json?since=10m"
```

### Polling for messages
You can also just poll for messages if you don't like the long-standing connection using the `poll=1`
query parameter. The connection will end after all available messages have been read. This parameter can be
combined with `since=` (defaults to `since=all`).

```
curl -s "ntfy.sh/mytopic/json?poll=1"
```

### Fetching scheduled messages
Messages that are [scheduled to be delivered](../publish.md#scheduled-delivery) at a later date are not typically 
returned when subscribing via the API, which makes sense, because after all, the messages have technically not been 
delivered yet. To also return scheduled messages from the API, you can use the `scheduled=1` (alias: `sched=1`) 
parameter (makes most sense with the `poll=1` parameter):

```
curl -s "ntfy.sh/mytopic/json?poll=1&sched=1"
```

### Subscribing to multiple topics
It's possible to subscribe to multiple topics in one HTTP call by providing a
comma-separated list of topics in the URL. This allows you to reduce the number of connections you have to maintain:

```
$ curl -s ntfy.sh/mytopic1,mytopic2/json
{"id":"0OkXIryH3H","time":1637182619,"event":"open","topic":"mytopic1,mytopic2,mytopic3"}
{"id":"dzJJm7BCWs","time":1637182634,"event":"message","topic":"mytopic1","message":"for topic 1"}
{"id":"Cm02DsxUHb","time":1637182643,"event":"message","topic":"mytopic2","message":"for topic 2"}
```
