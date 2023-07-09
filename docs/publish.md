# Publishing
Publishing messages can be done via HTTP PUT/POST or via the [ntfy CLI](install.md). Topics are created on the fly by 
subscribing or publishing to them. Because there is no sign-up, **the topic is essentially a password**, so pick 
something that's not easily guessable.

Here's an example showing how to publish a simple message using a POST request:

=== "Command line (curl)"
    ```
    curl -d "Backup successful 😀" ntfy.sh/mytopic
    ```

=== "ntfy CLI"
    ```
    ntfy publish mytopic "Backup successful 😀"
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

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mytopic"
      Body = "Backup successful"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/mytopic", 
        data="Backup successful 😀".encode(encoding='utf-8'))
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' => 'Content-Type: text/plain',
            'content' => 'Backup successful 😀'
        ]
    ]));
    ```

If you have the [Android app](subscribe/phone.md) installed on your phone, this will create a notification that looks like this:

<figure markdown>
  ![basic notification](static/img/android-screenshot-basic-notification.png){ width=500 }
  <figcaption>Android notification</figcaption>
</figure>

There are more features related to publishing messages: You can set a [notification priority](#message-priority), 
a [title](#message-title), and [tag messages](#tags-emojis) 🥳 🎉. Here's an example that uses some of them at together:

=== "Command line (curl)"
    ```
    curl \
      -H "Title: Unauthorized access detected" \
      -H "Priority: urgent" \
      -H "Tags: warning,skull" \
      -d "Remote access to phils-laptop detected. Act right away." \
      ntfy.sh/phil_alerts
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --title "Unauthorized access detected" \
        --tags warning,skull \
        --priority urgent \
        mytopic \
        "Remote access to phils-laptop detected. Act right away."
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

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/phil_alerts"
      Headers = @{
        Title = "Unauthorized access detected"
        Priority = "urgent"
        Tags = "warning,skull"
      }
      Body = "Remote access to phils-laptop detected. Act right away."
    }
    Invoke-RestMethod @Request
    ```
    
=== "Python"
    ``` python
    requests.post("https://ntfy.sh/phil_alerts",
        data="Remote access to phils-laptop detected. Act right away.",
        headers={
            "Title": "Unauthorized access detected",
            "Priority": "urgent",
            "Tags": "warning,skull"
        })
    ```

=== "PHP"
    ``` php-inline
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
  ![priority notification](static/img/priority-notification.png){ width=500 }
  <figcaption>Urgent notification with tags and title</figcaption>
</figure>

You can also do multi-line messages. Here's an example using a [click action](#click-action), an [action button](#action-buttons),
an [external image attachment](#attach-file-from-a-url) and [email publishing](#e-mail-publishing):

=== "Command line (curl)"
    ```
    curl \
      -H "Click: https://home.nest.com/" \
      -H "Attach: https://nest.com/view/yAxkasd.jpg" \
      -H "Actions: http, Open door, https://api.nest.com/open/yAxkasd, clear=true" \
      -H "Email: phil@example.com" \
      -d "There's someone at the door. 🐶
   
    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell." \
      ntfy.sh/mydoorbell
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
	    --click="https://home.nest.com/" \
        --attach="https://nest.com/view/yAxkasd.jpg" \
        --actions="http, Open door, https://api.nest.com/open/yAxkasd, clear=true" \
        --email="phil@example.com" \
        mydoorbell \
        "There's someone at the door. 🐶
   
    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell."
    ```

=== "HTTP"
    ``` http
    POST /mydoorbell HTTP/1.1
    Host: ntfy.sh
    Click: https://home.nest.com/
    Attach: https://nest.com/view/yAxkasd.jpg
    Actions: http, Open door, https://api.nest.com/open/yAxkasd, clear=true
    Email: phil@example.com
    
    There's someone at the door. 🐶
   
    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.
    ```
    
=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mydoorbell', {
        method: 'POST', // PUT works too
        headers: {
            'Click': 'https://home.nest.com/',
            'Attach': 'https://nest.com/view/yAxkasd.jpg',
	        'Actions': 'http, Open door, https://api.nest.com/open/yAxkasd, clear=true',
	        'Email': 'phil@example.com'
        },
        body: `There's someone at the door. 🐶
       
    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.`,
    })
    ```

=== "Go"
    ``` go
	req, _ := http.NewRequest("POST", "https://ntfy.sh/mydoorbell",
		strings.NewReader(`There's someone at the door. 🐶
   
    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.`))
	req.Header.Set("Click", "https://home.nest.com/")
	req.Header.Set("Attach", "https://nest.com/view/yAxkasd.jpg")
	req.Header.Set("Actions", "http, Open door, https://api.nest.com/open/yAxkasd, clear=true")
	req.Header.Set("Email", "phil@example.com")
	http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mydoorbell"
      Headers = @{
        Click = "https://home.nest.com"
        Attach = "https://nest.com/view/yAxksd.jpg"
        Actions = "http, Open door, https://api.nest.com/open/yAxkasd, clear=true"
        Email = "phil@example.com"
      }
      Body = "There's someone at the door. 🐶`n
      `n
      Please check if it's a good boy or a hooman.`n
      Doggies have been known to ring the doorbell.`n"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/mydoorbell",
        data="""There's someone at the door. 🐶

    Please check if it's a good boy or a hooman.
    Doggies have been known to ring the doorbell.""".encode('utf-8'),
        headers={
            "Click": "https://home.nest.com/",
            "Attach": "https://nest.com/view/yAxkasd.jpg",
            "Actions": "http, Open door, https://api.nest.com/open/yAxkasd, clear=true",
            "Email": "phil@example.com"
        })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mydoorbell', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Click: https://home.nest.com/\r\n" .
                "Attach: https://nest.com/view/yAxkasd.jpg\r\n" .
                "Actions": "http, Open door, https://api.nest.com/open/yAxkasd, clear=true\r\n" .
                "Email": "phil@example.com\r\n",
            'content' => 'There\'s someone at the door. 🐶
   
    Please check if it\'s a good boy or a hooman.
    Doggies have been known to ring the doorbell.'
        ]
    ]));
    ```

<figure markdown>
  ![priority notification](static/img/android-screenshot-notification-multiline.jpg){ width=500 }
  <figcaption>Notification using a click action, a user action, with an external image attachment and forwarded via email</figcaption>
</figure>

## Message title
_Supported on:_ :material-android: :material-apple: :material-firefox:

The notification title is typically set to the topic short URL (e.g. `ntfy.sh/mytopic`). To override the title, 
you can set the `X-Title` header (or any of its aliases: `Title`, `ti`, or `t`).

=== "Command line (curl)"
    ```
    curl -H "X-Title: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    curl -H "Title: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    curl -H "t: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        -t "Dogs are better than cats" \
        controversial "Oh my ..."
    ```

=== "HTTP"
    ``` http
    POST /controversial HTTP/1.1
    Host: ntfy.sh
    Title: Dogs are better than cats
    
    Oh my ...
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/controversial', {
        method: 'POST',
        body: 'Oh my ...',
        headers: { 'Title': 'Dogs are better than cats' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/controversial", strings.NewReader("Oh my ..."))
    req.Header.Set("Title", "Dogs are better than cats")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/controversial"
      Headers = @{
        Title = "Dogs are better than cats"
      }
      Body = "Oh my ..."
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/controversial",
        data="Oh my ...",
        headers={ "Title": "Dogs are better than cats" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/controversial', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Title: Dogs are better than cats",
            'content' => 'Oh my ...'
        ]
    ]));
    ```

<figure markdown>
  ![notification with title](static/img/notification-with-title.png){ width=500 }
  <figcaption>Detail view of notification with title</figcaption>
</figure>

!!! info
    ntfy supports UTF-8 in HTTP headers, but [not every library or programming language does](https://www.jmix.io/blog/utf-8-in-http-headers/).
    If non-ASCII characters are causing issues for you in the title (i.e. you're seeing `?` symbols), you may also encode any header (including the title)
    as [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047#section-2), e.g. `=?UTF-8?B?8J+HqfCfh6o=?=` ([base64](https://en.wikipedia.org/wiki/Base64)),
    or `=?UTF-8?Q?=C3=84pfel?=` ([quoted-printable](https://en.wikipedia.org/wiki/Quoted-printable)).

## Message priority
_Supported on:_ :material-android: :material-apple: :material-firefox:

All messages have a priority, which defines how urgently your phone notifies you. On Android, you can set custom
notification sounds and vibration patterns on your phone to map to these priorities (see [Android config](subscribe/phone.md)).

The following priorities exist:

| Priority             | Icon                                       | ID  | Name           | Description                                                                                            |
|----------------------|--------------------------------------------|-----|----------------|--------------------------------------------------------------------------------------------------------|
| Max priority         | ![min priority](static/img/priority-5.svg) | `5` | `max`/`urgent` | Really long vibration bursts, default notification sound with a pop-over notification.                 |
| High priority        | ![min priority](static/img/priority-4.svg) | `4` | `high`         | Long vibration burst, default notification sound with a pop-over notification.                         |
| **Default priority** | *(none)*                                   | `3` | `default`      | Short default vibration and sound. Default notification behavior.                                      |
| Low priority         | ![min priority](static/img/priority-2.svg) | `2` | `low`          | No vibration or sound. Notification will not visibly show up until notification drawer is pulled down. |
| Min priority         | ![min priority](static/img/priority-1.svg) | `1` | `min`          | No vibration or sound. The notification will be under the fold in "Other notifications".               |

You can set the priority with the header `X-Priority` (or any of its aliases: `Priority`, `prio`, or `p`).

=== "Command line (curl)"
    ```
    curl -H "X-Priority: 5" -d "An urgent message" ntfy.sh/phil_alerts
    curl -H "Priority: low" -d "Low priority message" ntfy.sh/phil_alerts
    curl -H p:4 -d "A high priority message" ntfy.sh/phil_alerts
    ```

=== "ntfy CLI"
    ```
    ntfy publish \ 
        -p 5 \
        phil_alerts An urgent message
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

=== "PowerShell"
    ``` powershell
    $Request = @{
      URI = "https://ntfy.sh/phil_alerts"
      Headers = @{
        Priority = "5"
      }
      Body = "An urgent message"
    }
    Invoke-RestMethod @Request
    ```
    
=== "Python"
    ``` python
    requests.post("https://ntfy.sh/phil_alerts",
        data="An urgent message",
        headers={ "Priority": "5" })
    ```

=== "PHP"
    ``` php-inline
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
  ![priority notification](static/img/priority-detail-overview.png){ width=500 }
  <figcaption>Detail view of priority notifications</figcaption>
</figure>

## Tags & emojis 🥳 🎉
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can tag messages with emojis and other relevant strings:

* **Emojis**: If a tag matches an [emoji short code](emojis.md), it'll be converted to an emoji and prepended 
  to title or message.
* **Other tags:** If a tag doesn't match, it will be listed below the notification. 

This feature is useful for things like warnings (⚠️, ️🚨, or 🚩), but also to simply tag messages otherwise (e.g. script 
names, hostnames, etc.). Use [the emoji short code list](emojis.md) to figure out what tags can be converted to emojis. 
Here's an **excerpt of emojis** I've found very useful in alert messages:

<table class="remove-md-box"><tr>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody>
    <tr><td><code>+1</code></td><td>👍</td></tr>
    <tr><td><code>partying_face</code></td><td>🥳</td></tr>
    <tr><td><code>tada</code></td><td>🎉</td></tr>
    <tr><td><code>heavy_check_mark</code></td><td>✔️</td></tr>
    <tr><td><code>loudspeaker</code></td><td>📢</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody> 
    <tr><td><code>-1</code></td><td>👎️</td></tr>
    <tr><td><code>warning</code></td><td>⚠️</td></tr>
    <tr><td><code>rotating_light</code></td><td>️🚨</td></tr>
    <tr><td><code>triangular_flag_on_post</code></td><td>🚩</td></tr>
    <tr><td><code>skull</code></td><td>💀</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody>
    <tr><td><code>facepalm</code></td><td>🤦</td></tr>
    <tr><td><code>no_entry</code></td><td>⛔</td></tr>
    <tr><td><code>no_entry_sign</code></td><td>🚫</td></tr>
    <tr><td><code>cd</code></td><td>💿</td></tr> 
    <tr><td><code>computer</code></td><td>💻</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
</tr></table>

You can set tags with the `X-Tags` header (or any of its aliases: `Tags`, `tag`, or `ta`). Specify multiple tags by separating
them with a comma, e.g. `tag1,tag2,tag3`.

=== "Command line (curl)"
    ```
    curl -H "X-Tags: warning,mailsrv13,daily-backup" -d "Backup of mailsrv13 failed" ntfy.sh/backups
    curl -H "Tags: horse,unicorn" -d "Unicorns are just horses with unique horns" ntfy.sh/backups
    curl -H ta:dog -d "Dogs are awesome" ntfy.sh/backups
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --tags=warning,mailsrv13,daily-backup \
        backups "Backup of mailsrv13 failed"
    ```

=== "HTTP"
    ``` http
    POST /backups HTTP/1.1
    Host: ntfy.sh
    Tags: warning,mailsrv13,daily-backup
    
    Backup of mailsrv13 failed
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/backups', {
        method: 'POST',
        body: 'Backup of mailsrv13 failed',
        headers: { 'Tags': 'warning,mailsrv13,daily-backup' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/backups", strings.NewReader("Backup of mailsrv13 failed"))
    req.Header.Set("Tags", "warning,mailsrv13,daily-backup")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/backups"
      Headers = @{
        Tags = "warning,mailsrv13,daily-backup"
      }
      Body = "Backup of mailsrv13 failed"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/backups",
        data="Backup of mailsrv13 failed",
        headers={ "Tags": "warning,mailsrv13,daily-backup" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/backups', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Tags: warning,mailsrv13,daily-backup",
            'content' => 'Backup of mailsrv13 failed'
        ]
    ]));
    ```

<figure markdown>
  ![priority notification](static/img/notification-with-tags.png){ width=500 }
  <figcaption>Detail view of notifications with tags</figcaption>
</figure>

!!! info
    ntfy supports UTF-8 in HTTP headers, but [not every library or programming language does](https://www.jmix.io/blog/utf-8-in-http-headers/).
    If non-ASCII characters are causing issues for you in the title (i.e. you're seeing `?` symbols), you may also encode the tags header or individual tags
    as [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047#section-2), e.g. `tag1,=?UTF-8?B?8J+HqfCfh6o=?=` ([base64](https://en.wikipedia.org/wiki/Base64)),
    or `=?UTF-8?Q?=C3=84pfel?=,tag2` ([quoted-printable](https://en.wikipedia.org/wiki/Quoted-printable)).

## Markdown formatting
_Supported on:_ :material-firefox:

You can format messages using [Markdown](https://www.markdownguide.org/basic-syntax/) 🤩. That means you can use 
**bold text**, *italicized text*, links, images, and more. Supported Markdown features (web app only for now):

- [Emphasis](https://www.markdownguide.org/basic-syntax/#emphasis) such as **bold** (`**bold**`), *italics* (`*italics*`)
- [Links](https://www.markdownguide.org/basic-syntax/#links) (`[some tool](https://ntfy.sh)`)
- [Images](https://www.markdownguide.org/basic-syntax/#images) (`![some image](https://bing.com/logo.png)`)
- [Code blocks](https://www.markdownguide.org/basic-syntax/#code-blocks) (` ```code blocks``` `) and [inline code](https://www.markdownguide.org/basic-syntax/#inline-code) (`` `inline code` ``)
- [Headings](https://www.markdownguide.org/basic-syntax/#headings) (`# headings`, `## headings`, etc.)
- [Lists](https://www.markdownguide.org/basic-syntax/#lists) (`- lists`, `1. lists`, etc.)
- [Blockquotes](https://www.markdownguide.org/basic-syntax/#blockquotes) (`> blockquotes`)
- [Horizontal rules](https://www.markdownguide.org/basic-syntax/#horizontal-rules) (`---`)

By default, messages sent to ntfy are rendered as plain text. To enable Markdown, set the `X-Markdown` header (or any of
its aliases: `Markdown`, or `md`) to `true` (or `1` or `yes`), or set the `Content-Type` header to `text/markdown`.
As of today, **Markdown is only supported in the web app.** Here's an example of how to enable Markdown formatting:

=== "Command line (curl)"
    ```
    curl \
        -d "Look ma, **bold text**, *italics*, ..." \
        -H "Markdown: yes" \
        ntfy.sh/mytopic
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        mytopic \
        --markdown \
        "Look ma, **bold text**, *italics*, ..."
    ```

=== "HTTP"
    ``` http
    POST /mytopic HTTP/1.1
    Host: ntfy.sh
    Markdown: yes

    Look ma, **bold text**, *italics*, ...
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mytopic', {
      method: 'POST', // PUT works too
      body: 'Look ma, **bold text**, *italics*, ...',
      headers: { 'Markdown': 'yes' }
    })
    ```

=== "Go"
    ``` go
    http.Post("https://ntfy.sh/mytopic", "text/markdown",
        strings.NewReader("Look ma, **bold text**, *italics*, ..."))

    // or
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mytopic", 
        strings.NewReader("Look ma, **bold text**, *italics*, ..."))
    req.Header.Set("Markdown", "yes")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mytopic"
      Body = "Look ma, **bold text**, *italics*, ..."
      Headers = @{
        Markdown = "yes"
      }
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/mytopic", 
        data="Look ma, **bold text**, *italics*, ..."
        headers={ "Markdown": "yes" }))
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' => 'Content-Type: text/markdown', // !
            'content' => 'Look ma, **bold text**, *italics*, ...'
        ]
    ]));
    ```

Here's what that looks like in the web app:

<figure markdown>
  ![markdown](static/img/web-markdown.png){ width=500 }
  <figcaption>Markdown formatting in the web app</figcaption>
</figure>

## Scheduled delivery
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can delay the delivery of messages and let ntfy send them at a later date. This can be used to send yourself 
reminders or even to execute commands at a later date (if your subscriber acts on messages).

Usage is pretty straight forward. You can set the delivery time using the `X-Delay` header (or any of its aliases: `Delay`, 
`X-At`, `At`, `X-In` or `In`), either by specifying a Unix timestamp (e.g. `1639194738`), a duration (e.g. `30m`, 
`3h`, `2 days`), or a natural language time string (e.g. `10am`, `8:30pm`, `tomorrow, 3pm`, `Tuesday, 7am`, 
[and more](https://github.com/olebedev/when)). 

As of today, the minimum delay you can set is **10 seconds** and the maximum delay is **3 days**. This can currently
not be configured otherwise ([let me know](https://github.com/binwiederhier/ntfy/issues) if you'd like to change 
these limits).

For the purposes of [message caching](config.md#message-cache), scheduled messages are kept in the cache until 12 hours 
after they were delivered (or whatever the server-side cache duration is set to). For instance, if a message is scheduled
to be delivered in 3 days, it'll remain in the cache for 3 days and 12 hours. Also note that naturally, 
[turning off server-side caching](#message-caching) is not possible in combination with this feature.  

=== "Command line (curl)"
    ```
    curl -H "At: tomorrow, 10am" -d "Good morning" ntfy.sh/hello
    curl -H "In: 30min" -d "It's 30 minutes later now" ntfy.sh/reminder
    curl -H "Delay: 1639194738" -d "Unix timestamps are awesome" ntfy.sh/itsaunixsystem
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --at="tomorrow, 10am" \
        hello "Good morning"
    ```

=== "HTTP"
    ``` http
    POST /hello HTTP/1.1
    Host: ntfy.sh
    At: tomorrow, 10am

    Good morning
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/hello', {
        method: 'POST',
        body: 'Good morning',
        headers: { 'At': 'tomorrow, 10am' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/hello", strings.NewReader("Good morning"))
    req.Header.Set("At", "tomorrow, 10am")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/hello"
      Headers = @{
        At = "tomorrow, 10am"
      }
      Body = "Good morning"
    }
    Invoke-RestMethod @Request
    ```
    
=== "Python"
    ``` python
    requests.post("https://ntfy.sh/hello",
        data="Good morning",
        headers={ "At": "tomorrow, 10am" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/backups', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "At: tomorrow, 10am",
            'content' => 'Good morning'
        ]
    ]));
    ```

Here are a few examples (assuming today's date is **12/10/2021, 9am, Eastern Time Zone**):

<table class="remove-md-box"><tr>
<td>
    <table><thead><tr><th><code>Delay/At/In</code> header</th><th>Message will be delivered at</th><th>Explanation</th></tr></thead><tbody>
    <tr><td><code>30m</code></td><td>12/10/2021, 9:<b>30</b>am</td><td>30 minutes from now</td></tr>
    <tr><td><code>2 hours</code></td><td>12/10/2021, <b>11:30</b>am</td><td>2 hours from now</td></tr>
    <tr><td><code>1 day</code></td><td>12/<b>11</b>/2021, 9am</td><td>24 hours from now</td></tr>
    <tr><td><code>10am</code></td><td>12/10/2021, <b>10am</b></td><td>Today at 10am (same day, because it's only 9am)</td></tr>
    <tr><td><code>8am</code></td><td>12/<b>11</b>/2021, <b>8am</b></td><td>Tomorrow at 8am (because it's 9am already)</td></tr>
    <tr><td><code>1639152000</code></td><td>12/10/2021, 11am (EST)</td><td> Today at 11am (EST)</td></tr>
    </tbody></table>
</td>
</tr></table>

## Webhooks (publish via GET) 
_Supported on:_ :material-android: :material-apple: :material-firefox:

In addition to using PUT/POST, you can also send to topics via simple HTTP GET requests. This makes it easy to use 
a ntfy topic as a [webhook](https://en.wikipedia.org/wiki/Webhook), or if your client has limited HTTP support (e.g.
like the [MacroDroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid) Android app).

To send messages via HTTP GET, simply call the `/publish` endpoint (or its aliases `/send` and `/trigger`). Without 
any arguments, this will send the message `triggered` to the topic. However, you can provide all arguments that are 
also supported as HTTP headers as URL-encoded arguments. Be sure to check the list of all 
[supported parameters and headers](#list-of-all-parameters) for details.

For instance, assuming your topic is `mywebhook`, you can simply call `/mywebhook/trigger` to send a message 
(aka trigger the webhook):

=== "Command line (curl)"
    ```
    curl ntfy.sh/mywebhook/trigger
    ```

=== "ntfy CLI"
    ```
    ntfy trigger mywebhook
    ```

=== "HTTP"
    ``` http
    GET /mywebhook/trigger HTTP/1.1
    Host: ntfy.sh
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mywebhook/trigger')
    ```

=== "Go"
    ``` go
    http.Get("https://ntfy.sh/mywebhook/trigger")
    ```

=== "PowerShell"
    ``` powershell
    Invoke-RestMethod "ntfy.sh/mywebhook/trigger"
    ```    

=== "Python"
    ``` python
    requests.get("https://ntfy.sh/mywebhook/trigger")
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mywebhook/trigger');
    ```

To add a custom message, simply append the `message=` URL parameter. And of course you can set the 
[message priority](#message-priority), the [message title](#message-title), and [tags](#tags-emojis) as well. 
For a full list of possible parameters, check the list of [supported parameters and headers](#list-of-all-parameters).

Here's an example with a custom message, tags and a priority:

=== "Command line (curl)"
    ```
    curl "ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull"
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        -p 5 --tags=warning,skull \
        mywebhook "Webhook triggered"
    ```

=== "HTTP"
    ``` http
    GET /mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull HTTP/1.1
    Host: ntfy.sh
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull')
    ```

=== "Go"
    ``` go
    http.Get("https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull")
    ```

=== "PowerShell"
    ``` powershell
    Invoke-RestMethod "ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull"
    ```

=== "Python"
    ``` python
    requests.get("https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull")
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull');
    ```

## Publish as JSON
_Supported on:_ :material-android: :material-apple: :material-firefox:

For some integrations with other tools (e.g. [Jellyfin](https://jellyfin.org/), [overseerr](https://overseerr.dev/)), 
adding custom headers to HTTP requests may be tricky or impossible, so ntfy also allows publishing the entire message 
as JSON in the request body.

To publish as JSON, simple PUT/POST the JSON object directly to the ntfy root URL. The message format is described below
the example.

!!! info
    To publish as JSON, you must **PUT/POST to the ntfy root URL**, not to the topic URL. Be sure to check that you're
    POST-ing to `https://ntfy.sh/` (correct), and not to `https://ntfy.sh/mytopic` (incorrect). 

Here's an example using most supported parameters. Check the table below for a complete list. The `topic` parameter 
is the only required one:

=== "Command line (curl)"
    ```
    curl ntfy.sh \
      -d '{
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
      }'
    ```

=== "HTTP"
    ``` http
    POST / HTTP/1.1
    Host: ntfy.sh

    {
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
    }
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            "topic": "mytopic",
            "message": "Disk space is low at 5.1 GB",
            "title": "Low disk space alert",
            "tags": ["warning","cd"],
            "priority": 4,
            "attach": "https://filesrv.lan/space.jpg",
            "filename": "diskspace.jpg",
            "click": "https://homecamera.lan/xasds1h2xsSsa/",
            "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
        })
    })
    ```

=== "Go"
    ``` go
    // You should probably use json.Marshal() instead and make a proper struct,
    // or even just use req.Header.Set() like in the other examples, but for the 
    // sake of the example, this is easier.
    
    body := `{
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh"
      Body = @{
        Topic    = "mytopic"
        Title    = "Low disk space alert"
        Message  = "Disk space is low at 5.1 GB"
        Priority = 4
        Attach   = "https://filesrv.lan/space.jpg"
        FileName = "diskspace.jpg"
        Tags     = @("warning", "cd")
        Click    = "https://homecamera.lan/xasds1h2xsSsa/"
        Actions  = ConvertTo-JSON @(
          @{ 
            Action = "view"
            Label  = "Admin panel"
            URL    = "https://filesrv.lan/admin"
          }
        )
      }
      ContentType = "application/json"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "mytopic",
            "message": "Disk space is low at 5.1 GB",
            "title": "Low disk space alert",
            "tags": ["warning","cd"],
            "priority": 4,
            "attach": "https://filesrv.lan/space.jpg",
            "filename": "diskspace.jpg",
            "click": "https://homecamera.lan/xasds1h2xsSsa/",
            "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
        })
    )
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "mytopic",
                "message": "Disk space is low at 5.1 GB",
                "title": "Low disk space alert",
                "tags": ["warning","cd"],
                "priority": 4,
                "attach": "https://filesrv.lan/space.jpg",
                "filename": "diskspace.jpg",
                "click": "https://homecamera.lan/xasds1h2xsSsa/",
                "actions": [["action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" ]]
            ])
        ]
    ]));
    ```

The JSON message format closely mirrors the format of the message you can consume when you [subscribe via the API](subscribe/api.md) 
(see [JSON message format](subscribe/api.md#json-message-format) for details), but is not exactly identical. Here's an overview of
all the supported fields:

| Field      | Required | Type                             | Example                                   | Description                                                           |
|------------|----------|----------------------------------|-------------------------------------------|-----------------------------------------------------------------------|
| `topic`    | ✔️       | *string*                         | `topic1`                                  | Target topic name                                                     |
| `message`  | -        | *string*                         | `Some message`                            | Message body; set to `triggered` if empty or not passed               |
| `title`    | -        | *string*                         | `Some title`                              | Message [title](#message-title)                                       |
| `tags`     | -        | *string array*                   | `["tag1","tag2"]`                         | List of [tags](#tags-emojis) that may or not map to emojis            |
| `priority` | -        | *int (one of: 1, 2, 3, 4, or 5)* | `4`                                       | Message [priority](#message-priority) with 1=min, 3=default and 5=max |
| `actions`  | -        | *JSON array*                     | *(see [action buttons](#action-buttons))* | Custom [user action buttons](#action-buttons) for notifications       |
| `click`    | -        | *URL*                            | `https://example.com`                     | Website opened when notification is [clicked](#click-action)          |
| `attach`   | -        | *URL*                            | `https://example.com/file.jpg`            | URL of an attachment, see [attach via URL](#attach-file-from-url)     |
| `markdown` | -        | *bool*                           | `true`                                    | Set to true if the `message` is Markdown-formatted                    |
| `icon`     | -        | *string*                         | `https://example.com/icon.png`            | URL to use as notification [icon](#icons)                             |
| `filename` | -        | *string*                         | `file.jpg`                                | File name of the attachment                                           |
| `delay`    | -        | *string*                         | `30min`, `9am`                            | Timestamp or duration for delayed delivery                            |
| `email`    | -        | *e-mail address*                 | `phil@example.com`                        | E-mail address for e-mail notifications                               |
| `call`     | -        | *phone number or 'yes'*          | `+1222334444` or `yes`                    | Phone number to use for [voice call](#phone-calls)                    |

## Action buttons
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can add action buttons to notifications to allow yourself to react to a notification directly. This is incredibly
useful and has countless applications. 

You can control your home appliances (open/close garage door, change temperature on thermostat, ...), react to common 
monitoring alerts (clear logs when disk is full, ...), and many other things. The sky is the limit.

As of today, the following actions are supported:

* [`view`](#open-websiteapp): Opens a website or app when the action button is tapped
* [`broadcast`](#send-android-broadcast): Sends an [Android broadcast](https://developer.android.com/guide/components/broadcasts) intent
  when the action button is tapped (only supported on Android)
* [`http`](#send-http-request): Sends HTTP POST/GET/PUT request when the action button is tapped

Here's an example of what that a notification with actions can look like:

<figure markdown>
  ![notification with actions](static/img/android-screenshot-notification-actions.png){ width=500 }
  <figcaption>Notification with two user actions</figcaption>
</figure>

### Defining actions
You can define **up to three user actions** in your notifications, using either of the following methods:

* In the [`X-Actions` header](#using-a-header), using a simple comma-separated format
* As a [JSON array](#using-a-json-array) in the `actions` key, when [publishing as JSON](#publish-as-json) 

#### Using a header
To define actions using the `X-Actions` header (or any of its aliases: `Actions`, `Action`), use the following format:

=== "Header format (long)"
    ```
    action=<action1>, label=<label1>, paramN=... [; action=<action2>, label=<label2>, ...]
    ```

=== "Header format (short)"
    ```
    <action1>, <label1>, paramN=... [; <action2>, <label2>, ...]
    ```

Multiple actions are separated by a semicolon (`;`), and key/value pairs are separated by commas (`,`). Values may be 
quoted with double quotes (`"`) or single quotes (`'`) if the value itself contains commas or semicolons. 

The `action=` and `label=` prefix are optional in all actions, and the `url=` prefix is optional in the `view` and 
`http` action. The only limitation of this format is that depending on your language/library, UTF-8 characters may not 
work. If they don't, use the [JSON array format](#using-a-json-array) instead.

As an example, here's how you can create the above notification using this format. Refer to the [`view` action](#open-websiteapp) and 
[`http` action](#send-http-request) section for details on the specific actions:

=== "Command line (curl)"
    ```
    body='{"temperature": 65}'
    curl \
        -d "You left the house. Turn down the A/C?" \
        -H "Actions: view, Open portal, https://home.nest.com/, clear=true; \
                     http, Turn down, https://api.nest.com/, body='$body'" \
        ntfy.sh/myhome
    ```

=== "ntfy CLI"
    ```
    body='{"temperature": 65}'
    ntfy publish \
        --actions="view, Open portal, https://home.nest.com/, clear=true; \
                   http, Turn down, https://api.nest.com/, body='$body'" \
        myhome \
        "You left the house. Turn down the A/C?"
    ```

=== "HTTP"
    ``` http
    POST /myhome HTTP/1.1
    Host: ntfy.sh
    Actions: view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{"temperature": 65}'

    You left the house. Turn down the A/C?
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'You left the house. Turn down the A/C?',
        headers: { 
            'Actions': 'view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body=\'{"temperature": 65}\'' 
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("You left the house. Turn down the A/C?"))
    req.Header.Set("Actions", "view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/myhome"
      Headers = @{
        Actions="view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'"
      }
      Body = "You left the house. Turn down the A/C?"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/myhome",
        data="You left the house. Turn down the A/C?",
        headers={ "Actions": "view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'",
            'content' => 'You left the house. Turn down the A/C?'
        ]
    ]));
    ```

!!! info
    ntfy supports UTF-8 in HTTP headers, but [not every library or programming language does](https://www.jmix.io/blog/utf-8-in-http-headers/).
    If non-ASCII characters are causing issues for you in the title (i.e. you're seeing `?` symbols), you may also encode any header (including actions) 
    as [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047#section-2), e.g. `=?UTF-8?B?8J+HqfCfh6o=?=` ([base64](https://en.wikipedia.org/wiki/Base64)),
    or `=?UTF-8?Q?=C3=84pfel?=` ([quoted-printable](https://en.wikipedia.org/wiki/Quoted-printable)).

#### Using a JSON array
Alternatively, the same actions can be defined as **JSON array**, if the notification is defined as part of the JSON body 
(see [publish as JSON](#publish-as-json)):

=== "Command line (curl)"
    ```
    curl ntfy.sh \
      -d '{
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
      }'
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions '[
            {
                "action": "view",
                "label": "Open portal",
                "url": "https://home.nest.com/",
                "clear": true
            },
            {
                "action": "http",
                "label": "Turn down",
                "url": "https://api.nest.com/",
                "body": "{\"temperature\": 65}"
            }
        ]' \
        myhome \
        "You left the house. Turn down the A/C?"
    ```

=== "HTTP"
    ``` http
    POST / HTTP/1.1
    Host: ntfy.sh

    {
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
    }
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message: "You left the house. Turn down the A/C?",
            actions: [
                {
                    action: "view",
                    label: "Open portal",
                    url: "https://home.nest.com/",
                    clear: true
                },
                {
                    action: "http",
                    label: "Turn down",
                    url: "https://api.nest.com/",
                    body: "{\"temperature\": 65}"
                }
            ]
        })
    })
    ```

=== "Go"
    ``` go
    // You should probably use json.Marshal() instead and make a proper struct,
    // but for the sake of the example, this is easier.
    
    body := `{
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh"
      Body = ConvertTo-JSON @{
        Topic   = "myhome"
        Message = "You left the house. Turn down the A/C?"
        Actions = @(
          @{
            Action = "view"
            Label  = "Open portal"
            URL    = "https://home.nest.com/"
            Clear  = $true
          },
          @{
            Action = "http"
            Label  = "Turn down"
            URL    = "https://api.nest.com/"
            Body   = '{"temperature": 65}'
          }
        )
      }
      ContentType = "application/json"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "You left the house. Turn down the A/C?",
            "actions": [
                {
                    "action": "view",
                    "label": "Open portal",
                    "url": "https://home.nest.com/",
                    "clear": true
                },
                {
                    "action": "http",
                    "label": "Turn down",
                    "url": "https://api.nest.com/",
                    "body": "{\"temperature\": 65}"
                }
            ]
        })
    )
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "You left the house. Turn down the A/C?",
                "actions": [
                    [
                        "action": "view",
                        "label": "Open portal",
                        "url": "https://home.nest.com/",
                        "clear": true
                    ],
                    [
                        "action": "http",
                        "label": "Turn down",
                        "url": "https://api.nest.com/",
                        "headers": [
                            "Authorization": "Bearer ..."
                        ],
                        "body": "{\"temperature\": 65}"
                    ]
                ]
            ])
        ]
    ]));
    ```

The required/optional fields for each action depend on the type of the action itself. Please refer to 
[`view` action](#open-websiteapp), [`broadcast` action](#send-android-broadcast), and [`http` action](#send-http-request) 
for details.

### Open website/app
_Supported on:_ :material-android: :material-apple: :material-firefox:

The `view` action **opens a website or app when the action button is tapped**, e.g. a browser, a Google Maps location, or
even a deep link into Twitter or a show ntfy topic. How exactly the action is handled depends on how Android and your 
desktop browser treat the links. Normally it'll just open a link in the browser. 

Examples:

* `http://` or `https://` will open your browser (or an app if it registered for a URL)
* `mailto:` links will open your mail app, e.g. `mailto:phil@example.com`
* `geo:` links will open Google Maps, e.g. `geo:0,0?q=1600+Amphitheatre+Parkway,+Mountain+View,+CA`
* `ntfy://` links will open ntfy (see [ntfy:// links](subscribe/phone.md#ntfy-links)), e.g. `ntfy://ntfy.sh/stats`
* `twitter://` links will open Twitter, e.g. `twitter://user?screen_name=..`
* ...

Here's an example using the [`X-Actions` header](#using-a-header):

=== "Command line (curl)"
    ```
    curl \
        -d "Somebody retweeted your tweet." \
        -H "Actions: view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" \
    ntfy.sh/myhome
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions="view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" \
        myhome \
        "Somebody retweeted your tweet."
    ```

=== "HTTP"
    ``` http
    POST /myhome HTTP/1.1
    Host: ntfy.sh
    Actions: view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392

    Somebody retweeted your tweet.
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'Somebody retweeted your tweet.',
        headers: { 
            'Actions': 'view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392' 
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("Somebody retweeted your tweet."))
    req.Header.Set("Actions", "view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/myhome"
      Headers = @{
        Actions = "view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392"
      }
      Body = "Somebody retweeted your tweet."
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/myhome",
        data="Somebody retweeted your tweet.",
        headers={ "Actions": "view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392",
            'content' => 'Somebody retweeted your tweet.'
        ]
    ]));
    ```

And the same example using [JSON publishing](#publish-as-json):

=== "Command line (curl)"
    ```
    curl ntfy.sh \
      -d '{
        "topic": "myhome",
        "message": "Somebody retweeted your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
      }'
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions '[
            {
                "action": "view",
                "label": "Open Twitter",
                "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
            }
        ]' \
        myhome \
        "Somebody retweeted your tweet."
    ```

=== "HTTP"
    ``` http
    POST / HTTP/1.1
    Host: ntfy.sh

    {
        "topic": "myhome",
        "message": "Somebody retweeted your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
    }
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message": "Somebody retweeted your tweet.",
            actions: [
                {
                    action: "view",
                    label: "Open Twitter",
                    url: "https://twitter.com/binwiederhier/status/1467633927951163392"
                }
            ]
        })
    })
    ```

=== "Go"
    ``` go
    // You should probably use json.Marshal() instead and make a proper struct,
    // but for the sake of the example, this is easier.
    
    body := `{
        "topic": "myhome",
        "message": "Somebody retweeted your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh"
      Body = ConvertTo-JSON @{
        Topic = "myhome"
        Message = "Somebody retweeted your tweet."
        Actions = @(
          @{
            Action = "view"
            Label  = "Open Twitter"
            URL    = "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        )
      }
      ContentType = "application/json"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "Somebody retweeted your tweet.",
            "actions": [
                {
                    "action": "view",
                    "label": "Open Twitter",
                    "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
                }
            ]
        })
    )
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "Somebody retweeted your tweet.",
                "actions": [
                    [
                        "action": "view",
                        "label": "Open Twitter",
                        "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
                    ]
                ]
            ])
        ]
    ]));
    ```

The `view` action supports the following fields:

| Field    | Required | Type      | Default | Example               | Description                                      |
|----------|----------|-----------|---------|-----------------------|--------------------------------------------------|
| `action` | ✔️       | *string*  | -       | `view`                | Action type (**must be `view`**)                 |
| `label`  | ✔️       | *string*  | -       | `Turn on light`       | Label of the action button in the notification   |
| `url`    | ✔️       | *URL*     | -       | `https://example.com` | URL to open when action is tapped                |
| `clear`  | -️       | *boolean* | `false` | `true`                | Clear notification after action button is tapped |

### Send Android broadcast
_Supported on:_ :material-android:

The `broadcast` action **sends an [Android broadcast](https://developer.android.com/guide/components/broadcasts) intent
when the action button is tapped**. This allows integration into automation apps such as [MacroDroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid)
or [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm), which basically means
you can do everything your phone is capable of. Examples include taking pictures, launching/killing apps, change device
settings, write/read files, etc.

By default, the intent action **`io.heckel.ntfy.USER_ACTION`** is broadcast, though this can be changed with the `intent` parameter (see below).
To send extras, use the `extras` parameter. Currently, **only string extras are supported**.

!!! info
    If you have no idea what this is, check out the [automation apps](subscribe/phone.md#automation-apps) section, which shows
    how to integrate Tasker and MacroDroid **with screenshots**. The action button integration is identical, except that
    you have to use **the intent action `io.heckel.ntfy.USER_ACTION`** instead.

Here's an example using the [`X-Actions` header](#using-a-header):

=== "Command line (curl)"
    ```
    curl \
        -d "Your wife requested you send a picture of yourself." \
        -H "Actions: broadcast, Take picture, extras.cmd=pic, extras.camera=front" \
    ntfy.sh/wifey
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions="broadcast, Take picture, extras.cmd=pic, extras.camera=front" \
        wifey \
        "Your wife requested you send a picture of yourself."
    ```

=== "HTTP"
    ``` http
    POST /wifey HTTP/1.1
    Host: ntfy.sh
    Actions: broadcast, Take picture, extras.cmd=pic, extras.camera=front

    Your wife requested you send a picture of yourself.
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/wifey', {
        method: 'POST',
        body: 'Your wife requested you send a picture of yourself.',
        headers: { 
            'Actions': 'broadcast, Take picture, extras.cmd=pic, extras.camera=front' 
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/wifey", strings.NewReader("Your wife requested you send a picture of yourself."))
    req.Header.Set("Actions", "broadcast, Take picture, extras.cmd=pic, extras.camera=front")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/wifey"
      Headers = @{
        Actions = "broadcast, Take picture, extras.cmd=pic, extras.camera=front"
      }
      Body = "Your wife requested you send a picture of yourself."
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/wifey",
        data="Your wife requested you send a picture of yourself.",
        headers={ "Actions": "broadcast, Take picture, extras.cmd=pic, extras.camera=front" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/wifey', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: broadcast, Take picture, extras.cmd=pic, extras.camera=front",
            'content' => 'Your wife requested you send a picture of yourself.'
        ]
    ]));
    ```

And the same example using [JSON publishing](#publish-as-json):

=== "Command line (curl)"
    ```
    curl ntfy.sh \
      -d '{
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
      }'
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions '[
            {
                "action": "broadcast",
                "label": "Take picture",
                "extras": {
                    "cmd": "pic",
                    "camera": "front"
                }
            }
        ]' \
        wifey \
        "Your wife requested you send a picture of yourself."
    ```

=== "HTTP"
    ``` http
    POST / HTTP/1.1
    Host: ntfy.sh

    {
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
    }
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "wifey",
            message": "Your wife requested you send a picture of yourself.",
            actions: [
                {
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": {
                        "cmd": "pic",
                        "camera": "front"
                    }
                }
            ]
        })
    })
    ```

=== "Go"
    ``` go
    // You should probably use json.Marshal() instead and make a proper struct,
    // but for the sake of the example, this is easier.
    
    body := `{
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    # Powershell requires the 'Depth' argument to equal 3 here to expand 'Extras',
    # otherwise it will read System.Collections.Hashtable in the returned JSON
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh"
      Body = @{
        Topic = "wifey"
        Message = "Your wife requested you send a picture of yourself."
        Actions = ConvertTo-Json -Depth 3 @(
          @{
            Action = "broadcast"
            Label = "Take picture"
            Extras = @{
              CMD ="pic"
              Camera = "front"
            }
          }
        )
      }
      ContentType = "application/json"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "wifey",
            "message": "Your wife requested you send a picture of yourself.",
            "actions": [
                {
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": {
                        "cmd": "pic",
                        "camera": "front"
                    }
                }
            ]
        })
    )
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "wifey",
                "message": "Your wife requested you send a picture of yourself.",
                "actions": [
                    [
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": [
                        "cmd": "pic",
                        "camera": "front"
                    ]
                ]
            ])
        ]
    ]));
    ```

The `broadcast` action supports the following fields:

| Field    | Required | Type             | Default                      | Example                 | Description                                                                                                                                                                            |
|----------|----------|------------------|------------------------------|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `action` | ✔️       | *string*         | -                            | `broadcast`             | Action type (**must be `broadcast`**)                                                                                                                                                  |
| `label`  | ✔️       | *string*         | -                            | `Turn on light`         | Label of the action button in the notification                                                                                                                                         |
| `intent` | -️       | *string*         | `io.heckel.ntfy.USER_ACTION` | `com.example.AN_INTENT` | Android intent name, **default is `io.heckel.ntfy.USER_ACTION`**                                                                                                                       |
| `extras` | -️       | *map of strings* | -                            | *see above*             | Android intent extras. Currently, only string extras are supported. When publishing as JSON, extras are passed as a map. When the simple format is used, use `extras.<param>=<value>`. |
| `clear`  | -️       | *boolean*        | `false`                      | `true`                  | Clear notification after action button is tapped                                                                                                                                       |

### Send HTTP request
_Supported on:_ :material-android: :material-apple: :material-firefox:

The `http` action **sends a HTTP request when the action button is tapped**. You can use this to trigger REST APIs
for whatever systems you have, e.g. opening the garage door, or turning on/off lights.

By default, this action sends a **POST request** (not GET!), though this can be changed with the `method` parameter.
The only required parameter is `url`. Headers can be passed along using the `headers` parameter.  

Here's an example using the [`X-Actions` header](#using-a-header):

=== "Command line (curl)"
    ```
    curl \
        -d "Garage door has been open for 15 minutes. Close it?" \
        -H "Actions: http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" \
        ntfy.sh/myhome
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions="http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" \
        myhome \
        "Garage door has been open for 15 minutes. Close it?"
    ```

=== "HTTP"
    ``` http
    POST /myhome HTTP/1.1
    Host: ntfy.sh
    Actions: http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={"action": "close"}

    Garage door has been open for 15 minutes. Close it?
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'Garage door has been open for 15 minutes. Close it?',
        headers: { 
            'Actions': 'http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}' 
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("Garage door has been open for 15 minutes. Close it?"))
    req.Header.Set("Actions", "http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/myhome"
      Headers = @{
        Actions="http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}"
      }
      Body = "Garage door has been open for 15 minutes. Close it?"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/myhome",
        data="Garage door has been open for 15 minutes. Close it?",
        headers={ "Actions": "http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}",
            'content' => 'Garage door has been open for 15 minutes. Close it?'
        ]
    ]));
    ```

And the same example using [JSON publishing](#publish-as-json):

=== "Command line (curl)"
    ```
    curl ntfy.sh \
      -d '{
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "url": "https://api.mygarage.lan/",
            "method": "PUT",
            "headers": {
                "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
      }'
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --actions '[
            {
              "action": "http",
              "label": "Close door",
              "url": "https://api.mygarage.lan/",
              "method": "PUT",
              "headers": {
                "Authorization": "Bearer zAzsx1sk.."
              },
              "body": "{\"action\": \"close\"}"
            }
        ]' \
        myhome \
        "Garage door has been open for 15 minutes. Close it?"
    ```

=== "HTTP"
    ``` http
    POST / HTTP/1.1
    Host: ntfy.sh

    {
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "url": "https://api.mygarage.lan/",
            "method": "PUT",
            "headers": {
              "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
    }
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message": "Garage door has been open for 15 minutes. Close it?",
            actions: [
              {
                "action": "http",
                "label": "Close door",
                "url": "https://api.mygarage.lan/",
                "method": "PUT",
                "headers": {
                  "Authorization": "Bearer zAzsx1sk.."
                },
                "body": "{\"action\": \"close\"}"
              }
            ]
        })
    })
    ```

=== "Go"
    ``` go
    // You should probably use json.Marshal() instead and make a proper struct,
    // but for the sake of the example, this is easier.
    
    body := `{
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "method": "PUT",
            "url": "https://api.mygarage.lan/",
            "headers": {
              "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    # Powershell requires the 'Depth' argument to equal 3 here to expand 'headers', 
    # otherwise it will read System.Collections.Hashtable in the returned JSON

    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh"
      Body = @{
        Topic   = "myhome"
        Message = "Garage door has been open for 15 minutes. Close it?"
        Actions = ConvertTo-Json -Depth 3 @(
          @{
            Action  = "http"
            Label   = "Close door"
            URL     = "https://api.mygarage.lan/"
            Method  = "PUT"
            Headers = @{
              Authorization = "Bearer zAzsx1sk.."
            }
            Body    = ConvertTo-JSON @{Action = "close"}
          }
        )
      }
      ContentType = "application/json"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "Garage door has been open for 15 minutes. Close it?",
            "actions": [
                {
                  "action": "http",
                  "label": "Close door",
                  "url": "https://api.mygarage.lan/",
                  "method": "PUT",
                  "headers": {
                    "Authorization": "Bearer zAzsx1sk.."
                  },
                  "body": "{\"action\": \"close\"}"
                }
            ]
        })
    )
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "Garage door has been open for 15 minutes. Close it?",
                "actions": [
                    [
                        "action": "http",
                        "label": "Close door",
                        "url": "https://api.mygarage.lan/",
                        "method": "PUT",
                        "headers": [
                            "Authorization": "Bearer zAzsx1sk.."
                         ],
                        "body": "{\"action\": \"close\"}"
                    ]
                ]
            ])
        ]
    ]));
    ```

The `http` action supports the following fields:

| Field     | Required | Type               | Default   | Example                   | Description                                                                                                                                             |
|-----------|----------|--------------------|-----------|---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `action`  | ✔️       | *string*           | -         | `http`                    | Action type (**must be `http`**)                                                                                                                        |
| `label`   | ✔️       | *string*           | -         | `Open garage door`        | Label of the action button in the notification                                                                                                          |
| `url`     | ✔️       | *string*           | -         | `https://ntfy.sh/mytopic` | URL to which the HTTP request will be sent                                                                                                              |
| `method`  | -️       | *GET/POST/PUT/...* | `POST` ⚠️ | `GET`                     | HTTP method to use for request, **default is POST** ⚠️                                                                                                  |
| `headers` | -️       | *map of strings*   | -         | *see above*               | HTTP headers to pass in request. When publishing as JSON, headers are passed as a map. When the simple format is used, use `headers.<header1>=<value>`. |
| `body`    | -️       | *string*           | *empty*   | `some body, somebody?`    | HTTP body                                                                                                                                               |
| `clear`   | -️       | *boolean*          | `false`   | `true`                    | Clear notification after HTTP request succeeds. If the request fails, the notification is not cleared.                                                  |

## Click action
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can define which URL to open when a notification is clicked. This may be useful if your notification is related 
to a Zabbix alert or a transaction that you'd like to provide the deep-link for. Tapping the notification will open
the web browser (or the app) and open the website.

To define a click action for the notification, pass a URL as the value of the `X-Click` header (or its aliase `Click`).
If you pass a website URL (`http://` or `https://`) the web browser will open. If you pass another URI that can be handled
by another app, the responsible app may open. 

Examples:

* `http://` or `https://` will open your browser (or an app if it registered for a URL)
* `mailto:` links will open your mail app, e.g. `mailto:phil@example.com`
* `geo:` links will open Google Maps, e.g. `geo:0,0?q=1600+Amphitheatre+Parkway,+Mountain+View,+CA`
* `ntfy://` links will open ntfy (see [ntfy:// links](subscribe/phone.md#ntfy-links)), e.g. `ntfy://ntfy.sh/stats`
* `twitter://` links will open Twitter, e.g. `twitter://user?screen_name=..`
* ...

Here's an example that will open Reddit when the notification is clicked:

=== "Command line (curl)"
    ```
    curl \
        -d "New messages on Reddit" \
        -H "Click: https://www.reddit.com/message/messages" \
        ntfy.sh/reddit_alerts
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --click="https://www.reddit.com/message/messages" \
        reddit_alerts "New messages on Reddit"
    ```

=== "HTTP"
    ``` http
    POST /reddit_alerts HTTP/1.1
    Host: ntfy.sh
    Click: https://www.reddit.com/message/messages 

    New messages on Reddit
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/reddit_alerts', {
        method: 'POST',
        body: 'New messages on Reddit',
        headers: { 'Click': 'https://www.reddit.com/message/messages' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/reddit_alerts", strings.NewReader("New messages on Reddit"))
    req.Header.Set("Click", "https://www.reddit.com/message/messages")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/reddit_alerts"
      Headers = @{ Click="https://www.reddit.com/message/messages" }
      Body = "New messages on Reddit"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/reddit_alerts",
        data="New messages on Reddit",
        headers={ "Click": "https://www.reddit.com/message/messages" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Click: https://www.reddit.com/message/messages",
            'content' => 'New messages on Reddit'
        ]
    ]));
    ```

## Attachments
_Supported on:_ :material-android: :material-firefox:

You can **send images and other files to your phone** as attachments to a notification. The attachments are then downloaded
onto your phone (depending on size and setting automatically), and can be used from the Downloads folder.

There are two different ways to send attachments: 

* sending [a local file](#attach-local-file) via PUT, e.g. from `~/Flowers/flower.jpg` or `ringtone.mp3`
* or by [passing an external URL](#attach-file-from-a-url) as an attachment, e.g. `https://f-droid.org/F-Droid.apk` 

### Attach local file
To **send a file from your computer** as an attachment, you can send it as the PUT request body. If a message is greater 
than the maximum message size (4,096 bytes) or consists of non UTF-8 characters, the ntfy server will automatically 
detect the mime type and size, and send the message as an attachment file. To send smaller text-only messages or files 
as attachments, you must pass a filename by passing the `X-Filename` header or query parameter (or any of its aliases 
`Filename`, `File` or `f`). 

By default, and how ntfy.sh is configured, the **max attachment size is 15 MB** (with 100 MB total per visitor). 
Attachments **expire after 3 hours**, which typically is plenty of time for the user to download it, or for the Android app
to auto-download it. Please also check out the [other limits below](#limitations).

Here's an example showing how to upload an image:

=== "Command line (curl)"
    ```
    curl \
        -T flower.jpg \
        -H "Filename: flower.jpg" \
        ntfy.sh/flowers
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --file=flower.jpg \
        flowers
    ```

=== "HTTP"
    ``` http
    PUT /flowers HTTP/1.1
    Host: ntfy.sh
    Filename: flower.jpg
    Content-Type: 52312
     
    (binary JPEG data)
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/flowers', {
        method: 'PUT',
        body: document.getElementById("file").files[0],
        headers: { 'Filename': 'flower.jpg' }
    })
    ```

=== "Go"
    ``` go
    file, _ := os.Open("flower.jpg")
    req, _ := http.NewRequest("PUT", "https://ntfy.sh/flowers", file)
    req.Header.Set("Filename", "flower.jpg")
    http.DefaultClient.Do(req)
    ```

=== "Python"
    ``` python
    requests.put("https://ntfy.sh/flowers",
        data=open("flower.jpg", 'rb'),
        headers={ "Filename": "flower.jpg" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/flowers', false, stream_context_create([
        'http' => [
            'method' => 'PUT',
            'header' =>
                "Content-Type: application/octet-stream\r\n" . // Does not matter
                "Filename: flower.jpg",
            'content' => file_get_contents('flower.jpg') // Dangerous for large files 
        ]
    ]));
    ```

Here's what that looks like on Android:

<figure markdown>
  ![image attachment](static/img/android-screenshot-attachment-image.png){ width=500 }
  <figcaption>Image attachment sent from a local file</figcaption>
</figure>

### Attach file from a URL
Instead of sending a local file to your phone, you can use **an external URL** to specify where the attachment is hosted.
This could be a Dropbox link, a file from social media, or any other publicly available URL. Since the files are 
externally hosted, the expiration or size limits from above do not apply here.

To attach an external file, simple pass the `X-Attach` header or query parameter (or any of its aliases `Attach` or `a`)
to specify the attachment URL. It can be any type of file. 

ntfy will automatically try to derive the file name from the URL (e.g `https://example.com/flower.jpg` will yield a 
filename `flower.jpg`). To override this filename, you may send the `X-Filename` header or query parameter (or any of its
aliases `Filename`, `File` or `f`).

Here's an example showing how to attach an APK file:

=== "Command line (curl)"
    ```
    curl \
        -X POST \
        -H "Attach: https://f-droid.org/F-Droid.apk" \
        ntfy.sh/mydownloads
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --attach="https://f-droid.org/F-Droid.apk" \
        mydownloads
    ```

=== "HTTP"
    ``` http
    POST /mydownloads HTTP/1.1
    Host: ntfy.sh
    Attach: https://f-droid.org/F-Droid.apk
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mydownloads', {
        method: 'POST',
        headers: { 'Attach': 'https://f-droid.org/F-Droid.apk' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mydownloads", file)
    req.Header.Set("Attach", "https://f-droid.org/F-Droid.apk")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mydownloads"
      Headers = @{ Attach="https://f-droid.org/F-Droid.apk" }
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.put("https://ntfy.sh/mydownloads",
        headers={ "Attach": "https://f-droid.org/F-Droid.apk" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mydownloads', false, stream_context_create([
        'http' => [
        'method' => 'PUT',
        'header' =>
            "Content-Type: text/plain\r\n" . // Does not matter
            "Attach: https://f-droid.org/F-Droid.apk",
        ]
    ]));
    ```

<figure markdown>
  ![file attachment](static/img/android-screenshot-attachment-file.png){ width=500 }
  <figcaption>File attachment sent from an external URL</figcaption>
</figure>

## Icons
_Supported on:_ :material-android:

You can include an icon that will appear next to the text of the notification. Simply pass the `X-Icon` header or query
parameter (or its alias `Icon`) to specify the URL that the icon is located at. The client will automatically download
the icon (unless it is already cached locally, and less than 24 hours old), and show it in the notification. Icons are 
cached locally in the client until the notification is deleted. **Only JPEG and PNG images are supported at this time**.

Here's an example showing how to include an icon:

=== "Command line (curl)"
    ```
    curl \
        -H "Icon: https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png" \
        -H "Title: Kodi: Resuming Playback" \
        -H "Tags: arrow_forward" \
        -d "The Wire, S01E01" \
        ntfy.sh/tvshows
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --icon="https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png" \
        --title="Kodi: Resuming Playback" \
        --tags="arrow_forward" \
        tvshows \
        "The Wire, S01E01"
    ```

=== "HTTP"
    ``` http
    POST /tvshows HTTP/1.1
    Host: ntfy.sh
    Icon: https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png
    Tags: arrow_forward
    Title: Kodi: Resuming Playback

    The Wire, S01E01
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/tvshows', {
        method: 'POST',
        headers: { 
            'Icon': 'https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png',
            'Title': 'Kodi: Resuming Playback',
            'Tags': 'arrow_forward'
        },
        body: "The Wire, S01E01"
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/tvshows", strings.NewReader("The Wire, S01E01"))
    req.Header.Set("Icon", "https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png")
    req.Header.Set("Tags", "arrow_forward")
    req.Header.Set("Title", "Kodi: Resuming Playback")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/tvshows"
      Headers = @{
        Title = "Kodi: Resuming Playback"
        Tags = "arrow_forward"
        Icon = "https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png"
      }
      Body = "The Wire, S01E01"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/tvshows",
        data="The Wire, S01E01",
        headers={
            "Title": "Kodi: Resuming Playback",
            "Tags": "arrow_forward",
            "Icon": "https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png"
        })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/tvshows', false, stream_context_create([
        'http' => [
        'method' => 'PUT',
        'header' =>
            "Content-Type: text/plain\r\n" . // Does not matter
            "Title: Kodi: Resuming Playback\r\n" .
            "Tags: arrow_forward\r\n" .
            "Icon: https://styles.redditmedia.com/t5_32uhe/styles/communityIcon_xnt6chtnr2j21.png",
        ],
        'content' => "The Wire, S01E01"
    ]));
    ```

Here's an example of how it will look on Android:

<figure markdown>
  ![file attachment](static/img/android-screenshot-icon.png){ width=500 }
  <figcaption>Custom icon from an external URL</figcaption>
</figure>

## E-mail notifications
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can forward messages to e-mail by specifying an address in the header. This can be useful for messages that 
you'd like to persist longer, or to blast-notify yourself on all possible channels. 

Usage is easy: Simply pass the `X-Email` header (or any of its aliases: `X-E-mail`, `Email`, `E-mail`, `Mail`, or `e`).
Only one e-mail address is supported.

Since ntfy does not provide auth (yet), the rate limiting is pretty strict (see [limitations](#limitations)). In the 
default configuration, you get **16 e-mails per visitor** (IP address) and then after that one per hour. On top of 
that, your IP address appears in the e-mail body. This is to prevent abuse.

=== "Command line (curl)"
    ```
    curl \
        -H "Email: phil@example.com" \
        -H "Tags: warning,skull,backup-host,ssh-login" \
        -H "Priority: high" \
        -d "Unknown login from 5.31.23.83 to backups.example.com" \
        ntfy.sh/alerts
    curl -H "Email: phil@example.com" -d "You've Got Mail" 
    curl -d "You've Got Mail" "ntfy.sh/alerts?email=phil@example.com"
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --email=phil@example.com \
        --tags=warning,skull,backup-host,ssh-login \
        --priority=high \
        alerts "Unknown login from 5.31.23.83 to backups.example.com"
    ```

=== "HTTP"
    ``` http
    POST /alerts HTTP/1.1
    Host: ntfy.sh
    Email: phil@example.com
    Tags: warning,skull,backup-host,ssh-login
    Priority: high

    Unknown login from 5.31.23.83 to backups.example.com
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/alerts', {
        method: 'POST',
        body: "Unknown login from 5.31.23.83 to backups.example.com",
        headers: { 
            'Email': 'phil@example.com',
            'Tags': 'warning,skull,backup-host,ssh-login',
            'Priority': 'high'
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/alerts", 
        strings.NewReader("Unknown login from 5.31.23.83 to backups.example.com"))
    req.Header.Set("Email", "phil@example.com")
    req.Header.Set("Tags", "warning,skull,backup-host,ssh-login")
    req.Header.Set("Priority", "high")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/alerts"
      Headers = @{
        Title = "Low disk space alert"
        Priority = "high"
        Tags = "warning,skull,backup-host,ssh-login")
        Email = "phil@example.com"
      }
      Body = "Unknown login from 5.31.23.83 to backups.example.com"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/alerts",
        data="Unknown login from 5.31.23.83 to backups.example.com",
        headers={ 
            "Email": "phil@example.com",
            "Tags": "warning,skull,backup-host,ssh-login",
            "Priority": "high"
        })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Email: phil@example.com\r\n" .
                "Tags: warning,skull,backup-host,ssh-login\r\n" .
                "Priority: high",
            'content' => 'Unknown login from 5.31.23.83 to backups.example.com'
        ]
    ]));
    ```

Here's what that looks like in Google Mail:

<figure markdown>
  ![e-mail notification](static/img/screenshot-email.png){ width=600 }
  <figcaption>E-mail notification</figcaption>
</figure>

## E-mail publishing
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can publish messages to a topic via e-mail, i.e. by sending an email to a specific address. For instance, you can
publish a message to the topic `sometopic` by sending an e-mail to `ntfy-sometopic@ntfy.sh`. This is useful for e-mail 
based integrations such as for statuspage.io (though these days most services also support webhooks and HTTP calls).

Depending on the [server configuration](config.md#e-mail-publishing), the e-mail address format can have a prefix to 
prevent spam on topics. For ntfy.sh, the prefix is configured to `ntfy-`, meaning that the general e-mail address 
format is:

```
ntfy-$topic@ntfy.sh
```

If [access control](config.md#access-control) is enabled, and the target topic does not support anonymous writes, e-mail publishing won't work without providing an authorized access token. That will change the format of the e-mail's recipient address to
```
ntfy-$topic+$token@ntfy.sh
```

As of today, e-mail publishing only supports adding a [message title](#message-title) (the e-mail subject). Tags, priority,
delay and other features are not supported (yet). Here's an example that will publish a message with the 
title `You've Got Mail` to topic `sometopic` (see [ntfy.sh/sometopic](https://ntfy.sh/sometopic)):

<figure markdown>
  ![e-mail publishing](static/img/screenshot-email-publishing-gmail.png){ width=500 }
  <figcaption>Publishing a message via e-mail</figcaption>
</figure>

## Phone calls
_Supported on:_ :material-android: :material-apple: :material-firefox:

You can use ntfy to call a phone and **read the message out loud using text-to-speech**. 
Similar to email notifications, this can be useful to blast-notify yourself on all possible channels, or to notify people that do not have 
the ntfy app installed on their phone.

**Phone numbers have to be previously verified** (via the [web app](https://ntfy.sh/account)), so this feature is 
**only available to authenticated users** (no anonymous phone calls). To forward a message as a voice call, pass a phone
number in the `X-Call` header (or its alias: `Call`), prefixed with a plus sign and the country code, e.g. `+12223334444`. 
You may also simply pass `yes` as a value to pick the first of your verified phone numbers. 
On ntfy.sh, this feature is only supported to [ntfy Pro](https://ntfy.sh/app) plans.

<figure markdown>
  ![phone number verification](static/img/web-phone-verify.png)
  <figcaption>Phone number verification in the <a href="https://ntfy.sh/account">web app</a></figcaption>
</figure>

As of today, the text-to-speed voice used will only support English. If there is demand for other languages, we'll
be happy to add support for that. Please [open an issue on GitHub](https://github.com/binwiederhier/ntfy/issues).

!!! info
    You are responsible for the message content, and **you must abide by the [Twilio Acceptable Use Policy](https://www.twilio.com/en-us/legal/aup)**.
    This particularly means that you must not use this feature to send unsolicited messages, or messages that are illegal or
    violate the rights of others. Please read the policy for details. Failure to do so may result in your account being suspended or terminated.

Here's how you use it:

=== "Command line (curl)"
    ```
    curl \
        -u :tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 \
        -H "Call: +12223334444" \
        -d "Your garage seems to be on fire. You should probably check that out." \
        ntfy.sh/alerts
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --token=tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 \
        --call=+12223334444 \
        alerts "Your garage seems to be on fire. You should probably check that out."
    ```

=== "HTTP"
    ``` http
    POST /alerts HTTP/1.1
    Host: ntfy.sh
    Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2
    Call: +12223334444

    Your garage seems to be on fire. You should probably check that out.
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/alerts', {
        method: 'POST',
        body: "Your garage seems to be on fire. You should probably check that out.",
        headers: { 
            'Authorization': 'Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2',
            'Call': '+12223334444'
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/alerts", 
        strings.NewReader("Your garage seems to be on fire. You should probably check that out."))
    req.Header.Set("Call", "+12223334444")
    req.Header.Set("Authorization", "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/alerts"
      Headers = @{
        Authorization = "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2"
        Call = "+12223334444"
      }
      Body = "Your garage seems to be on fire. You should probably check that out."
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/alerts",
        data="Your garage seems to be on fire. You should probably check that out.",
        headers={ 
            "Authorization": "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2",
            "Call": "+12223334444"
        })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2\r\n" .
                "Call: +12223334444",
            'content' => 'Your garage seems to be on fire. You should probably check that out.'
        ]
    ]));
    ```

Here's what a phone call from ntfy sounds like:

<audio controls>
    <source src="../static/audio/ntfy-phone-call.mp3" type="audio/mpeg">
    <source src="../static/audio/ntfy-phone-call.ogg" type="audio/ogg">
</audio>

Audio transcript:

> You have a notification from ntfy on topic alerts.        
> Message: Your garage seems to be on fire. You should probably check that out. End message.   
> This message was sent by user phil. It will be repeated up to three times.

## Authentication
Depending on whether the server is configured to support [access control](config.md#access-control), some topics
may be read/write protected so that only users with the correct credentials can subscribe or publish to them.
To publish/subscribe to protected topics, you can: 

* Use [username & password](#username-password) via Basic auth, e.g. `Authorization: Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk`
* Use [access tokens](#bearer-auth) via Bearer/Basic auth, e.g. `Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2`
* or use either with the [`auth` query parameter](#query-param), e.g. `?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw`

!!! warning
    When using Basic auth, base64 only encodes username and password. It **is not encrypting it**. For your 
    self-hosted server, **be sure to use HTTPS to avoid eavesdropping** and exposing your password. 

### Username + password
The simplest way to authenticate against a ntfy server is to use [Basic auth](https://en.wikipedia.org/wiki/Basic_access_authentication).
Here's an example with a user `testuser` and password `fakepassword`:

=== "Command line (curl)"
    ```
    curl \
      -u testuser:fakepassword \
      -d "Look ma, with auth" \
      https://ntfy.example.com/mysecrets
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
      -u testuser:fakepassword \
      ntfy.example.com/mysecrets \
      "Look ma, with auth"
    ```

=== "HTTP"
    ``` http
    POST /mysecrets HTTP/1.1
    Host: ntfy.example.com
    Authorization: Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk

    Look ma, with auth
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.example.com/mysecrets', {
        method: 'POST', // PUT works too
        body: 'Look ma, with auth',
        headers: {
            'Authorization': 'Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk'
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.example.com/mysecrets",
    strings.NewReader("Look ma, with auth"))
    req.Header.Set("Authorization", "Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell 7+"
    ``` powershell
    # Get the credentials from the user
    $Credential = Get-Credential testuser

    # Alternatively, create a PSCredential object with the password from scratch
    $Credential = [PSCredential]::new("testuser", (ConvertTo-SecureString "password" -AsPlainText -Force))
    
    # Note that the Authentication parameter requires PowerShell 7 or later
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets"
      Authentication = "Basic"
      Credential = $Credential
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "PowerShell 5 and earlier"
    ``` powershell
    # With PowerShell 5 or earlier, we need to create the base64 username:password string ourselves
    $CredentialString = "$($Credential.Username):$($Credential.GetNetworkCredential().Password)"
    $EncodedCredential = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($CredentialString))
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets"
      Headers = @{ Authorization = "Basic $EncodedCredential"}
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.example.com/mysecrets",
    data="Look ma, with auth",
    headers={
        "Authorization": "Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk"
    })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.example.com/mysecrets', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                'Content-Type: text/plain\r\n' .
                'Authorization: Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk',
            'content' => 'Look ma, with auth'
        ]
    ]));
    ```

To generate the `Authorization` header, use **standard base64** to encode the colon-separated `<username>:<password>` 
and prepend the word `Basic`, i.e. `Authorization: Basic base64(<username>:<password>)`. Here's some pseudo-code that 
hopefully explains it better:

```
username   = "testuser"
password   = "fakepassword"
authHeader = "Basic " + base64(username + ":" + password) // -> Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk
```

The following command will generate the appropriate value for you on *nix systems:

```
echo "Basic $(echo -n 'testuser:fakepassword' | base64)"
```

### Access tokens
In addition to username/password auth, ntfy also provides authentication via access tokens. Access tokens are useful
to avoid having to configure your password across multiple publishing/subscribing applications. For instance, you may
want to use a dedicated token to publish from your backup host, and one from your home automation system.

You can create access tokens using the `ntfy token` command, or in the web app in the "Account" section (when logged in).
See [access tokens](config.md#access-tokens) for details.

Once an access token is created, you can use it to authenticate against the ntfy server, e.g. when you publish or 
subscribe to topics. Here's an example using [Bearer auth](https://swagger.io/docs/specification/authentication/bearer-authentication/),
with the token `tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2`:

=== "Command line (curl)"
    ```
    curl \
      -H "Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2" \
      -d "Look ma, with auth" \
      https://ntfy.example.com/mysecrets
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
      --token tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 \
      ntfy.example.com/mysecrets \
      "Look ma, with auth"
    ```

=== "HTTP"
    ``` http
    POST /mysecrets HTTP/1.1
    Host: ntfy.example.com
    Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2

    Look ma, with auth
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.example.com/mysecrets', {
        method: 'POST', // PUT works too
        body: 'Look ma, with auth',
        headers: {
            'Authorization': 'Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2'
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.example.com/mysecrets",
    strings.NewReader("Look ma, with auth"))
    req.Header.Set("Authorization", "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell 7+"
    ``` powershell
    # With PowerShell 7 or greater, we can use the Authentication and Token parameters
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets"
      Authorization = "Bearer"
      Token = "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2"
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "PowerShell 5 and earlier"
    ``` powershell
    # In PowerShell 5 and below, we can only send the Bearer token as a string in the Headers
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets"
      Headers = @{ Authorization = "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2" }
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.example.com/mysecrets",
    data="Look ma, with auth",
    headers={
        "Authorization": "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2"
    })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.example.com/mysecrets', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                'Content-Type: text/plain\r\n' .
                'Authorization: Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2',
            'content' => 'Look ma, with auth'
        ]
    ]));
    ```

Alternatively, you can use [Basic Auth](https://en.wikipedia.org/wiki/Basic_access_authentication) to send the 
access token. When sending an empty username, the basic auth password is treated by the ntfy server as an 
access token. This is primarily useful to make `curl` calls easier, e.g. `curl -u:tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 ...`:

=== "Command line (curl)"
    ```
    curl \
      -u :tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 \
      -d "Look ma, with auth" \
      https://ntfy.example.com/mysecrets
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
      --token tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2 \
      ntfy.example.com/mysecrets \
      "Look ma, with auth"
    ```

=== "HTTP"
    ``` http
    POST /mysecrets HTTP/1.1
    Host: ntfy.example.com
    Authorization: Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy

    Look ma, with auth
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.example.com/mysecrets', {
        method: 'POST', // PUT works too
        body: 'Look ma, with auth',
        headers: {
            'Authorization': 'Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy'
        }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.example.com/mysecrets",
    strings.NewReader("Look ma, with auth"))
    req.Header.Set("Authorization", "Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    # Note that PSCredentials *must* have a username, so we fall back to placing the authorization in the Headers as with PowerShell 5
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets"
      Headers = @{
        Authorization = "Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy"
      }
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.example.com/mysecrets",
    data="Look ma, with auth",
    headers={
        "Authorization": "Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy"
    })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.example.com/mysecrets', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                'Content-Type: text/plain\r\n' .
                'Authorization: Basic OnRrX0FnUWRxN21WQm9GRDM3elFWTjI5Umh1TXpOSXoy',
            'content' => 'Look ma, with auth'
        ]
    ]));
    ```


### Query param
Here's an example using the `auth` query parameter:

=== "Command line (curl)"
    ```
    curl \
      -d "Look ma, with auth" \
      "https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw"
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
      -u testuser:fakepassword \
      ntfy.example.com/mysecrets \
      "Look ma, with auth"
    ```

=== "HTTP"
    ``` http
    POST /mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw HTTP/1.1
    Host: ntfy.example.com

    Look ma, with auth
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw', {
        method: 'POST', // PUT works too
        body: 'Look ma, with auth'
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw",
        strings.NewReader("Look ma, with auth"))
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw"
      Body = "Look ma, with auth"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw",
    data="Look ma, with auth"
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.example.com/mysecrets?auth=QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' => 'Content-Type: text/plain',
            'content' => 'Look ma, with auth'
        ]
    ]));
    ```

To generate the value of the `auth` parameter, encode the value of the `Authorization` header (see above) using 
**raw base64 encoding** (like base64, but strip any trailing `=`). Here's some pseudo-code that hopefully 
explains it better:

```
username   = "testuser"
password   = "fakepassword"
authHeader = "Basic " + base64(username + ":" + password) // -> Basic dGVzdHVzZXI6ZmFrZXBhc3N3b3Jk
authParam  = base64_raw(authHeader) // -> QmFzaWMgZEdWemRIVnpaWEk2Wm1GclpYQmhjM04zYjNKaw (no trailing =)

// If your language does not have a function to encode raw base64, simply use normal base64
// and REMOVE TRAILING "=" characters. 
```

The following command will generate the appropriate value for you on *nix systems:

```
echo -n "Basic `echo -n 'testuser:fakepassword' | base64`" | base64 | tr -d '='
```

For access tokens, you can use this instead:

```
echo -n "Bearer faketoken" | base64 | tr -d '='
```

## Advanced features

### Message caching
!!! info
    If `Cache: no` is used, messages will only be delivered to connected subscribers, and won't be re-delivered if a 
    client re-connects. If a subscriber has (temporary) network issues or is reconnecting momentarily, 
    **messages might be missed**.

By default, the ntfy server caches messages on disk for 12 hours (see [message caching](config.md#message-cache)), so
all messages you publish are stored server-side for a little while. The reason for this is to overcome temporary 
client-side network disruptions, but arguably this feature also may raise privacy concerns.

To avoid messages being cached server-side entirely, you can set `X-Cache` header (or its alias: `Cache`) to `no`. 
This will make sure that your message is not cached on the server, even if server-side caching is enabled. Messages
are still delivered to connected subscribers, but [`since=`](subscribe/api.md#fetch-cached-messages) and 
[`poll=1`](subscribe/api.md#poll-for-messages) won't return the message anymore.

=== "Command line (curl)"
    ```
    curl -H "X-Cache: no" -d "This message won't be stored server-side" ntfy.sh/mytopic
    curl -H "Cache: no" -d "This message won't be stored server-side" ntfy.sh/mytopic
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --no-cache \
        mytopic "This message won't be stored server-side"
    ```

=== "HTTP"
    ``` http
    POST /mytopic HTTP/1.1
    Host: ntfy.sh
    Cache: no

    This message won't be stored server-side
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mytopic', {
        method: 'POST',
        body: 'This message won't be stored server-side',
        headers: { 'Cache': 'no' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mytopic", strings.NewReader("This message won't be stored server-side"))
    req.Header.Set("Cache", "no")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mytopic"
      Headers = @{ Cache="no" }
      Body = "This message won't be stored server-side"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/mytopic",
        data="This message won't be stored server-side",
        headers={ "Cache": "no" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Cache: no",
            'content' => 'This message won't be stored server-side'
        ]
    ]));
    ```

### Disable Firebase
!!! info
    If `Firebase: no` is used and [instant delivery](subscribe/phone.md#instant-delivery) isn't enabled in the Android 
    app (Google Play variant only), **message delivery will be significantly delayed (up to 15 minutes)**. To overcome 
    this delay, simply enable instant delivery.

The ntfy server can be configured to use [Firebase Cloud Messaging (FCM)](https://firebase.google.com/docs/cloud-messaging)
(see [Firebase config](config.md#firebase-fcm)) for message delivery on Android (to minimize the app's battery footprint). 
The ntfy.sh server is configured this way, meaning that all messages published to ntfy.sh are also published to corresponding
FCM topics.

If you'd like to avoid forwarding messages to Firebase, you can set the `X-Firebase` header (or its alias: `Firebase`)
to `no`. This will instruct the server not to forward messages to Firebase.

=== "Command line (curl)"
    ```
    curl -H "X-Firebase: no" -d "This message won't be forwarded to FCM" ntfy.sh/mytopic
    curl -H "Firebase: no" -d "This message won't be forwarded to FCM" ntfy.sh/mytopic
    ```

=== "ntfy CLI"
    ```
    ntfy publish \
        --no-firebase \
        mytopic "This message won't be forwarded to FCM"
    ```

=== "HTTP"
    ``` http
    POST /mytopic HTTP/1.1
    Host: ntfy.sh
    Firebase: no

    This message won't be forwarded to FCM
    ```

=== "JavaScript"
    ``` javascript
    fetch('https://ntfy.sh/mytopic', {
        method: 'POST',
        body: 'This message won't be forwarded to FCM',
        headers: { 'Firebase': 'no' }
    })
    ```

=== "Go"
    ``` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mytopic", strings.NewReader("This message won't be forwarded to FCM"))
    req.Header.Set("Firebase", "no")
    http.DefaultClient.Do(req)
    ```

=== "PowerShell"
    ``` powershell
    $Request = @{
      Method = "POST"
      URI = "https://ntfy.sh/mytopic"
      Headers = @{ Firebase="no" }
      Body = "This message won't be forwarded to FCM"
    }
    Invoke-RestMethod @Request
    ```

=== "Python"
    ``` python
    requests.post("https://ntfy.sh/mytopic",
        data="This message won't be forwarded to FCM",
        headers={ "Firebase": "no" })
    ```

=== "PHP"
    ``` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Firebase: no",
            'content' => 'This message won't be stored server-side'
        ]
    ]));
    ```

### UnifiedPush
!!! info
    This setting is not relevant to users, only to app developers and people interested in [UnifiedPush](https://unifiedpush.org). 

[UnifiedPush](https://unifiedpush.org) is a standard for receiving push notifications without using the Google-owned
[Firebase Cloud Messaging (FCM)](https://firebase.google.com/docs/cloud-messaging) service. It puts push notifications
in the control of the user. ntfy can act as a **UnifiedPush distributor**, forwarding messages to apps that support it.

When publishing messages to a topic, apps using ntfy as a UnifiedPush distributor can set the `X-UnifiedPush` header or query
parameter (or any of its aliases `unifiedpush` or `up`) to `1` to [disable Firebase](#disable-firebase). As of today, this
option is mostly equivalent to `Firebase: no`, but was introduced to allow future flexibility. The flag additionally 
enables auto-detection of the message encoding. If the message is binary, it'll be encoded as base64.

### Matrix Gateway
The ntfy server implements a [Matrix Push Gateway](https://spec.matrix.org/v1.2/push-gateway-api/) (in combination with
[UnifiedPush](https://unifiedpush.org) as the [Provider Push Protocol](https://unifiedpush.org/developers/gateway/)). This makes it easier to integrate
with self-hosted [Matrix](https://matrix.org/) servers (such as [synapse](https://github.com/matrix-org/synapse)), since 
you don't have to set up a separate push proxy (such as [common-proxies](https://github.com/UnifiedPush/common-proxies)).

In short, ntfy accepts Matrix messages on the `/_matrix/push/v1/notify` endpoint (see [Push Gateway API](https://spec.matrix.org/v1.2/push-gateway-api/)), 
and forwards them to the ntfy topic defined in the `pushkey` of the message. The message will then be forwarded to the
ntfy Android app, and passed on to the Matrix client there.

There is a nice diagram in the [Push Gateway docs](https://spec.matrix.org/v1.2/push-gateway-api/). In this diagram, the
ntfy server plays the role of the Push Gateway, as well as the Push Provider. UnifiedPush is the Provider Push Protocol.

!!! info
    This is not a generic Matrix Push Gateway. It only works in combination with UnifiedPush and ntfy.

## Public topics
Obviously all topics on ntfy.sh are public, but there are a few designated topics that are used in examples, and topics
that you can use to try out what [authentication and access control](#authentication) looks like.

| Topic                                          | User                              | Permissions                                          | Description                          |
|------------------------------------------------|-----------------------------------|------------------------------------------------------|--------------------------------------|
| [announcements](https://ntfy.sh/announcements) | `*` (unauthenticated)             | Read-only for everyone                               | Release announcements and such       |
| [stats](https://ntfy.sh/stats)                 | `*` (unauthenticated)             | Read-only for everyone                               | Daily statistics about ntfy.sh usage |

## Limitations
There are a few limitations to the API to prevent abuse and to keep the server healthy. Almost all of these settings 
are configurable via the server side [rate limiting settings](config.md#rate-limiting). Most of these limits you won't run into,
but just in case, let's list them all:

| Limit                      | Description                                                                                                                                                                                                             |
|----------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Message length**         | Each message can be up to 4,096 bytes long. Longer messages are treated as [attachments](#attachments).                                                                                                                 |
| **Requests**               | By default, the server is configured to allow 60 requests per visitor at once, and then refills the your allowed requests bucket at a rate of one request per 5 seconds.                                                |
| **Daily messages**         | By default, the number of messages is governed by the request limits. This can be overridden. On ntfy.sh, the daily message limit is 250.                                                                               |
| **E-mails**                | By default, the server is configured to allow sending 16 e-mails per visitor at once, and then refills the your allowed e-mail bucket at a rate of one per hour. On ntfy.sh, the daily limit is 5.                      |
| **Phone calls**            | By default, the server does not allow any phone calls, except for users with a tier that has a call limit.                                                                                                              |
| **Subscription limit**     | By default, the server allows each visitor to keep 30 connections to the server open.                                                                                                                                   |
| **Attachment size limit**  | By default, the server allows attachments up to 15 MB in size, up to 100 MB in total per visitor and up to 5 GB across all visitors. On ntfy.sh, the attachment size limit is 2 MB, and the per-visitor total is 20 MB. |
| **Attachment expiry**      | By default, the server deletes attachments after 3 hours and thereby frees up space from the total visitor attachment limit.                                                                                            |
| **Attachment bandwidth**   | By default, the server allows 500 MB of GET/PUT/POST traffic for attachments per visitor in a 24 hour period. Traffic exceeding that is rejected. On ntfy.sh, the daily bandwidth limit is 200 MB.                      |
| **Total number of topics** | By default, the server is configured to allow 15,000 topics. The ntfy.sh server has higher limits though.                                                                                                               |

These limits can be changed on a per-user basis using [tiers](config.md#tiers). If [payments](config.md#payments) are enabled, a user tier can be changed by purchasing
a higher tier. ntfy.sh offers multiple paid tiers, which allows for much hier limits than the ones listed above. 

## List of all parameters
The following is a list of all parameters that can be passed when publishing a message. Parameter names are **case-insensitive**
when used in **HTTP headers**, and must be **lowercase** when used as **query parameters in the URL**. They are listed in the 
table in their canonical form.

!!! info
    ntfy supports UTF-8 in HTTP headers, but [not every library or programming language does](https://www.jmix.io/blog/utf-8-in-http-headers/).
    If non-ASCII characters are causing issues for you in the title (i.e. you're seeing `?` symbols), you may also encode any
    header as [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047#section-2), e.g. `=?UTF-8?B?8J+HqfCfh6o=?=` ([base64](https://en.wikipedia.org/wiki/Base64)),
    or `=?UTF-8?Q?=C3=84pfel?=` ([quoted-printable](https://en.wikipedia.org/wiki/Quoted-printable)).

| Parameter       | Aliases                                    | Description                                                                                   |
|-----------------|--------------------------------------------|-----------------------------------------------------------------------------------------------|
| `X-Message`     | `Message`, `m`                             | Main body of the message as shown in the notification                                         |
| `X-Title`       | `Title`, `t`                               | [Message title](#message-title)                                                               |
| `X-Priority`    | `Priority`, `prio`, `p`                    | [Message priority](#message-priority)                                                         |
| `X-Tags`        | `Tags`, `Tag`, `ta`                        | [Tags and emojis](#tags-emojis)                                                               |
| `X-Delay`       | `Delay`, `X-At`, `At`, `X-In`, `In`        | Timestamp or duration for [delayed delivery](#scheduled-delivery)                             |
| `X-Actions`     | `Actions`, `Action`                        | JSON array or short format of [user actions](#action-buttons)                                 |
| `X-Click`       | `Click`                                    | URL to open when [notification is clicked](#click-action)                                     |
| `X-Attach`      | `Attach`, `a`                              | URL to send as an [attachment](#attachments), as an alternative to PUT/POST-ing an attachment |
| `X-Markdown`    | `Markdown`, `md`                           | Enable [Markdown formatting](#markdown-formatting) in the notification body                   |
| `X-Icon`        | `Icon`                                     | URL to use as notification [icon](#icons)                                                     |
| `X-Filename`    | `Filename`, `file`, `f`                    | Optional [attachment](#attachments) filename, as it appears in the client                     |
| `X-Email`       | `X-E-Mail`, `Email`, `E-Mail`, `mail`, `e` | E-mail address for [e-mail notifications](#e-mail-notifications)                              |
| `X-Call`        | `Call`                                     | Phone number for [phone calls](#phone-calls)                                                  |
| `X-Cache`       | `Cache`                                    | Allows disabling [message caching](#message-caching)                                          |
| `X-Firebase`    | `Firebase`                                 | Allows disabling [sending to Firebase](#disable-firebase)                                     |
| `X-UnifiedPush` | `UnifiedPush`, `up`                        | [UnifiedPush](#unifiedpush) publish option, only to be used by UnifiedPush apps               |
| `X-Poll-ID`     | `Poll-ID`                                  | Internal parameter, used for [iOS push notifications](config.md#ios-instant-notifications)    |
| `Authorization` | -                                          | If supported by the server, you can [login to access](#authentication) protected topics       |
| `Content-Type`  | -                                          | If set to `text/markdown`, [Markdown formatting](#markdown-formatting) is enabled             |
