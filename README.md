![ntfy](server/static/img/ntfy.png)

# ntfy.sh | simple HTTP-based pub-sub

**ntfy** (pronounce: *notify*) is a simple HTTP-based [pub-sub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) notification service.
It allows you to **send notifications to your phone or desktop via scripts** from any computer, entirely **without signup or cost**.
It's also open source (as you can plainly see) if you want to run your own.

I run a free version of it at **[ntfy.sh](https://ntfy.sh)**, and there's an [Android app](https://play.google.com/store/apps/details?id=io.heckel.ntfy)
too.

<p>
  <img src="server/static/img/screenshot-curl.png" height="180">
  <img src="server/static/img/screenshot-web-detail.png" height="180">
  <img src="server/static/img/screenshot-phone-main.jpg" height="180">
  <img src="server/static/img/screenshot-phone-detail.jpg" height="180">
  <img src="server/static/img/screenshot-phone-notification.jpg" height="180">
</p>

## Usage

### Publishing messages

Publishing messages can be done via PUT or POST using. Topics are created on the fly by subscribing or publishing to them.
Because there is no sign-up, **the topic is essentially a password**, so pick something that's not easily guessable.

Here's an example showing how to publish a message using `curl`:

```
curl -d "long process is done" ntfy.sh/mytopic
```

Here's an example in JS with `fetch()` (see [full example](examples)):

```
fetch('https://ntfy.sh/mytopic', {
  method: 'POST', // PUT works too
  body: 'Hello from the other side.'
})
```

### Subscribe to a topic
You can create and subscribe to a topic either in this web UI, or in your own app by subscribing to an
[EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource), a JSON feed, or raw feed.

#### Subscribe via web
If you subscribe to a topic via this web UI in the field below, messages published to any subscribed topic
will show up as **desktop notification**.

You can try this easily on **[ntfy.sh](https://ntfy.sh)**.

#### Subscribe via phone
You can use the [Ntfy Android App](https://play.google.com/store/apps/details?id=io.heckel.ntfy) to receive 
notifications directly on your phone. Just like the server, this app is also [open source](https://github.com/binwiederhier/ntfy-android).

#### Subscribe via your app, or via the CLI
Using [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) in JS, you can consume
notifications like this (see [full example](examples)):

```javascript
const eventSource = new EventSource('https://ntfy.sh/mytopic/sse');<br/>
eventSource.onmessage = (e) => {<br/>
  // Do something with e.data<br/>
};
```

You can also use the same `/sse` endpoint via `curl` or any other HTTP library:

```
$ curl -s ntfy.sh/mytopic/sse
event: open
data: {"id":"weSj9RtNkj","time":1635528898,"event":"open","topic":"mytopic"}

data: {"id":"p0M5y6gcCY","time":1635528909,"event":"message","topic":"mytopic","message":"Hi!"}

event: keepalive
data: {"id":"VNxNIg5fpt","time":1635528928,"event":"keepalive","topic":"test"}
```

To consume JSON instead, use the `/json` endpoint, which prints one message per line:

```
$ curl -s ntfy.sh/mytopic/json
{"id":"SLiKI64DOt","time":1635528757,"event":"open","topic":"mytopic"}
{"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Hi!"}
{"id":"DGUDShMCsc","time":1635528787,"event":"keepalive","topic":"mytopic"}
```

Or use the `/raw` endpoint if you need something super simple (empty lines are keepalive messages):

```
$ curl -s ntfy.sh/mytopic/raw

This is a notification
```

#### Message buffering and polling
Messages are buffered in memory for a few hours to account for network interruptions of subscribers.
You can read back what you missed by using the `since=...` query parameter. It takes either a
duration (e.g. `10m` or `30s`) or a Unix timestamp (e.g. `1635528757`):

```
$ curl -s "ntfy.sh/mytopic/json?since=10m"
# Same output as above, but includes messages from up to 10 minutes ago
```

You can also just poll for messages if you don't like the long-standing connection using the `poll=1`
query parameter. The connection will end after all available messages have been read. This parameter has to be
combined with `since=`.

```
$ curl -s "ntfy.sh/mytopic/json?poll=1&since=10m"
# Returns messages from up to 10 minutes ago and ends the connection
```

## Examples
There are a few usage examples in the [examples](examples) directory. I'm sure there are tons of other ways to use it.

## Installation
Please check out the [releases page](https://github.com/binwiederhier/ntfy/releases) for binaries and
deb/rpm packages.

1. Install ntfy using one of the methods described below
2. Then (optionally) edit `/etc/ntfy/config.yml`
3. Then just run it with `ntfy` (or `systemctl start ntfy` when using the deb/rpm).

### Binaries and packages
**Debian/Ubuntu** (*from a repository*)**:**
```bash
curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
sudo apt install apt-transport-https
sudo sh -c "echo 'deb [arch=amd64] https://archive.heckel.io/apt debian main' > /etc/apt/sources.list.d/archive.heckel.io.list"  
sudo apt update
sudo apt install ntfy
```

**Debian/Ubuntu** (*manual install*)**:**
```bash
wget https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_amd64.deb
dpkg -i ntfy_1.4.6_amd64.deb
```

**Fedora/RHEL/CentOS:**
```bash
rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_amd64.rpm
```

**Docker:**
Without cache:
```
docker run -p 80:80 -it binwiederhier/ntfy
```

With cache:
```bash
docker run \
  -v /var/cache/ntfy:/var/cache/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy \
    --cache-file /var/cache/ntfy/cache.db
```

**Go:**
```bash
go get -u heckel.io/ntfy
```

**Manual install:**
```bash
# x86_64/amd64
wget https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_linux_x86_64.tar.gz

# ARMv6
wget https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_linux_armv6.tar.gz

# ARMv7
wget https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_linux_armv7.tar.gz

# arm64
wget https://github.com/binwiederhier/ntfy/releases/download/v1.4.6/ntfy_1.4.6_linux_arm64.tar.gz

# Extract and run
sudo tar -C /usr/bin -zxf ntfy_1.4.6_linux_x86_64.tar.gz ntfy
./ntfy
```

## Building
Building `ntfy` is simple. Here's how you do it:

```
make build-simple
# Builds to dist/ntfy_linux_amd64/ntfy
``` 

To build releases, I use [GoReleaser](https://goreleaser.com/). If you have that installed, you can run `make build` or
`make build-snapshot`.

## Contributing
I welcome any and all contributions. Just create a PR or an issue.

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io).   
The project is dual licensed under the [Apache License 2.0](LICENSE) and the [GPLv2 License](LICENSE.GPLv2).

Third party libraries and resources:
* [github.com/urfave/cli/v2](https://github.com/urfave/cli/v2) (MIT) is used to drive the CLI
* [Mixkit sound](https://mixkit.co/free-sound-effects/notification/) (Mixkit Free License) used as notification sound
* [Lato Font](https://www.latofonts.com/) (OFL) is used as a font in the Web UI
* [GoReleaser](https://goreleaser.com/) (MIT) is used to create releases
* [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) (MIT) is used to provide the persistent message cache
* [Firebase Admin SDK](https://github.com/firebase/firebase-admin-go) (Apache 2.0) is used to send FCM messages
* [Lightbox with vanilla JS](https://yossiabramov.com/blog/vanilla-js-lightbox) 
* [Statically linking go-sqlite3](https://www.arp242.net/static-go.html)
