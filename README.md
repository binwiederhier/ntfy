# ntfy

ntfy (pronounce: *notify*) is a super simple pub-sub notification service. It allows you to send desktop and (soon) phone notifications
via scripts. I run a free version of it on *[ntfy.sh](https://ntfy.sh)*. **No signups or cost.**

## Usage

### Subscribe to a topic
Topics are created on the fly by subscribing to them. You can create and subscribe to a topic either in a web UI, or in 
your own app by subscribing to an [SSE](https://en.wikipedia.org/wiki/Server-sent_events)/[EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource),
or a JSON or raw feed.  

Because there is no sign-up, **the topic is essentially a password**, so pick something that's not easily guessable.  

Here's how you can create a topic `mytopic`, subscribe to it topic and wait for events. This is using `curl`, but you
can use any library that can do HTTP GETs:

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

## Will you know what topics exist, can you spy on me?
If you don't trust me or your messages are sensitive, run your ntfy on your own server. That said, the logs do not 
contain any topic names

## TODO
- add HTTPS

## Contributing
I welcome any and all contributions. Just create a PR or an issue.

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io), distributed under the [Apache License 2.0](LICENSE).
