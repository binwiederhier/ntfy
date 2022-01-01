# Installing ntfy
The `ntfy` CLI allows you to [publish messages](publish.md), [subscribe to topics](subscribe/cli.md) as well as to
self-host your own ntfy server. It's all pretty straight forward. Just install the binary, package or Docker image, 
configure it and run it. Just like any other software. No fuzz. 

!!! info
    The following steps are only required if you want to **self-host your own ntfy server or you want to use the ntfy CLI**.
    If you just want to [send messages using ntfy.sh](publish.md), you don't need to install anything. You can just use
    `curl`.

## General steps
The ntfy server comes as a statically linked binary and is shipped as tarball, deb/rpm packages and as a Docker image.
We support amd64, armv7 and arm64.

1. Install ntfy using one of the methods described below
2. Then (optionally) edit `/etc/ntfy/server.yml` for the server (see [configuration](config.md) or [sample server.yml](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml))
3. Or (optionally) create/edit `~/.config/ntfy/client.yml` (or `/etc/ntfy/client.yml`, see [sample client.yml](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml))

To run the ntfy server, then just run `ntfy serve` (or `systemctl start ntfy` when using the deb/rpm).
To send messages, use `ntfy publish`. To subscribe to topics, use `ntfy subscribe` (see [subscribing via CLI][subscribe/cli.md]
for details). 

## Binaries and packages
Please check out the [releases page](https://github.com/binwiederhier/ntfy/releases) for binaries and
deb/rpm packages.

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_x86_64.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy serve
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_armv7.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy serve
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_arm64.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy serve
    ```

## Debian/Ubuntu repository
Installation via Debian repository:

=== "x86_64/amd64"
    ```bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=amd64] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=armhf] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=arm64] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

Manually installing the .deb file:

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_amd64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_armv7.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_arm64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

## Fedora/RHEL/CentOS

=== "x86_64/amd64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_amd64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_armv7.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.11.0/ntfy_1.11.0_linux_arm64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

## Arch Linux
ntfy can be installed using an [AUR package](https://aur.archlinux.org/packages/ntfysh-bin/). You can use an [AUR helper](https://wiki.archlinux.org/title/AUR_helpers) like `paru`, `yay` or others to download, build and install ntfy and keep it up to date.
```
paru -S ntfysh-bin
```

Alternatively, run the following commands to install ntfy manually:
```
curl https://aur.archlinux.org/cgit/aur.git/snapshot/ntfysh-bin.tar.gz | tar xzv
cd ntfysh-bin
makepkg -si
```


## Docker
The [ntfy image](https://hub.docker.com/r/binwiederhier/ntfy) is available for amd64, armv7 and arm64. It should be pretty
straight forward to use.

The server exposes its web UI and the API on port 80, so you need to expose that in Docker. To use the persistent 
[message cache](config.md#message-cache), you also need to map a volume to `/var/cache/ntfy`. To change other settings, 
you should map `/etc/ntfy`, so you can edit `/etc/ntfy/server.yml`.

Basic usage (no cache or additional config):
```
docker run -p 80:80 -it binwiederhier/ntfy serve
```

With persistent cache (configured as command line arguments):
```bash
docker run \
  -v /var/cache/ntfy:/var/cache/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy \
    --cache-file /var/cache/ntfy/cache.db \
    serve
```

With other config options (configured via `/etc/ntfy/server.yml`, see [configuration](config.md) for details):
```bash
docker run \
  -v /etc/ntfy:/etc/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy \
  serve
```

Alternatively, you may wish to build a customized Docker image that can be run with fewer command-line arguments and without delivering the configuration file separately.
```
FROM binwiederhier/ntfy
COPY server.yml /etc/ntfy/server.yml
ENTRYPOINT ["ntfy", "serve"]
```
This image can be pushed to a container registry and shipped independently. All that's needed when running it is mapping ntfy's port to a host port.

## Go
To install via Go, simply run:
```bash
go install heckel.io/ntfy@latest
```

!!! info
    Please [let me know](https://github.com/binwiederhier/ntfy/issues) if there are any issues with this installation
    method. The SQLite bindings require CGO and it works for me, but I have the feeling it may not work for everyone.
