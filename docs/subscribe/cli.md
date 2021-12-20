# Subscribe via ntfy CLI
In addition to subscribing via the [web UI](web.md), the [phone app](phone.md), or the [API](api.md), you can subscribe
to topics via the ntfy CLI. The CLI is included in the same `ntfy` binary that can be used to [self-host a server](../install.md).

!!! info
    The **ntfy CLI is not required to send or receive messages**. You can instead [send messages with curl](../publish.md),
    and even use it to [subscribe to topics](api.md). It may be a little more convenient to use the ntfy CLI than writing 
    your own script. Or it may not be. It all depends on the use case. 😀

## Install + configure
To install the ntfy CLI, simply follow the steps outlined on the [install page](../install.md). The ntfy server and 
client are the same binary, so it's all very convenient. After installing, you can (optionally) configure the client 
by creating `~/.config/ntfy/client.yml` (for the non-root user), or `/etc/ntfy/client.yml` (for the root user). You 
can find a [skeleton config](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml) on GitHub. 

If you just want to use [ntfy.sh](https://ntfy.sh), you don't have to change anything. If you **self-host your own server**,
you may want to edit the `default-host` option:

``` yaml
# Base URL used to expand short topic names in the "ntfy publish" and "ntfy subscribe" commands.
# If you self-host a ntfy server, you'll likely want to change this.
#
default-host: https://ntfy.myhost.com
```

## Publish using the ntfy CLI
You can send messages with the ntfy CLI using the `ntfy publish` command (or any of its aliases `pub`, `send` or 
`trigger`). There are a lot of examples on the page about [publishing messages](../publish.md), but here are a few
quick ones:

=== "Simple send"
    ```
    ntfy publish mytopic This is a message
    ntfy publish mytopic "This is a message"
    ntfy pub mytopic "This is a message" 
    ```

=== "Send with title, priority, and tags"
    ```
    ntfy publish \
        --title="Thing sold on eBay" \
        --priority=high \
        --tags=partying_face \
        mytopic \
        "Somebody just bought the thing that you sell"
    ```

=== "Send at 8:30am"
    ```
    ntfy pub --at=8:30am delayed_topic Laterzz
    ```

=== "Triggering a webhook"
    ```
    ntfy trigger mywebhook
    ntfy pub mywebhook
    ```

## Subscribe using the ntfy CLI
You can subscribe to topics using `ntfy subscribe`. Depending on how it is called, this command
will either print or execute a command for every arriving message. There are a few different ways 
in which the command can be run:

### Stream messages and print JSON
If run like this `ntfy subscribe TOPIC`, the command prints the JSON representation of every incoming 
message. This is useful when you have a command that wants to stream-read incoming JSON messages. 
Unless `--poll` is passed, this command stays open forever.

```
$ ntfy sub mytopic
{"id":"nZ8PjH5oox","time":1639971913,"event":"message","topic":"mytopic","message":"hi there"}
{"id":"sekSLWTujn","time":1639972063,"event":"message","topic":"mytopic","tags":["warning","skull"],"message":"Oh no, something went wrong"}
```

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-1.mp4"></video>
  <figcaption>Subscribe in JSON mode</figcaption>
</figure>

### Execute a command for every incoming message
If run like this `ntfy subscribe TOPIC COMMAND`, a COMMAND is executed for every incoming messages. 
The message fields are passed to the command as environment variables and can be used in scripts:

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-2.webm"></video>
  <figcaption>Execute command on incoming messages</figcaption>
</figure>

| Variable | Aliases | Description |
|---|---|---
| `$NTFY_ID` | `$id` | Unique message ID |
| `$NTFY_TIME` | `$time` | Unix timestamp of the message delivery |
| `$NTFY_TOPIC` | `$topic` | Topic name |
| `$NTFY_MESSAGE` | `$message`, `$m` | Message body |
| `$NTFY_TITLE` | `$title`, `$t` | Message title |
| `$NTFY_PRIORITY` | `$priority`, `$p` | Message priority (1=min, 5=max) |
| `$NTFY_TAGS` | `$tags`, `$ta` | Message tags (comma separated list) |
   
     Examples:
       ntfy sub mytopic 'notify-send "$m"'    # Execute command for incoming messages
       ntfy sub topic1 /my/script.sh          # Execute script for incoming messages

### Using a config file
ntfy subscribe --from-config
Service mode (used in ntfy-client.service). This reads the config file (/etc/ntfy/client.yml
or ~/.config/ntfy/client.yml) and sets up subscriptions for every topic in the "subscribe:"
block (see config file).

     Examples: 
       ntfy sub --from-config                           # Read topics from config file
       ntfy sub --config=/my/client.yml --from-config   # Read topics from alternate config file

The default config file for all client commands is /etc/ntfy/client.yml (if root user),
or ~/.config/ntfy/client.yml for all other users.


### Using the systemd service

```
[Service]
User=pheckel
Group=pheckel
Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"
```

Here's an example for a complete client config for a self-hosted server:

