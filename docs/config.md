# Configuring the ntfy server
The ntfy server can be configured in three ways: using a config file (typically at `/etc/ntfy/config.yml`, 
see [config.yml](https://github.com/binwiederhier/ntfy/blob/main/config/config.yml)), via command line arguments 
or using environment variables.

## Quick start
By default, simply running `ntfy` will start the server at port 80. No configuration needed. Batteries included 😀. 
If everything works as it should, you'll see something like this:
```
$ ntfy                
2021/11/30 19:59:08 Listening on :80
```

You can immediately start [publishing messages](publish.md), or subscribe via the [Android app](subscribe/phone.md),
[the web UI](subscribe/web.md), or simply via [curl or your favorite HTTP client](subscribe/api.md). To configure 
the server further, check out the [config options table](#config-options) or simply type `ntfy --help` to
get a list of [command line options](#command-line-options).

## Message cache
If desired, ntfy can temporarily keep notifications in an in-memory or an on-disk cache. Caching messages for a short period
of time is important to allow [phones](subscribe/phone.md) and other devices with brittle Internet connections to be able to retrieve
notifications that they may have missed. 

By default, ntfy keeps messages in memory for 12 hours, which means that **cached messages do not survive an application
restart**. You can override this behavior using the following config settings:

* `cache-file`: if set, ntfy will store messages in a SQLite based cache (default is empty, which means in-memory cache).
  **This is required if you'd like messages to be retained across restarts**.
* `cache-duration`: defines the duration for which messages are stored in the cache (default is `12h`)

Subscribers can retrieve cached messaging using the [`poll=1` parameter](subscribe/api.md#polling), as well as the
[`since=` parameter](subscribe/api.md#since).

## Behind a proxy (TLS, etc.)

!!! warning
    If you are running ntfy behind a proxy, you must set the `behind-proxy` flag. Otherwise all visitors are rate limited
    as if they are one.

**Rate limiting:** If you are running ntfy behind a proxy (e.g. nginx, HAproxy or Apache), you should set the `behind-proxy` 
flag. This will instruct the [rate limiting](#rate-limiting) logic to use the `X-Forwarded-For` header as the primary 
identifier for a visitor, as opposed to the remote IP address. If the `behind-proxy` flag is not set, all visitors will
be counted as one, because from the perspective of the ntfy server, they all share the proxy's IP address.

**TLS/SSL:** ntfy supports HTTPS/TLS by setting the `listen-https` config option. However, if you are behind a proxy, it is
recommended that TLS/SSL termination is done by the proxy itself.

## Firebase (FCM)
!!! info
    Using Firebase is **optional** and only works if you modify and build your own Android .apk.
    For a self-hosted instance, it's easier to just not bother with FCM.

[Firebase Cloud Messaging (FCM)](https://firebase.google.com/docs/cloud-messaging) is the Google approved way to send
push messages to Android devices. FCM is the only method that an Android app can receive messages without having to run a
[foreground service](https://developer.android.com/guide/components/foreground-services).

For the main host [ntfy.sh](https://ntfy.sh), the [ntfy Android App](subscribe/phone.md) uses Firebase to send messages
to the device. For other hosts, instant delivery is used and FCM is not involved.

To configure FCM for your self-hosted instance of the ntfy server, follow these steps:

1. Sign up for a [Firebase account](https://console.firebase.google.com/)
2. Create an app and download the key file (e.g. `myapp-firebase-adminsdk-ahnce-....json`)
3. Place the key file in `/etc/ntfy`, set the `firebase-key-file` in `config.yml` accordingly and restart the ntfy server
4. Build your own Android .apk following [these instructions]()

Example:
```
# If set, also publish messages to a Firebase Cloud Messaging (FCM) topic for your app.
# This is optional and only required to support Android apps (which don't allow background services anymore).
#
firebase-key-file: "/etc/ntfy/ntfy-sh-firebase-adminsdk-ahnce-9f4d6f14b5.json"
```

## Rate limiting
!!! info
    Be aware that if you are running ntfy behind a proxy, you must set the `behind-proxy` flag. 
    Otherwise all visitors are rate limited as if they are one.

By default, ntfy runs without authentication, so it is vitally important that we protect the server from abuse or overload.
There are various limits and rate limits in place that you can use to configure the server. Let's do the easy ones first:

* `global-topic-limit` defines the total number of topics before the server rejects new topics. It defaults to 5000.
* `visitor-subscription-limit` is the number of subscriptions (open connections) per visitor. This value defaults to 30.

A **visitor** is identified by its IP address (or the `X-Forwarded-For` header if `behind-proxy` is set). All config 
options that start with the word `visitor` apply only on a per-visitor basis.   

In addition to the limits above, there is a requests/second limit per visitor for all sensitive GET/PUT/POST requests.
This limit uses a [token bucket](https://en.wikipedia.org/wiki/Token_bucket) (using Go's [rate package](https://pkg.go.dev/golang.org/x/time/rate)):

Each visitor has a bucket of 60 requests they can fire against the server (defined by `visitor-request-limit-burst`). 
After the 60, new requests will encounter a `429 Too Many Requests` response. The visitor request bucket is refilled at a rate of one
request every 10s (defined by `visitor-request-limit-replenish`)

* `visitor-request-limit-burst` is the initial bucket of requests each visitor has. This defaults to 60.
* `visitor-request-limit-replenish` is the rate at which the bucket is refilled (one request per x). Defaults to 10s.

During normal usage, you shouldn't encounter this limit at all, and even if you burst a few requests shortly (e.g. when you 
reconnect after a connection drop), it shouldn't have any effect.

## Config options
Each config options can be set in the config file `/etc/ntfy/config.yml` (e.g. `listen-http: :80`) or as a
CLI option (e.g. `--listen-http :80`. Here's a list of all available options. Alternatively, you can set an environment
variable before running the `ntfy` command (e.g. `export NTFY_LISTEN_HTTP=:80`).

| Config option | Env variable | Format | Default | Description |
|---|---|---|---|---|
| `listen-http` | `NTFY_LISTEN_HTTP` | `[host]:port` | `:80` | Listen address for the HTTP web server |
| `listen-https` | `NTFY_LISTEN_HTTPS` | `[host]:port` | - | Listen address for the HTTPS web server. If set, you also need to set `key-file` and `cert-file`. |
| `key-file` | `NTFY_KEY_FILE` | *filename* | - | HTTPS/TLS private key file, only used if `listen-https` is set. |
| `cert-file` | `NTFY_CERT_FILE` | *filename* | - | HTTPS/TLS certificate file, only used if `listen-https` is set. |
| `firebase-key-file` | `NTFY_FIREBASE_KEY_FILE` | *filename* | - | If set, also publish messages to a Firebase Cloud Messaging (FCM) topic for your app. This is optional and only required to save battery when using the Android app. |
| `cache-file` | `NTFY_CACHE_FILE` | *filename* | - | If set, messages are cached in a local SQLite database instead of only in-memory. This allows for service restarts without losing messages in support of the since= parameter. |
| `cache-duration` | `NTFY_CACHE_DURATION` | *duration* | 12h | Duration for which messages will be buffered before they are deleted. This is required to support the `since=...` and `poll=1` parameter. |
| `keepalive-interval` | `NTFY_KEEPALIVE_INTERVAL` | *duration* | 30s | Interval in which keepalive messages are sent to the client. This is to prevent intermediaries closing the connection for inactivity. Note that the Android app has a hardcoded timeout at 77s, so it should be less than that. |
| `manager-interval` | `$NTFY_MANAGER_INTERVAL` | *duration* | 1m | Interval in which the manager prunes old messages, deletes topics and prints the stats. |
| `global-topic-limit` | `NTFY_GLOBAL_TOPIC_LIMIT` | *number* | 5000 | Rate limiting: Total number of topics before the server rejects new topics. |
| `visitor-subscription-limit` | `NTFY_VISITOR_SUBSCRIPTION_LIMIT` | *number* | 30 | Rate limiting: Number of subscriptions per visitor (IP address) |
| `visitor-request-limit-burst` | `NTFY_VISITOR_REQUEST_LIMIT_BURST` | *number* | 60 | Allowed GET/PUT/POST requests per second, per visitor. This setting is the initial bucket of requests each visitor has |
| `visitor-request-limit-replenish` | `NTFY_VISITOR_REQUEST_LIMIT_REPLENISH` | *duration* | 10s | Strongly related to `visitor-request-limit-burst`: The rate at which the bucket is refilled |
| `behind-proxy` | `NTFY_BEHIND_PROXY` | *bool* | false | If set, the X-Forwarded-For header is used to determine the visitor IP address instead of the remote address of the connection. |

The format for a *duration* is: `<number>(smh)`, e.g. 30s, 20m or 1h.

## Command line options
```
$ ntfy --help
NAME:
   ntfy - Simple pub-sub notification service

USAGE:
   ntfy [OPTION..]

GLOBAL OPTIONS:
   --config value, -c value                           config file (default: /etc/ntfy/config.yml) [$NTFY_CONFIG_FILE]
   --listen-http value, -l value                      ip:port used to as listen address (default: ":80") [$NTFY_LISTEN_HTTP]
   --firebase-key-file value, -F value                Firebase credentials file; if set additionally publish to FCM topic [$NTFY_FIREBASE_KEY_FILE]
   --cache-file value, -C value                       cache file used for message caching [$NTFY_CACHE_FILE]
   --cache-duration since, -b since                   buffer messages for this time to allow since requests (default: 12h0m0s) [$NTFY_CACHE_DURATION]
   --keepalive-interval value, -k value               interval of keepalive messages (default: 30s) [$NTFY_KEEPALIVE_INTERVAL]
   --manager-interval value, -m value                 interval of for message pruning and stats printing (default: 1m0s) [$NTFY_MANAGER_INTERVAL]
   --global-topic-limit value, -T value               total number of topics allowed (default: 5000) [$NTFY_GLOBAL_TOPIC_LIMIT]
   --visitor-subscription-limit value, -V value       number of subscriptions per visitor (default: 30) [$NTFY_VISITOR_SUBSCRIPTION_LIMIT]
   --visitor-request-limit-burst value, -B value      initial limit of requests per visitor (default: 60) [$NTFY_VISITOR_REQUEST_LIMIT_BURST]
   --visitor-request-limit-replenish value, -R value  interval at which burst limit is replenished (one per x) (default: 10s) [$NTFY_VISITOR_REQUEST_LIMIT_REPLENISH]
   --behind-proxy, -P                                 if set, use X-Forwarded-For header to determine visitor IP address (for rate limiting) (default: false) [$NTFY_BEHIND_PROXY]

Try 'ntfy COMMAND --help' for more information.

ntfy v1.4.8 (7b8185c), runtime go1.17, built at 1637872539
Copyright (C) 2021 Philipp C. Heckel, distributed under the Apache License 2.0
```

