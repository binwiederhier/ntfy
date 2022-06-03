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
2. Then (optionally) edit `/etc/ntfy/server.yml` for the server (Linux only, see [configuration](config.md) or [sample server.yml](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml))
3. Or (optionally) create/edit `~/.config/ntfy/client.yml` (or `/etc/ntfy/client.yml`, see [sample client.yml](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml))

To run the ntfy server, then just run `ntfy serve` (or `systemctl start ntfy` when using the deb/rpm).
To send messages, use `ntfy publish`. To subscribe to topics, use `ntfy subscribe` (see [subscribing via CLI][subscribe/cli.md]
for details). 

## Linux binaries
Please check out the [releases page](https://github.com/binwiederhier/ntfy/releases) for binaries and
deb/rpm packages.

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_x86_64.tar.gz
    tar zxvf ntfy_1.25.1_linux_x86_64.tar.gz
    sudo cp -a ntfy_1.25.1_linux_x86_64/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.25.1_linux_x86_64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "armv6"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv6.tar.gz
    tar zxvf ntfy_1.25.1_linux_armv6.tar.gz
    sudo cp -a ntfy_1.25.1_linux_armv6/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.25.1_linux_armv6/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv7.tar.gz
    tar zxvf ntfy_1.25.1_linux_armv7.tar.gz
    sudo cp -a ntfy_1.25.1_linux_armv7/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.25.1_linux_armv7/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_arm64.tar.gz
    tar zxvf ntfy_1.25.1_linux_arm64.tar.gz
    sudo cp -a ntfy_1.25.1_linux_arm64/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.25.1_linux_arm64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
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
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_amd64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv6"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv6.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv7.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_arm64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

## Fedora/RHEL/CentOS

=== "x86_64/amd64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_amd64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "armv6"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv6.rpm
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_armv7.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_1.25.1_linux_arm64.rpm
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

## NixOS / Nix
ntfy is packaged in nixpkgs as `ntfy-sh`. It can be installed by adding the package name to the configuration file and calling `nixos-rebuild`. Alternatively, the following command can be used to install ntfy in the current user environment:
```
nix-env -iA ntfy-sh
```

## macOS
The [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) is supported on macOS as well. 
To install, please download the tarball, extract it and place it somewhere in your `PATH` (e.g. `/usr/local/bin/ntfy`). 

If run as `root`, ntfy will look for its config at `/etc/ntfy/client.yml`. For all other users, it'll look for it at 
`~/Library/Application Support/ntfy/client.yml` (sample included in the tarball).

```bash
curl https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_v1.25.1_macOS_all.tar.gz > ntfy_v1.25.1_macOS_all.tar.gz
tar zxvf ntfy_v1.25.1_macOS_all.tar.gz
sudo cp -a ntfy_v1.25.1_macOS_all/ntfy /usr/local/bin/ntfy
mkdir ~/Library/Application\ Support/ntfy 
cp ntfy_v1.25.1_macOS_all/client/client.yml ~/Library/Application\ Support/ntfy/client.yml
ntfy --help
```

!!! info
    If there is a desire to install ntfy via [Homebrew](https://brew.sh/), please create a 
    [GitHub issue](https://github.com/binwiederhier/ntfy/issues) to let me know. Also, you can build and run the
    ntfy server on macOS as well, though I don't officially support that. Check out the [build instructions](develop.md)
    for details.

## Windows
The [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) is supported on Windows as well.
To install, please [download the latest ZIP](https://github.com/binwiederhier/ntfy/releases/download/v1.25.1/ntfy_v1.25.1_windows_x86_64.zip),
extract it and place the `ntfy.exe` binary somewhere in your `%Path%`. 

The default path for the client config file is at `%AppData%\ntfy\client.yml` (not created automatically, sample in the ZIP file).

!!! info
    There is currently no installer for Windows, and the binary is not signed. If this is desired, please create a
    [GitHub issue](https://github.com/binwiederhier/ntfy/issues) to let me know.

## Docker
The [ntfy image](https://hub.docker.com/r/binwiederhier/ntfy) is available for amd64, armv6, armv7 and arm64. It should 
be pretty straight forward to use.

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

With other config options, timezone, and non-root user (configured via `/etc/ntfy/server.yml`, see [configuration](config.md) for details):
```bash
docker run \
  -v /etc/ntfy:/etc/ntfy \
  -e TZ=UTC \
  -p 80:80 \
  -u UID:GID \
  -it \
  binwiederhier/ntfy \
  serve
```

Using docker-compose with non-root user:
```yaml
version: "2.1"

services:
  ntfy:
    image: binwiederhier/ntfy
    container_name: ntfy
    command:
      - serve
    environment:
      - TZ=UTC    # optional: set desired timezone
    user: UID:GID # optional: replace with your own user/group or uid/gid
    volumes:
      - /var/cache/ntfy:/var/cache/ntfy
      - /etc/ntfy:/etc/ntfy
    ports:
      - 80:80
    restart: unless-stopped
```

If using a non-root user when running the docker version, be sure to chown the server.yml, user.db, and cache.db files to the same uid/gid.

Alternatively, you may wish to build a customized Docker image that can be run with fewer command-line arguments and without delivering the configuration file separately.
```
FROM binwiederhier/ntfy
COPY server.yml /etc/ntfy/server.yml
ENTRYPOINT ["ntfy", "serve"]
```
This image can be pushed to a container registry and shipped independently. All that's needed when running it is mapping ntfy's port to a host port.
