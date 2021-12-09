# Install your own ntfy server
**Self-hosting your own ntfy server** is pretty straight forward. Just install the binary, package or Docker image, then 
configure it and run it. Just like any other software. No fuzz. 

!!! info
    The following steps are only required if you want to **self-host your own ntfy server**. If you just want to 
    [send messages using ntfy.sh](publish.md), you don't need to install anything.

## General steps
The ntfy server comes as a statically linked binary and is shipped as tarball, deb/rpm packages and as a Docker image.
We support amd64, armv7 and arm64.

1. Install ntfy using one of the methods described below
2. Then (optionally) edit `/etc/ntfy/config.yml` (see [configuration](config.md))
3. Then just run it with `ntfy` (or `systemctl start ntfy` when using the deb/rpm).

## Binaries and packages
Please check out the [releases page](https://github.com/binwiederhier/ntfy/releases) for binaries and
deb/rpm packages.

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_x86_64.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_armv7.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_arm64.tar.gz
    sudo tar -C /usr/bin -zxf ntfy_*.tar.gz ntfy
    sudo ./ntfy
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
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_amd64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_armv7.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_arm64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

## Fedora/RHEL/CentOS

=== "x86_64/amd64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_amd64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_armv7.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.5.2/ntfy_1.5.2_linux_arm64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

## Docker
The [ntfy image](https://hub.docker.com/r/binwiederhier/ntfy) is available for amd64, armv7 and arm64. It should be pretty
straight forward to use.

The server exposes its web UI and the API on port 80, so you need to expose that in Docker. To use the persistent 
[message cache](config.md#message-cache), you also need to map a volume to `/var/cache/ntfy`. To change other settings, you should map `/etc/ntfy`,
so you can edit `/etc/ntfy/config.yml`.

Basic usage (no cache or additional config):
```
docker run -p 80:80 -it binwiederhier/ntfy
```

With persistent cache (configured as command line arguments):
```bash
docker run \
  -v /var/cache/ntfy:/var/cache/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy \
    --cache-file /var/cache/ntfy/cache.db
```

With other config options (configured via `/etc/ntfy/config.yml`, see [configuration](config.md) for details):
```bash
docker run \
  -v /etc/ntfy:/etc/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy
```

## Go
To install via Go, simply run:
```bash
go install heckel.io/ntfy@latest
```

!!! info
    Please [let me know](https://github.com/binwiederhier/ntfy/issues) if there are any issues with this installation
    method. The SQLite bindings require CGO and it works for me, but I have the feeling it may not work for everyone.
