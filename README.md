# ntfy

ntfy is a super simple pub-sub notification service. It allows you to send desktop and (soon) phone notifications
via scripts. I run a free version of it on *[ntfy.sh](https://ntfy.sh)*. No signups or cost.

## Usage

### Subscribe to a topic

You can subscribe to a topic either in a web UI, or in your own app by subscribing to an SSE/EventSource
or JSON feed. 

Here's how to do it via curl see the SSE stream in `curl`:

```
curl -s localhost:9997/mytopic/sse
```

You can easily script it to execute any command when a message arrives:
```
while read json; do 
  msg="$(echo "$json" | jq -r .message)"
  notify-send "$msg"
done < <(stdbuf -i0 -o0 curl -s localhost:9997/mytopic/json)
```

### Publish messages

Publishing messages can be done via PUT or POST using. Here's an example using `curl`:
```
curl -d "long process is done" ntfy.sh/mytopic
```

## TODO
- /raw endpoint
- netcat usage
- rate limiting / abuse protection
- release/packaging

## Contributing
I welcome any and all contributions. Just create a PR or an issue.

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io), distributed under the [Apache License 2.0](LICENSE).
