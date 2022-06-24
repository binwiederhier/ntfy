# Instalación de ntfy

El `ntfy` CLI le permite [publicar mensajes](publish.md), [suscribirse a temas](subscribe/cli.md) así como a
autohospede su propio servidor ntfy. Todo es bastante sencillo. Simplemente instale el binario, el paquete o la imagen de Docker,
configurarlo y ejecutarlo. Al igual que cualquier otro software. Sin pelusa.

!!! información
Los siguientes pasos solo son necesarios si desea **Autohospede su propio servidor NTFY o desea utilizar la CLI de Ntfy**.
Si solo quieres [enviar mensajes mediante ntfy.sh](publish.md), no necesita instalar nada. Puedes usar
`curl`.

## Pasos generales

El servidor ntfy viene como un binario vinculado estáticamente y se envía como paquetes tarball, deb / rpm y como una imagen docker.
Admitimos amd64, armv7 y arm64.

1.  Instale ntfy mediante uno de los métodos que se describen a continuación
2.  Luego (opcionalmente) editar `/etc/ntfy/server.yml` para el servidor (sólo Linux, consulte [configuración](config.md) o [ejemplo server.yml](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml))
3.  O (opcionalmente) crear/editar `~/.config/ntfy/client.yml` (o `/etc/ntfy/client.yml`ver [ejemplo client.yml](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml))

Para ejecutar el servidor ntfy, simplemente ejecute `ntfy serve` (o `systemctl start ntfy` cuando se utiliza el deb/rpm).
Para enviar mensajes, utilice `ntfy publish`. Para suscribirse a temas, utilice `ntfy subscribe` (consulte \[suscripción a través de CLI]\[subscribe/cli.md]
para más detalles).

## Binarios de Linux

Por favor, echa un vistazo a la [página de lanzamientos](https://github.com/binwiederhier/ntfy/releases) para binarios y
paquetes deb/rpm.

\=== "x86\_64/amd64"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_x86_64.tar.gz
    tar zxvf ntfy_1.27.2_linux_x86_64.tar.gz
    sudo cp -a ntfy_1.27.2_linux_x86_64/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.27.2_linux_x86_64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
     `

\=== "armv6"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv6.tar.gz
    tar zxvf ntfy_1.27.2_linux_armv6.tar.gz
    sudo cp -a ntfy_1.27.2_linux_armv6/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.27.2_linux_armv6/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
     `

\=== "armv7/armhf"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv7.tar.gz
    tar zxvf ntfy_1.27.2_linux_armv7.tar.gz
    sudo cp -a ntfy_1.27.2_linux_armv7/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.27.2_linux_armv7/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
     `

\=== "arm64"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_arm64.tar.gz
    tar zxvf ntfy_1.27.2_linux_arm64.tar.gz
    sudo cp -a ntfy_1.27.2_linux_arm64/ntfy /usr/bin/ntfy
    sudo mkdir /etc/ntfy && sudo cp ntfy_1.27.2_linux_arm64/{client,server}/*.yml /etc/ntfy
    sudo ntfy serve
     `

## Repositorio Debian/Ubuntu

Instalación a través del repositorio de Debian:

\=== "x86\_64/amd64"
` bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=amd64] https://archive.heckel.io/apt debian main' \         > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "armv7/armhf"
` bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=armhf] https://archive.heckel.io/apt debian main' \         > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "arm64"
` bash
    curl -sSL https://archive.heckel.io/apt/pubkey.txt | sudo apt-key add -
    sudo apt install apt-transport-https
    sudo sh -c "echo 'deb [arch=arm64] https://archive.heckel.io/apt debian main' \         > /etc/apt/sources.list.d/archive.heckel.io.list"  
    sudo apt update
    sudo apt install ntfy
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

Instalación manual del archivo .deb:

\=== "x86\_64/amd64"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_amd64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "armv6"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv6.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "armv7/armhf"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv7.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "arm64"
` bash
    wget https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_arm64.deb
    sudo dpkg -i ntfy_*.deb
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

## Fedora/RHEL/CentOS

\=== "x86\_64/amd64"
` bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_amd64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
     `

\=== "armv6"
` bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv6.rpm
    sudo systemctl enable ntfy
    sudo systemctl start ntfy
     `

\=== "armv7/armhf"
` bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_armv7.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
     `

\=== "arm64"
` bash
    sudo rpm -ivh https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_linux_arm64.rpm
    sudo systemctl enable ntfy 
    sudo systemctl start ntfy
     `

## Arch Linux

ntfy se puede instalar mediante un [Paquete AUR](https://aur.archlinux.org/packages/ntfysh-bin/). Puede utilizar un [Ayudante de AUR](https://wiki.archlinux.org/title/AUR_helpers) gustar `paru`, `yay` u otros para descargar, construir e instalar ntfy y mantenerlo actualizado.

    paru -S ntfysh-bin

Como alternativa, ejecute los siguientes comandos para instalar ntfy manualmente:

    curl https://aur.archlinux.org/cgit/aur.git/snapshot/ntfysh-bin.tar.gz | tar xzv
    cd ntfysh-bin
    makepkg -si

## NixOS / Nix

ntfy está empaquetado en nixpkgs como `ntfy-sh`. Se puede instalar agregando el nombre del paquete al archivo de configuración y llamando `nixos-rebuild`. Como alternativa, se puede utilizar el siguiente comando para instalar ntfy en el entorno de usuario actual:

    nix-env -iA ntfy-sh

## macOS

El [CLI ntfy](subscribe/cli.md) (`ntfy publish` y `ntfy subscribe` only) también es compatible con macOS.
Para instalar, por favor [descargar el tarball](https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy\_1.27.2\_macOS_all.tar.gz),
extráelo y colóquelo en algún lugar de su `PATH` (por ejemplo, `/usr/local/bin/ntfy`).

Si se ejecuta como `root`, ntfy buscará su configuración en `/etc/ntfy/client.yml`. Para todos los demás usuarios, lo buscará en
`~/Library/Application Support/ntfy/client.yml` (muestra incluida en el tarball).

```bash
curl -L https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy_1.27.2_macOS_all.tar.gz > ntfy_1.27.2_macOS_all.tar.gz
tar zxvf ntfy_1.27.2_macOS_all.tar.gz
sudo cp -a ntfy_1.27.2_macOS_all/ntfy /usr/local/bin/ntfy
mkdir ~/Library/Application\ Support/ntfy 
cp ntfy_1.27.2_macOS_all/client/client.yml ~/Library/Application\ Support/ntfy/client.yml
ntfy --help
```

!!! información
Hay un [Problema de GitHub](https://github.com/binwiederhier/ntfy/issues/286) acerca de cómo hacer que ntfy sea instalable a través de
[Homebrew](https://brew.sh/). Eventualmente llegaré a eso, pero también me encantaría que alguien más se acercara para hacerlo.
Además, también puede compilar y ejecutar el servidor ntfy en macOS, aunque oficialmente no lo admito.
Echa un vistazo a la [instrucciones de compilación](develop.md) para más detalles.

## Windows

El [CLI ntfy](subscribe/cli.md) (`ntfy publish` y `ntfy subscribe` only) también es compatible con Windows.
Para instalar, por favor [descargue el último zip](https://github.com/binwiederhier/ntfy/releases/download/v1.27.2/ntfy\_1.27.2\_windows_x86\_64.zip),
extraerlo y colocar el `ntfy.exe` binario en algún lugar de su `%Path%`.

La ruta predeterminada para el archivo de configuración del cliente es en `%AppData%\ntfy\client.yml` (no creado automáticamente, muestra en el archivo ZIP).

También disponible en [Scoop's](https://scoop.sh) Repositorio principal:

`scoop install ntfy`

!!! información
Actualmente no hay ningún instalador para Windows y el binario no está firmado. Si esto se desea, por favor cree un
[Problema de GitHub](https://github.com/binwiederhier/ntfy/issues) para hacérmelo saber.

## Estibador

El [Imagen ntfy](https://hub.docker.com/r/binwiederhier/ntfy) está disponible para amd64, armv6, armv7 y arm64. Debería
ser bastante sencillo de usar.

El servidor expone su interfaz de usuario web y la API en el puerto 80, por lo que debe exponerlo en Docker. Para utilizar la persistencia
[caché de mensajes](config.md#message-cache), también debe asignar un volumen a `/var/cache/ntfy`. Para cambiar otros ajustes,
debe mapear `/etc/ntfy`, para que pueda editar `/etc/ntfy/server.yml`.

Uso básico (sin caché ni configuración adicional):

    docker run -p 80:80 -it binwiederhier/ntfy serve

Con caché persistente (configurada como argumentos de línea de comandos):

```bash
docker run \
  -v /var/cache/ntfy:/var/cache/ntfy \
  -p 80:80 \
  -it \
  binwiederhier/ntfy \
    serve \
    --cache-file /var/cache/ntfy/cache.db
```

Con otras opciones de configuración, zona horaria y usuario no root (configurado a través de `/etc/ntfy/server.yml`ver [configuración](config.md) para más detalles):

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

Uso de docker-compose con un usuario no root:

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

Si utiliza un usuario no root al ejecutar la versión de Docker, asegúrese de chown los archivos server.yml, user.db y cache.db en el mismo uid/gid.

Como alternativa, es posible que desee crear una imagen de Docker personalizada que se pueda ejecutar con menos argumentos de línea de comandos y sin entregar el archivo de configuración por separado.

    FROM binwiederhier/ntfy
    COPY server.yml /etc/ntfy/server.yml
    ENTRYPOINT ["ntfy", "serve"]

Esta imagen se puede enviar a un registro de contenedores y enviarse de forma independiente. Todo lo que se necesita al ejecutarlo es asignar el puerto de ntfy a un puerto host.
