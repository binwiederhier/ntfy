# ntfy

ntfy (pronounce: *notify*) is a super simple pub-sub notification service. It allows you to send desktop and (soon) phone notifications
via scripts. I run a free version of it on *[ntfy.sh](https://ntfy.sh)*. **No signups or cost.**

## Usage

### Subscribe to a topic
You can subscribe to a topic either in a web UI, or in your own app by subscribing to an 
[SSE](https://en.wikipedia.org/wiki/Server-sent_events)/[EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource),
or a JSON or raw feed.  

Here's how to see the raw/json/sse stream in `curl`. This will subscribe to the topic and wait for events.

```
# Subscribe to "mytopic" and output one message per line (\n are replaced with a space)
curl -s ntfy.sh/mytopic/raw

# Subscribe to "mytopic" and output one JSON message per line
curl -s ntfy.sh/mytopic/json

# Subscribe to "mytopic" and output an SSE stream (supported via JS/EventSource)
curl -s ntfy.sh/mytopic/sse
```

You can easily script it to execute any command when a message arrives. This sends desktop notifications (just like 
the web UI, but without it):
```
while read msg; do 
  notify-send "$msg"
done < <(stdbuf -i0 -o0 curl -s ntfy.sh/mytopic/raw)
```

### Publish messages
Publishing messages can be done via PUT or POST using. Here's an example using `curl`:
```
curl -d "long process is done" ntfy.sh/mytopic
```

Messages published to a non-existing topic or a topic without subscribers will not be delivered later. There is (currently)
no buffering of any kind. If you're not listening, the message won't be delivered.

## FAQ

### Isn't this like ...?
Probably. I didn't do a whole lot of research before making this.

### Can I use this in my app?
Yes. As long as you don't abuse it, it'll be available and free of charge.

### What are the uptime guarantees?
Best effort.

### Why is the web UI so ugly?
I don't particularly like JS or dealing with CSS. I'll make it pretty after it's functional.

## TODO
- rate limiting / abuse protection
- release/packaging
- add HTTPS

## Contributing
I welcome any and all contributions. Just create a PR or an issue.

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io), distributed under the [Apache License 2.0](LICENSE).
