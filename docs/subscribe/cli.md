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

## Sending messages
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

=== "Triggering a webhook"
    ```
    ntfy trigger mywebhook
    ntfy pub mywebhook
    ```

## Using the systemd service

```
[Service]
User=pheckel
Group=pheckel
Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"
```

Here's an example for a complete client config for a self-hosted server:

