# Subscribe via ntfy CLI

In addition to subscribing via the [web UI](web.md), the [phone app](phone.md), or the [API](api.md), you can subscribe
to topics via the ntfy CLI. The CLI is included in the same `ntfy` binary that can be used to [self-host a server](../install.md).

!!! info
The **ntfy CLI is not required to send or receive messages**. You can instead [send messages with curl](../publish.md),
and even use it to [subscribe to topics](api.md). It may be a little more convenient to use the ntfy CLI than writing
your own script. It all depends on the use case. 😀

## Install + configure

To install the ntfy CLI, simply **follow the steps outlined on the [install page](../install.md)**. The ntfy server and
client are the same binary, so it's all very convenient. After installing, you can (optionally) configure the client
by creating `~/.config/ntfy/client.yml` (for the non-root user), or `/etc/ntfy/client.yml` (for the root user). You
can find a [skeleton config](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml) on GitHub.

If you just want to use [ntfy.sh](https://ntfy.sh), you don't have to change anything. If you **self-host your own server**,
you may want to edit the `default-host` option:

```yaml
# Base URL used to expand short topic names in the "ntfy publish" and "ntfy subscribe" commands.
# If you self-host a ntfy server, you'll likely want to change this.
#
default-host: https://ntfy.myhost.com
```

## Publish messages

You can send messages with the ntfy CLI using the `ntfy publish` command (or any of its aliases `pub`, `send` or
`trigger`). There are a lot of examples on the page about [publishing messages](../publish.md), but here are a few
quick ones:

=== "Simple send"

    ```sh
    ntfy publish mytopic This is a message
    ntfy publish mytopic "This is a message"
    ntfy pub mytopic "This is a message"
    ```

=== "Send with title, priority, and tags"

    ```sh
    ntfy publish \
        --title="Thing sold on eBay" \
        --priority=high \
        --tags=partying_face \
        mytopic \
        "Somebody just bought the thing that you sell"
    ```

=== "Send at 8:30am"

    ```sh
    ntfy pub --at=8:30am delayed_topic Laterzz
    ```

=== "Triggering a webhook"

    ```sh
    ntfy trigger mywebhook
    ntfy pub mywebhook
    ```

### Attaching a local file

You can easily upload and attach a local file to a notification:

```sh
$ ntfy pub --file README.md mytopic | jq .
{
  "id": "meIlClVLABJQ",
  "time": 1655825460,
  "event": "message",
  "topic": "mytopic",
  "message": "You received a file: README.md",
  "attachment": {
    "name": "README.md",
    "type": "text/plain; charset=utf-8",
    "size": 2892,
    "expires": 1655836260,
    "url": "https://ntfy.sh/file/meIlClVLABJQ.txt"
  }
}
```

### Wait for PID/command

If you have a long-running command and want to **publish a notification when the command completes**,
you may wrap it with `ntfy publish --wait-cmd` (aliases: `--cmd`, `--done`). Or, if you forgot to wrap it, and the
command is already running, you can wait for the process to complete with `ntfy publish --wait-pid` (alias: `--pid`).

Run a command and wait for it to complete (here: `rsync ...`):

```sh
$ ntfy pub --wait-cmd mytopic rsync -av ./ root@example.com:/backups/ | jq .
{
  "id": "Re0rWXZQM8WB",
  "time": 1655825624,
  "event": "message",
  "topic": "mytopic",
  "message": "Command succeeded after 56.553s: rsync -av ./ root@example.com:/backups/"
}
```

Or, if you already started the long-running process and want to wait for it using its process ID (PID), you can do this:

=== "Using a PID directly"

    ```sh
    $ ntfy pub --wait-pid 8458 mytopic | jq .
    {
      "id": "orM6hJKNYkWb",
      "time": 1655825827,
      "event": "message",
      "topic": "mytopic",
      "message": "Process with PID 8458 exited after 2.003s"
    }
    ```

=== "Using a `pidof`"

    ```sh
    $ ntfy pub --wait-pid $(pidof rsync) mytopic | jq .
    {
      "id": "orM6hJKNYkWb",
      "time": 1655825827,
      "event": "message",
      "topic": "mytopic",
      "message": "Process with PID 8458 exited after 2.003s"
    }
    ```

## Subscribe to topics

You can subscribe to topics using `ntfy subscribe`. Depending on how it is called, this command
will either print or execute a command for every arriving message. There are a few different ways
in which the command can be run:

### Stream messages as JSON

```sh
ntfy subscribe TOPIC
```

If you run the command like this, it prints the JSON representation of every incoming message. This is useful
when you have a command that wants to stream-read incoming JSON messages. Unless `--poll` is passed, this command
stays open forever.

```sh
$ ntfy sub mytopic
{"id":"nZ8PjH5oox","time":1639971913,"event":"message","topic":"mytopic","message":"hi there"}
{"id":"sekSLWTujn","time":1639972063,"event":"message","topic":"mytopic",priority:5,"message":"Oh no!"}
...
```

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-1.mp4"></video>
  <figcaption>Subscribe in JSON mode</figcaption>
</figure>

### Run command for every message

```sh
ntfy subscribe TOPIC COMMAND
```

If you run it like this, a COMMAND is executed for every incoming messages. Scroll down to see a list of available
environment variables. Here are a few examples:

```sh
ntfy sub mytopic 'notify-send "$m"'
ntfy sub topic1 /my/script.sh
ntfy sub topic1 'echo "Message $m was received. Its title was $t and it had priority $p'
```

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-2.webm"></video>
  <figcaption>Execute command on incoming messages</figcaption>
</figure>

The message fields are passed to the command as environment variables and can be used in scripts. Note that since
these are environment variables, you typically don't have to worry about quoting too much, as long as you enclose them
in double-quotes, you should be fine:

| Variable         | Aliases                    | Description                            |
| ---------------- | -------------------------- | -------------------------------------- |
| `$NTFY_ID`       | `$id`                      | Unique message ID                      |
| `$NTFY_TIME`     | `$time`                    | Unix timestamp of the message delivery |
| `$NTFY_TOPIC`    | `$topic`                   | Topic name                             |
| `$NTFY_MESSAGE`  | `$message`, `$m`           | Message body                           |
| `$NTFY_TITLE`    | `$title`, `$t`             | Message title                          |
| `$NTFY_PRIORITY` | `$priority`, `$prio`, `$p` | Message priority (1=min, 5=max)        |
| `$NTFY_TAGS`     | `$tags`, `$tag`, `$ta`     | Message tags (comma separated list)    |
| `$NTFY_RAW`      | `$raw`                     | Raw JSON message                       |

### Subscribe to multiple topics

```sh
ntfy subscribe --from-config
```

To subscribe to multiple topics at once, and run different commands for each one, you can use `ntfy subscribe --from-config`,
which will read the `subscribe` config from the config file. Please also check out the [ntfy-client systemd service](#using-the-systemd-service).

Here's an example config file that subscribes to three different topics, executing a different command for each of them:

=== "~/.config/ntfy/client.yml (Linux)"

    ```yaml
    subscribe:
    - topic: echo-this
      command: 'echo "Message received: $message"'
    - topic: alerts
      command: notify-send -i /usr/share/ntfy/logo.png "Important" "$m"
      if:
        priority: high,urgent
    - topic: calc
      command: 'gnome-calculator 2>/dev/null &'
    - topic: print-temp
      command: |
            echo "You can easily run inline scripts, too."
            temp="$(sensors | awk '/Pack/ { print substr($4,2,2) }')"
            if [ $temp -gt 80 ]; then
              echo "Warning: CPU temperature is $temp. Too high."
            else
              echo "CPU temperature is $temp. That's alright."
            fi
    ```

=== "~/Library/Application Support/ntfy/client.yml (macOS)"

    ```yaml
    subscribe:
      - topic: echo-this
        command: 'echo "Message received: $message"'
      - topic: alerts
        command: osascript -e "display notification \"$message\""
        if:
          priority: high,urgent
      - topic: calc
        command: open -a Calculator
    ```

=== "%AppData%\ntfy\client.yml (Windows)"

    ```yaml
    subscribe:
    - topic: echo-this
      command: 'echo Message received: %message%'
    - topic: alerts
      command: |
        notifu /m "%NTFY_MESSAGE%"
        exit 0
      if:
        priority: high,urgent
    - topic: calc
      command: calc
    ```

In this example, when `ntfy subscribe --from-config` is executed:

- Messages to `echo-this` simply echos to standard out
- Messages to `alerts` display as desktop notification for high priority messages using [notify-send](https://manpages.ubuntu.com/manpages/focal/man1/notify-send.1.html) (Linux),
  [notifu](https://www.paralint.com/projects/notifu/) (Windows) or `osascript` (macOS)
- Messages to `calc` open the calculator 😀 (_because, why not_)
- Messages to `print-temp` execute an inline script and print the CPU temperature (Linux version only)

I hope this shows how powerful this command is. Here's a short video that demonstrates the above example:

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-3.webm"></video>
  <figcaption>Execute all the things</figcaption>
</figure>

If most (or all) of your subscriptions use the same credentials, you can set defaults in `client.yml`. Use `default-user` and `default-password` or `default-token` (but not both).
You can also specify a `default-command` that will run when a message is received. If a subscription does not include credentials to use or does not have a command, the defaults
will be used, otherwise, the subscription settings will override the defaults.

!!! warning
Because the `default-user`, `default-password`, and `default-token` will be sent for each topic that does not have its own username/password (even if the topic does not
require authentication), be sure that the servers/topics you subscribe to use HTTPS to prevent leaking the username and password.

### Using the systemd service

You can use the `ntfy-client` systemd service (see [ntfy-client.service](https://github.com/binwiederhier/ntfy/blob/main/client/ntfy-client.service))
to subscribe to multiple topics just like in the example above. The service is automatically installed (but not started)
if you install the deb/rpm package. To configure it, simply edit `/etc/ntfy/client.yml` and run `sudo systemctl restart ntfy-client`.

!!! info
The `ntfy-client.service` runs as user `ntfy`, meaning that typical Linux permission restrictions apply. See below
for how to fix this.

If the service runs on your personal desktop machine, you may want to override the service user/group (`User=` and `Group=`), and
adjust the `DISPLAY` and `DBUS_SESSION_BUS_ADDRESS` environment variables. This will allow you to run commands in your X session
as the primary machine user.

You can either manually override these systemd service entries with `sudo systemctl edit ntfy-client`, and add this
(assuming your user is `phil`). Don't forget to run `sudo systemctl daemon-reload` and `sudo systemctl restart ntfy-client`
after editing the service file:

=== "/etc/systemd/system/ntfy-client.service.d/override.conf"

    ```ini
    [Service]
    User=phil
    Group=phil
    Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"
    ```

Or you can run the following script that creates this override config for you:

```sh
sudo sh -c 'cat > /etc/systemd/system/ntfy-client.service.d/override.conf' <<EOF
[Service]
User=$USER
Group=$USER
Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$(id -u)/bus"
EOF

sudo systemctl daemon-reload
sudo systemctl restart ntfy-client
```

### Authentication

Depending on whether the server is configured to support [access control](../config.md#access-control), some topics
may be read/write protected so that only users with the correct credentials can subscribe or publish to them.
To publish/subscribe to protected topics, you can use [Basic Auth](https://en.wikipedia.org/wiki/Basic_access_authentication)
with a valid username/password. For your self-hosted server, **be sure to use HTTPS to avoid eavesdropping** and exposing
your password.

You can either add your username and password to the configuration file:
=== "~/.config/ntfy/client.yml"

    ```yaml
     - topic: secret
       command: 'notify-send "$m"'
       user: phill
       password: mypass
    ```

Or with the `ntfy subscibe` command:

```sh
ntfy subscribe \
  -u phil:mypass \
  ntfy.example.com/mysecrets
```
