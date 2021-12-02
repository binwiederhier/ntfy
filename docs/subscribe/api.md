# Subscribe via API
    
## Fetching cached messages
Messages may be cached for a couple of hours (see [message caching](../config.md#message-cache)) to account for network
interruptions of subscribers. If the server has configured message caching, you can read back what you missed by using 
the `since=` query parameter. It takes either a duration (e.g. `10m` or `30s`), a Unix timestamp (e.g. `1635528757`) 
or `all` (all cached messages).

```
curl -s "ntfy.sh/mytopic/json?since=10m"
```

## Polling
You can also just poll for messages if you don't like the long-standing connection using the `poll=1`
query parameter. The connection will end after all available messages have been read. This parameter can be
combined with `since=` (defaults to `since=all`).

```
curl -s "ntfy.sh/mytopic/json?poll=1"
```

## Subscribing to multiple topics
It's possible to subscribe to multiple topics in one HTTP call by providing a
comma-separated list of topics in the URL. This allows you to reduce the number of connections you have to maintain:

```
$ curl -s ntfy.sh/mytopic1,mytopic2/json
{"id":"0OkXIryH3H","time":1637182619,"event":"open","topic":"mytopic1,mytopic2,mytopic3"}
{"id":"dzJJm7BCWs","time":1637182634,"event":"message","topic":"mytopic1","message":"for topic 1"}
{"id":"Cm02DsxUHb","time":1637182643,"event":"message","topic":"mytopic2","message":"for topic 2"}
```
