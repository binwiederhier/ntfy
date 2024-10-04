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
3. Or (optionally) create/edit `~/.config/ntfy/client.yml` (for the non-root user), `~/Library/Application Support/ntfy/client.yml` (for the macOS non-root user), or `/etc/ntfy/client.yml` (for the root user), see [sample client.yml](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml))

To run the ntfy server, then just run `ntfy serve` (or `systemctl start ntfy` when using the deb/rpm).
To send messages, use `ntfy publish`. To subscribe to topics, use `ntfy subscribe` (see [subscribing via CLI](subscribe/cli.md)
for details). 

If you like tutorials, check out :simple-youtube: [Kris Occhipinti's ntfy install guide](https://www.youtube.com/watch?v=bZzqrX05mNU) on YouTube, or
[Alex's Docker-based setup guide](https://blog.alexsguardian.net/posts/2023/09/12/selfhosting-ntfy/). Both are great
resources to get started. _I am not affiliated with Kris or Alex, I just liked their video/post._

## Linux binaries
Please check out the [releases page](https://github.com/binwiederhier/ntfy/releases) for binaries and
deb/rpm packages.

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_amd64.tar.gz
    tar zxvf ntfy_2.11.0_linux_amd64.tar.gz
    sudo cp -a ntfy_2.11.0_linux_amd64/ntfy /usr/local/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_2.11.0_linux_amd64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "armv5"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.3.1/ntfy_2.3.1_linux_armv5.tar.gz
    tar zxvf ntfy_2.3.1_linux_armv5.tar.gz
    sudo cp -a ntfy_2.3.1_linux_armv5/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_2.3.1_linux_armv5/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "armv6"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv6.tar.gz
    tar zxvf ntfy_2.11.0_linux_armv6.tar.gz
    sudo cp -a ntfy_2.11.0_linux_armv6/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_2.11.0_linux_armv6/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv7.tar.gz
    tar zxvf ntfy_2.11.0_linux_armv7.tar.gz
    sudo cp -a ntfy_2.11.0_linux_armv7/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_2.11.0_linux_armv7/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_arm64.tar.gz
    tar zxvf ntfy_2.11.0_linux_arm64.tar.gz
    sudo cp -a ntfy_2.11.0_linux_arm64/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_2.11.0_linux_arm64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
    ```

## Debian/Ubuntu repository
Installation via Debian repository:

=== "x86_64/amd64"
    ```bash
    sudo mkdir -p /etc/apt/keyrings
    curl -fsSL https://archive.heckel.io/apt/pubkey.txt | sudo gpg --dearmor -o /etc/apt/keyrings/archive.heckel.io.gpg
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=amd64 signed-by=/etc/apt/keyrings/archive.heckel.io.gpg] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv5/armel"
    ```bash
    sudo mkdir -p /etc/apt/keyrings
    curl -fsSL https://archive.heckel.io/apt/pubkey.txt | sudo gpg --dearmor -o /etc/apt/keyrings/archive.heckel.io.gpg
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=armel signed-by=/etc/apt/keyrings/archive.heckel.io.gpg] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    sudo mkdir -p /etc/apt/keyrings
    curl -fsSL https://archive.heckel.io/apt/pubkey.txt | sudo gpg --dearmor -o /etc/apt/keyrings/archive.heckel.io.gpg
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=armhf signed-by=/etc/apt/keyrings/archive.heckel.io.gpg] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    sudo mkdir -p /etc/apt/keyrings
    curl -fsSL https://archive.heckel.io/apt/pubkey.txt | sudo gpg --dearmor -o /etc/apt/keyrings/archive.heckel.io.gpg
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=arm64 signed-by=/etc/apt/keyrings/archive.heckel.io.gpg] https://archive.heckel.io/apt debian main' \
        > /etc/apt/sources.list.d/archive.heckel.io.list"
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

Manually installing the .deb file:

=== "x86_64/amd64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_amd64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv5"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.3.1/ntfy_2.3.1_linux_armv5.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy

=== "armv6"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv6.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv7.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_arm64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

## Fedora/RHEL/CentOS

=== "x86_64/amd64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_amd64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "armv5"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v2.3.1/ntfy_2.3.1_linux_armv5.rpm
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv6"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv6.rpm
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
    ```

=== "armv7/armhf"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_armv7.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

=== "arm64"
    ```bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_linux_arm64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
    ```

## Arch Linux
ntfy can be installed using an [AUR package](https://aur.archlinux.org/packages/ntfysh-bin/). 
You can use an [AUR helper](https://wiki.archlinux.org/title/AUR_helpers) like `paru`, `yay` or others to download, 
build and install ntfy and keep it up to date.
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

NixOS also supports [declarative setup of the ntfy server](https://search.nixos.org/options?channel=unstable&show=services.ntfy-sh.enable&from=0&size=50&sort=relevance&type=packages&query=ntfy). 

## macOS
The [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) is supported on macOS as well. 
To install, please [download the tarball](https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_darwin_all.tar.gz), 
extract it and place it somewhere in your `PATH` (e.g. `/usr/local/bin/ntfy`). 

If run as `root`, ntfy will look for its config at `/etc/ntfy/client.yml`. For all other users, it'll look for it at 
`~/Library/Application Support/ntfy/client.yml` (sample included in the tarball).

```bash
curl -L https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_darwin_all.tar.gz > ntfy_2.11.0_darwin_all.tar.gz
tar zxvf ntfy_2.11.0_darwin_all.tar.gz
sudo cp -a ntfy_2.11.0_darwin_all/ntfy /usr/local/bin/ntfy
mkdir ~/Library/Application\ Support/ntfy 
cp ntfy_2.11.0_darwin_all/client/client.yml ~/Library/Application\ Support/ntfy/client.yml
ntfy --help
```

!!! info
    Only the ntfy CLI is supported on macOS. ntfy server is currently not supported, but you can build and run it for 
    development as well. Check out the [build instructions](develop.md) for details.

## Homebrew
To install the [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) via Homebrew (Linux and macOS),
simply run:
```
brew install ntfy
```


## Windows
The [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) is supported on Windows as well.
To install, please [download the latest ZIP](https://github.com/binwiederhier/ntfy/releases/download/v2.11.0/ntfy_2.11.0_windows_amd64.zip),
extract it and place the `ntfy.exe` binary somewhere in your `%Path%`. 

The default path for the client config file is at `%AppData%\ntfy\client.yml` (not created automatically, sample in the ZIP file).

Also available in [Scoop's](https://scoop.sh) Main repository:

`scoop install ntfy`

!!! info
    There is currently no installer for Windows, and the binary is not signed. If this is desired, please create a
    [GitHub issue](https://github.com/binwiederhier/ntfy/issues) to let me know.

## Docker
The [ntfy image](https://hub.docker.com/r/binwiederhier/ntfy) is available for amd64, armv6, armv7 and arm64. It should 
be pretty straight forward to use.

The server exposes its web UI and the API on port 80, so you need to expose that in Docker. To use the persistent 
[message cache](config.md#message-cache), you also need to map a volume to `/var/cache/ntfy`. To change other settings, 
you should map `/etc/ntfy`, so you can edit `/etc/ntfy/server.yml`.

!!! info
    Note that the Docker image **does not contain a `/etc/ntfy/server.yml` file**. If you'd like to use a config file, 
    please manually create one outside the image and map it as a volume, e.g. via `-v /etc/ntfy:/etc/ntfy`. You may
    use the [`server.yml` file on GitHub](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml) as a template.

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
    serve \
    --cache-file /var/cache/ntfy/cache.db
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

Using docker-compose with non-root user and healthchecks enabled:
```yaml
version: "2.3"

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
    healthcheck: # optional: remember to adapt the host:port to your environment
        test: ["CMD-SHELL", "wget -q --tries=1 http://localhost:80/v1/health -O - | grep -Eo '\"healthy\"\\s*:\\s*true' || exit 1"]
        interval: 60s
        timeout: 10s
        retries: 3
        start_period: 40s
    restart: unless-stopped
```

If using a non-root user when running the docker version, be sure to chown the server.yml, user.db, and cache.db files and attachments directory to the same uid/gid.

Alternatively, you may wish to build a customized Docker image that can be run with fewer command-line arguments and without delivering the configuration file separately.
```
FROM binwiederhier/ntfy
COPY server.yml /etc/ntfy/server.yml
ENTRYPOINT ["ntfy", "serve"]
```
This image can be pushed to a container registry and shipped independently. All that's needed when running it is mapping ntfy's port to a host port.

## Kubernetes

The setup for Kubernetes is very similar to that for Docker, and requires a fairly minimal deployment or pod definition to function. There
are a few options to mix and match, including a deployment without a cache file, a stateful set with a persistent cache, and a standalone
unmanned pod.


=== "deployment"
    ```yaml
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: ntfy
    spec:
      selector:
        matchLabels:
          app: ntfy
      template:
        metadata:
          labels:
            app: ntfy
        spec:
          containers:
          - name: ntfy
            image: binwiederhier/ntfy
            args: ["serve"]
            resources:
              limits:
                memory: "128Mi"
                cpu: "500m"
            ports:
            - containerPort: 80
              name: http
            volumeMounts:
            - name: config
              mountPath: "/etc/ntfy"
              readOnly: true
          volumes:
            - name: config
              configMap:
                name: ntfy
    ---
    # Basic service for port 80
    apiVersion: v1
    kind: Service
    metadata:
      name: ntfy
    spec:
      selector:
        app: ntfy
      ports:
      - port: 80
        targetPort: 80
    ```

=== "stateful set"
    ```yaml
    apiVersion: apps/v1
    kind: StatefulSet
    metadata:
      name: ntfy
    spec:
      selector:
        matchLabels:
          app: ntfy
      serviceName: ntfy
      template:
        metadata:
          labels:
            app: ntfy
        spec:
          containers:
          - name: ntfy
            image: binwiederhier/ntfy
            args: ["serve", "--cache-file", "/var/cache/ntfy/cache.db"]
            ports:
            - containerPort: 80
              name: http
            volumeMounts:
            - name: config
              mountPath: "/etc/ntfy"
              readOnly: true
            - name: cache
              mountPath: "/var/cache/ntfy"
          volumes:
            - name: config
              configMap:
                name: ntfy
      volumeClaimTemplates:
      - metadata:
          name: cache
        spec:
          accessModes: [ "ReadWriteOnce" ]
          resources:
            requests:
              storage: 1Gi
    ```

=== "pod"
    ```yaml
    apiVersion: v1
    kind: Pod
    metadata:
      labels:
        app: ntfy
    spec:
      containers:
      - name: ntfy
        image: binwiederhier/ntfy
        args: ["serve"]
        resources:
          limits:
            memory: "128Mi"
            cpu: "500m"
        ports:
        - containerPort: 80
          name: http
        volumeMounts:
        - name: config
          mountPath: "/etc/ntfy"
          readOnly: true
      volumes:
        - name: config
          configMap:
            name: ntfy
    ```

Configuration is relatively straightforward. As an example, a minimal configuration is provided.

=== "resource definition"
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ntfy
    data:
      server.yml: |
        # Template: https://github.com/binwiederhier/ntfy/blob/main/server/server.yml
        base-url: https://ntfy.sh
    ```

=== "from-file"
    ```bash
    kubectl create configmap ntfy --from-file=server.yml 
    ```

## Kustomize

ntfy can be deployed in a Kubernetes cluster with [Kustomize](https://github.com/kubernetes-sigs/kustomize), a tool used
to customize Kubernetes objects using a `kustomization.yaml` file.

1. Create new folder - `ntfy`
2. Add all files listed below 
    1. `kustomization.yaml` - stores all configmaps and resources used in a deployment
    2. `ntfy-deployment.yaml` - define deployment type and its parameters
    3. `ntfy-pvc.yaml` - describes how [persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) will be created 
    4. `ntfy-svc.yaml` - expose application to the internal kubernetes network
    5. `ntfy-ingress.yaml` - expose service to outside the network using [ingress controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/)
    6. `server.yaml` - simple server configuration
3. Replace **TESTNAMESPACE** within `kustomization.yaml` with designated namespace 
4. Replace **ntfy.test** within `ntfy-ingress.yaml` with desired DNS name
5. Apply configuration to cluster set in current context: 

```bash
kubectl apply -k /ntfy
```

=== "kustomization.yaml"
    ```yaml
    apiVersion: kustomize.config.k8s.io/v1beta1
    kind: Kustomization
    resources:
      - ntfy-deployment.yaml # deployment definition
      - ntfy-svc.yaml # service connecting pods to cluster network
      - ntfy-pvc.yaml # pvc used to store cache and attachment
      - ntfy-ingress.yaml # ingress definition
    configMapGenerator: # will parse config from raw config to configmap,it allows for dynamic reload of application if additional app is deployed ie https://github.com/stakater/Reloader
        - name: server-config
          files: 
            - server.yml
    namespace: TESTNAMESPACE # select namespace for whole application 
    ```
=== "ntfy-deployment.yaml"
    ```yaml
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: ntfy-deployment
      labels:
        app: ntfy-deployment
    spec:
      revisionHistoryLimit: 1
      replicas: 1
      selector:
        matchLabels:
          app: ntfy-pod
      template:
        metadata:
          labels:
            app: ntfy-pod
        spec:
          containers:
            - name: ntfy 
              image: binwiederhier/ntfy:v1.28.0 # set deployed version
              args: ["serve"]
              env:  #example of adjustments made in environmental variables
                - name: TZ # set timezone
                  value: XXXXXXX
                - name: NTFY_DEBUG # enable/disable debug
                  value: "false"
                - name: NTFY_LOG_LEVEL # adjust log level
                  value: INFO
                - name: NTFY_BASE_URL # add base url
                  value: XXXXXXXXXX 
              ports: 
                - containerPort: 80
                  name: http-ntfy
              resources:
                limits:
                  memory: 300Mi
                  cpu:  200m
                requests:
                      cpu: 150m
                      memory: 150Mi
              volumeMounts:
                  - mountPath: /etc/ntfy/server.yml
                    subPath: server.yml
                    name: config-volume # generated vie configMapGenerator from kustomization file
                  - mountPath: /var/cache/ntfy
                    name: cache-volume #cache volume mounted to persistent volume
            volumes:
              - name: config-volume
                configMap:  # uses configmap generator to parse server.yml to configmap
                  name: server-config
              - name: cache-volume
                persistentVolumeClaim: # stores /cache/ntfy in defined pv
                  claimName: ntfy-pvc
    ```
  
=== "ntfy-pvc.yaml"
    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: ntfy-pvc
    spec:
      accessModes:
        - ReadWriteOnce
      storageClassName: local-path # adjust storage if needed
      resources:
        requests:
          storage: 1Gi
    ```

=== "ntfy-svc.yaml"
    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: ntfy-svc  
    spec:
      type: ClusterIP
      selector:
        app: ntfy-pod
      ports:
        - name: http-ntfy-out
          protocol: TCP
          port: 80
          targetPort:  http-ntfy
    ```

=== "ntfy-ingress.yaml"
    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: Ingress
    metadata:
      name: ntfy-ingress
    spec:
      rules:
        - host: ntfy.test #select own
          http:
            paths:
              - path: /
                pathType: Prefix
                backend:
                  service:
                    name:  ntfy-svc
                    port:
                      number: 80
    ```

=== "server.yml"
    ```yaml
    cache-file: "/var/cache/ntfy/cache.db"
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    ```
