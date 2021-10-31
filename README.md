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
  [ -n "$msg" ] && notify-send "$msg"
done < <(stdbuf -i0 -o0 curl -s ntfy.sh/mytopic/raw)
```

### Publish messages
Publishing messages can be done via PUT or POST using. Here's an example using `curl`:
```
curl -d "long process is done" ntfy.sh/mytopic
```

Messages published to a non-existing topic or a topic without subscribers will not be delivered later. There is (currently)
no buffering of any kind. If you're not listening, the message won't be delivered.

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
sudo apt install tmux
wget https://github.com/binwiederhier/ntfy/releases/download/v1.1.2/ntfy_1.1.2_amd64.deb
dpkg -i ntfy_1.1.2_amd64.deb
```

**Fedora/RHEL/CentOS:**
```bash
rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.1.2/ntfy_1.1.2_amd64.rpm
```

**Docker:**
```bash
docker run --rm -it binwiederhier/ntfy
```

**Go:**
```bash
go get -u heckel.io/ntfy
```

**Manual install** (*any x86_64-based Linux*)**:**
```bash
wget https://github.com/binwiederhier/ntfy/releases/download/v1.1.2/ntfy_1.1.2_linux_x86_64.tar.gz
sudo tar -C /usr/bin -zxf ntfy_1.1.2_linux_x86_64.tar.gz ntfy
./ntfy
```

## Building
Building ntfy is simple. Here's how you do it:

```
make build-simple
# Builds to dist/ntfy_linux_amd64/ntfy
``` 

To build releases, I use [GoReleaser](https://goreleaser.com/). If you have that installed, you can run `make build` or
`make build-snapshot`.

## TODO
- add HTTPS
- make limits configurable
- limit max number of subscriptions 

## Contributing
I welcome any and all contributions. Just create a PR or an issue.

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io), distributed under the [Apache License 2.0](LICENSE).

Third party libraries and resources:
* [github.com/urfave/cli/v2](https://github.com/urfave/cli/v2) (MIT) is used to drive the CLI
* [Mixkit sound](https://mixkit.co/free-sound-effects/notification/) (Mixkit Free License) used as notification sound
* [Lato Font](https://www.latofonts.com/) (OFL) is used as a font in the Web UI
* [GoReleaser](https://goreleaser.com/) (MIT) is used to create releases 
