# Publishing

Publishing messages can be done via PUT or POST. Topics are created on the fly by subscribing or publishing to them.
Because there is no sign-up, <b>the topic is essentially a password</b>, so pick something that's not easily guessable.

Here's an example showing how to publish a simple message using a POST request:
=== "Command line (curl)"
    ```
    curl -d "Backup successful 😀" ntfy.sh/mytopic
    ```

=== "HTTP"
    ``` http
    POST /mytopic HTTP/1.1
    Host: ntfy.sh

    Backup successful 😀
    ```
=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mytopic', {
      method: 'POST', // PUT works too
      body: 'Backup successful 😀'
    })
    ```

=== "Go"
    ``` go
    http.Post("https://ntfy.sh/mytopic", "text/plain",
        strings.NewReader("Backup successful 😀"))
    ```

=== "PHP"
    ``` php
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' => 'Content-Type: text/plain',
            'content' => 'Backup successful 😀'
        ]
    ]));
    ```

If you have the [Android app](../subscribe/phone.md) installed on your phone, this will create a notification that looks like this:

<figure markdown>
  ![basic notification](../static/img/basic-notification.png){ width=500 }
  <figcaption>Android notification</figcaption>
</figure>

There are more features related to publishing messages: You can set a [notification priority](#message-priority), 
a [title](#message-title), and [tag messages](#tags-emojis) 🥳 🎉. Here's an example that uses all of them at once:

=== "Command line (curl)"
    ```
    curl \
      -H "Title: Unauthorized access detected" \
      -H "Priority: urgent" \
      -H "Tags: warning,skull" \
      -d "Remote access to phils-laptop detected. Act right away." \
      ntfy.sh/phil_alerts
    ```

=== "HTTP"
    ``` http
    POST /phil_alerts HTTP/1.1
    Host: ntfy.sh
    Title: Unauthorized access detected
    Priority: urgent
    Tags: warning,skull
    
    Remote access to phils-laptop detected. Act right away.
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/phil_alerts', {
        method: 'POST', // PUT works too
        body: 'Remote access to phils-laptop detected. Act right away.',
        headers: {
            'Title': 'Unauthorized access detected',
            'Priority': 'urgent',
            'Tags': 'warning,skull'
        }
    })
    ```

=== "Go"
    ``` go
	req, _ := http.NewRequest("POST", "https://ntfy.sh/phil_alerts",
		strings.NewReader("Remote access to phils-laptop detected. Act right away."))
	req.Header.Set("Title", "Unauthorized access detected")
	req.Header.Set("Priority", "urgent")
	req.Header.Set("Tags", "warning,skull")
	http.DefaultClient.Do(req)
    ```

=== "PHP"
    ``` php
    file_get_contents('https://ntfy.sh/phil_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Title: Unauthorized access detected\r\n" .
                "Priority: urgent\r\n" .
                "Tags: warning,skull",
            'content' => 'Remote access to phils-laptop detected. Act right away.'
        ]
    ]));
    ```

<figure markdown>
  ![priority notification](../static/img/priority-notification.png){ width=500 }
  <figcaption>Urgent notification with tags and title</figcaption>
</figure>

## Message priority
All messages have a priority, which defines how urgently your phone notifies you. You can set custom
notification sounds and vibration patterns on your phone to map to these priorities (see [Android config](../subscribe/phone.md)).

The following priorities exist:

| Priority | Icon | ID | Name | Description |
|---|---|---|---|---|
| Max priority | ![min priority](../static/img/priority-5.svg) | `5` | `max`/`urgent` | Really long vibration bursts, default notification sound with a pop-over notification. |
| High priority | ![min priority](../static/img/priority-4.svg) | `4` | `high` | Long vibration burst, default notification sound with a pop-over notification. |
| **Default priority** | *(none)* | `3` | `default` | Short default vibration and sound. Default notification behavior. |
| Low priority | ![min priority](../static/img/priority-2.svg) |`2` | `low` | No vibration or sound. Notification will not visibly show up until notification drawer is pulled down. |
| Min priority | ![min priority](../static/img/priority-1.svg) | `1` | `min` | No vibration or sound. The notification will be under the fold in "Other notifications". |

You can set the priority with the header `X-Priority` (or any of its aliases: `Priority`, `prio`, or `p`).

=== "Command line (curl)"
    ```
    curl -H "X-Priority: 5" -d "An urgent message" ntfy.sh/phil_alerts
    curl -H "Priority: low" -d "Low priority message" ntfy.sh/phil_alerts
    curl -H p:4 -d "A high priority message" ntfy.sh/phil_alerts
    ```

=== "HTTP"
    ``` http
    POST /phil_alerts HTTP/1.1
    Host: ntfy.sh
    Priority: 5

    An urgent message
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/phil_alerts', {
        method: 'POST',
        body: 'An urgent message',
        headers: { 'Priority': '5' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/phil_alerts", strings.NewReader("An urgent message"))
    req.Header.Set("Priority", "5")
    http.DefaultClient.Do(req)
    ```

=== "PHP"
    ``` php
    file_get_contents('https://ntfy.sh/phil_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Priority: 5",
            'content' => 'An urgent message'
        ]
    ]));
    ```

<figure markdown>
  ![priority notification](../static/img/priority-detail-overview.png){ width=500 }
  <figcaption>Detail view of priority notifications</figcaption>
</figure>

## Tags & emojis 🥳 🎉
You can tag messages with emojis (or other relevant strings). If a tag matches a <a href="https://raw.githubusercontent.com/github/gemoji/master/db/emoji.json">known emoji short code</a>,
it will be converted to an emoji. If it doesn't match, it will be listed below the notification. This is useful
for things like warnings and such (⚠️, ️🚨, or 🚩), but also to simply tag messages otherwise (e.g. which script the
message came from, ...).

You can set tags with the `X-Tags` header (or any of its aliases: `Tags`, or `ta`).
Use <a href="https://raw.githubusercontent.com/github/gemoji/master/db/emoji.json">this reference</a>
to figure out what tags can be converted to emojis. In the example below, the tag "warning" matches the emoji ⚠️,
the tag "ssh-login" doesn't match and will be displayed below the message.

```
$ curl -H "Tags: warning,ssh-login" -d "Unauthorized SSH access" ntfy.sh/mytopic
{"id":"ZEIwjfHlSS",...,"tags":["warning","ssh-login"],"message":"Unauthorized SSH access"}
```

## Message title
The notification title is typically set to the topic short URL (e.g. `ntfy.sh/mytopic`.
To override it, you can set the `X-Title` header (or any of its aliases: `Title`, `ti`, or `t`).

```
curl -H "Title: Dogs are better than cats" -d "Oh my ..." ntfy.sh/mytopic<
```
        
