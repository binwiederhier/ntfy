# Desarrollo

¡Hurra 🥳 🎉, estás interesado en escribir código para ntfy! **Eso es increíble.** 😎

Hice todo lo posible para escribir instrucciones detalladas, pero si en algún momento te encuentras con problemas, no lo hagas.
dudar en **contáctame en [Discordia](https://discord.gg/cT7ECsZj9w) o [Matriz](https://matrix.to/#/#ntfy:matrix.org)**.

## Servidor ntfy

El código fuente del servidor ntfy está disponible [en GitHub](https://github.com/binwiederhier/ntfy). La base de código para el
el servidor consta de tres componentes:

*   **El servidor/cliente principal** está escrito en [Ir](https://go.dev/) (así que necesitarás Go). Su punto de entrada principal está en
    [main.go](https://github.com/binwiederhier/ntfy/blob/main/main.go), y la carne que probablemente te interese es
    en [servidor.go](https://github.com/binwiederhier/ntfy/blob/main/server/server.go). En particular, el servidor utiliza un
    [SQLite](https://sqlite.org) biblioteca llamada [go-sqlite3](https://github.com/mattn/go-sqlite3), que requiere
    [Cgo](https://go.dev/blog/cgo) y `CGO_ENABLED=1` para ser establecido. De lo contrario, las cosas no funcionarán (ver más abajo).
*   **La documentación** es generado por [MkDocs](https://www.mkdocs.org/) y [Material para MkDocs](https://squidfunk.github.io/mkdocs-material/),
    que está escrito en [Pitón](https://www.python.org/). Necesitarás Python y MkDocs (a través de `pip`) sólo si desea
    compila los documentos.
*   **La aplicación web** está escrito en [Reaccionar](https://reactjs.org/)Usando [MUI](https://mui.com/). Utiliza [Crear la aplicación React](https://create-react-app.dev/)
    para construir la compilación de producción. Si desea modificar la aplicación web, necesita [nodejs](https://nodejs.org/en/) (para `npm`)
    e instalar todas las 100.000 dependencias (*suspirar*).

Todos estos componentes se construyen y luego **horneado en un binario**.

### Navegación por el código

Código:

*   [main.go](https://github.com/binwiederhier/ntfy/blob/main/main.go) - Punto de entrada principal en la CLI, tanto para el servidor como para el cliente
*   [cmd/](https://github.com/binwiederhier/ntfy/tree/main/cmd) - Comandos de la CLI, como `serve` o `publish`
*   [servidor/](https://github.com/binwiederhier/ntfy/tree/main/server) - La carne de la lógica del servidor
*   [documentos/](https://github.com/binwiederhier/ntfy/tree/main/docs) - El [MkDocs](https://www.mkdocs.org/) documentación, consulte también `mkdocs.yml`
*   [web/](https://github.com/binwiederhier/ntfy/tree/main/web) - El [Reaccionar](https://reactjs.org/) aplicación, ver también `web/package.json`

Construcción relacionada:

*   [Makefile](https://github.com/binwiederhier/ntfy/blob/main/Makefile) - Punto de entrada principal para todo lo relacionado con la construcción
*   [.goreleaser.yml](https://github.com/binwiederhier/ntfy/blob/main/.goreleaser.yml) - Describe todos los resultados de compilación (para [GoReleaser](https://goreleaser.com/))
*   [go.mod](https://github.com/binwiederhier/ntfy/blob/main/go.mod) - Archivo de dependencia de módulos Go
*   [mkdocs.yml](https://github.com/binwiederhier/ntfy/blob/main/mkdocs.yml) - Archivo de configuración para los documentos (para [MkDocs](https://www.mkdocs.org/))
*   [web/paquete.json](https://github.com/binwiederhier/ntfy/blob/main/web/package.json) - Compilación y archivo de dependencia para la aplicación web (para npm)

El `web/` y `docs/` son los orígenes de la aplicación web y la documentación. Durante el proceso de construcción,
El resultado generado se copia en `server/site` (aplicación web y página de destino) y `server/docs` (documentación).

### Requisitos de compilación

*   [Ir](https://go.dev/) (requerido para el servidor principal)
*   [Gcc](https://gcc.gnu.org/) (servidor principal requerido, para enlaces basados en SQLite cgo)
*   [Hacer](https://www.gnu.org/software/make/) (requerido por conveniencia)
*   [libsqlite3/libsqlite3-dev](https://www.sqlite.org/) (requerido para el servidor principal, para enlaces basados en SQLite cgo)
*   [GoReleaser](https://goreleaser.com/) (requerido para una compilación adecuada del servidor principal)
*   [Pitón](https://www.python.org/) (para `pip`, sólo para compilar los documentos)
*   [nodejs](https://nodejs.org/en/) (para `npm`, solo para compilar la aplicación web)

### Instalar dependencias

Estos pasos **asumir Ubuntu**. Los pasos pueden variar en diferentes distribuciones de Linux.

Primero, instale [Ir](https://go.dev/) (ver [instrucciones oficiales](https://go.dev/doc/install)):

```shell
wget https://go.dev/dl/go1.18.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.18.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
go version   # verifies that it worked
```

Instalar [GoReleaser](https://goreleaser.com/) (ver [instrucciones oficiales](https://goreleaser.com/install/)):

```shell
go install github.com/goreleaser/goreleaser@latest
goreleaser -v   # verifies that it worked
```

Instalar [nodejs](https://nodejs.org/en/) (ver [instrucciones oficiales](https://nodejs.org/en/download/package-manager/)):

```shell
curl -fsSL https://deb.nodesource.com/setup_17.x | sudo -E bash -
sudo apt-get install -y nodejs
npm -v   # verifies that it worked
```

A continuación, instale algunas otras cosas necesarias:

```shell
sudo apt install \
    build-essential \
    libsqlite3-dev \
    gcc-arm-linux-gnueabi \
    gcc-aarch64-linux-gnu \
    python3-pip \
    upx \
    git
```

### Comprobar código

Ahora echa un vistazo a través de git desde el [Repositorio de GitHub](https://github.com/binwiederhier/ntfy):

\=== "vía HTTPS"
` shell
    git clone https://github.com/binwiederhier/ntfy.git
    cd ntfy
    `

\=== "vía SSH"
` shell
    git clone git@github.com:binwiederhier/ntfy.git 
    cd ntfy
    `

### Construye todas las cosas

Ahora finalmente puedes construir todo. Hay toneladas de `make` objetivos, así que tal vez solo revise lo que hay primero
escribiendo `make`:

```shell
$ make 
Typical commands (more see below):
  make build                   - Build web app, documentation and server/client (sloowwww)
  make cli-linux-amd64         - Build server/client binary (amd64, no web app or docs)
  make install-linux-amd64     - Install ntfy binary to /usr/bin/ntfy (amd64)
  make web                     - Build the web app
  make docs                    - Build the documentation
  make check                   - Run all tests, vetting/formatting checks and linters
...
```

Si quieres construir el **Ntfy binario, incluida la aplicación web y los documentos para todas las arquitecturas compatibles** (amd64, armv7 y arm64),
simplemente puede ejecutar `make build`:

```shell
$ make build
...
# This builds web app, docs, and the ntfy binary (for amd64, armv7 and arm64). 
# This will be SLOW (5+ minutes on my laptop on the first run). Maybe look at the other make targets?
```

Verás todas las salidas en el `dist/` carpeta después:

```bash
$ find dist 
dist
dist/metadata.json
dist/ntfy_arm64_linux_arm64
dist/ntfy_arm64_linux_arm64/ntfy
dist/ntfy_armv7_linux_arm_7
dist/ntfy_armv7_linux_arm_7/ntfy
dist/ntfy_amd64_linux_amd64
dist/ntfy_amd64_linux_amd64/ntfy
dist/config.yaml
dist/artifacts.json
```

Si también desea construir el **Paquetes Debian/RPM e imágenes de Docker para todas las arquitecturas soportadas**Puedes
utilice el botón `make release-snapshot` blanco:

```shell
$ make release-snapshot
...
# This will be REALLY SLOW (sometimes 5+ minutes on my laptop)
```

Durante el desarrollo, es posible que desee ser más exigente y construir solo ciertas cosas. Aquí hay algunos ejemplos.

### Construir el binario ntfy

Para construir sólo el `ntfy` binario **sin la aplicación web o la documentación**, utilice el botón `make cli-...` Objetivos:

```shell
$ make
Build server & client (using GoReleaser, not release version):
  make cli                        - Build server & client (all architectures)
  make cli-linux-amd64            - Build server & client (Linux, amd64 only)
  make cli-linux-armv6            - Build server & client (Linux, armv6 only)
  make cli-linux-armv7            - Build server & client (Linux, armv7 only)
  make cli-linux-arm64            - Build server & client (Linux, arm64 only)
  make cli-windows-amd64          - Build client (Windows, amd64 only)
  make cli-darwin-all             - Build client (macOS, arm64+amd64 universal binary)
```

Entonces, si está en una máquina basada en amd64 / x86\_64, es posible que solo desee ejecutar `make cli-linux-amd64` durante las pruebas. En un moderno
sistema, esto no debería tomar más de 5-10 segundos. A menudo lo combino con `install-linux-amd64` para que pueda ejecutar el binario
Ahora mismo:

```shell
$ make cli-linux-amd64 install-linux-amd64
$ ntfy serve
```

**Durante el desarrollo de la aplicación principal, también puede usar `go run main.go`**, siempre y cuando corras
`make cli-deps-static-sites`al menos una vez y `CGO_ENABLED=1`:

```shell
$ export CGO_ENABLED=1
$ make cli-deps-static-sites
$ go run main.go serve
2022/03/18 08:43:55 Listening on :2586[http]
...
```

Si no corres `cli-deps-static-sites`, es posible que vea un error *`pattern ...: no matching files found`*:

    $ go run main.go serve
    server/server.go:85:13: pattern docs: no matching files found

Esto se debe a que usamos `go:embed` para incrustar la documentación y la aplicación web, por lo que el código Go espera que los archivos sean
presente en `server/docs` y `server/site`. Si no lo son, verá el error anterior. El `cli-deps-static-sites`
target crea archivos ficticios que garantizan que podrá compilar.

Aunque no es oficialmente compatible (o lanzado), puede compilar y ejecutar el servidor **en macOS** También. Simplemente ejecute
`make cli-darwin-server` para crear un binario, o `go run main.go serve` (ver arriba) para ejecutarlo.

### Compilar la aplicación web

Los orígenes de la aplicación web viven en `web/`. Siempre y cuando tengas `npm` instalado (consulte más arriba), creación de la aplicación web
es realmente simple. Simplemente escriba `make web` y estás en el negocio:

```shell
$ make web
...
```

Esto creará la aplicación web mediante Crear aplicación React y luego **Copie la compilación de producción en el cuadro `server/site` carpeta**así que
que cuando `make cli` (o `make cli-linux-amd64`, ...), tendrás la web app incluida en el `ntfy` binario.

Si está desarrollando en la aplicación web, es mejor simplemente `cd web` y ejecutar `npm start` manualmente. Esto abrirá su navegador
en `http://127.0.0.1:3000` con la aplicación web, y a medida que edite los archivos de origen, se volverán a compilar y el explorador
se actualizará automáticamente:

```shell
$ cd web
$ npm start
```

### Crear los documentos

Las fuentes de los documentos viven en `docs/`. De manera similar a la aplicación web, simplemente puede ejecutar `make docs` para crear el
documentación. Siempre y cuando tengas `mkdocs` instalado (ver arriba), esto debería funcionar bien:

```shell
$ make docs
...
```

Si va a cambiar la documentación, debería estar ejecutando `mkdocs serve` directamente. Esto construirá la documentación,
Servir los archivos en `http://127.0.0.1:8000/`y vuelva a generar cada vez que guarde los archivos de origen:

    $ mkdocs serve
    INFO     -  Building documentation...
    INFO     -  Cleaning site directory
    INFO     -  Documentation built in 5.53 seconds
    INFO     -  [16:28:14] Serving on http://127.0.0.1:8000/

Luego puede navegar a http://127.0.0.1:8000/ y cada vez que cambie un archivo de marcado en su editor de texto, se actualizará automáticamente.

## Aplicación para Android

El código fuente de la aplicación ntfy para Android está disponible [en GitHub](https://github.com/binwiederhier/ntfy-android).
La aplicación de Android tiene dos sabores:

*   **Google Play:** El `play` sabor incluye [Base de fuego (FCM)](https://firebase.google.com/) y requiere una cuenta de Firebase
*   **F-Droide:** El `fdroid` Flavor no incluye dependencias de Firebase o Google

### Navegación por el código

*   [principal/](https://github.com/binwiederhier/ntfy-android/tree/main/app/src/main) - Código fuente principal de la aplicación Android
*   [jugar/](https://github.com/binwiederhier/ntfy-android/tree/main/app/src/play) - Código específico de Google Play / Firebase
*   [fdroid/](https://github.com/binwiederhier/ntfy-android/tree/main/app/src/fdroid) - Talones de F-Droid Firebase
*   [build.gradle](https://github.com/binwiederhier/ntfy-android/blob/main/app/build.gradle) - Archivo de compilación principal

### IDE/Medio ambiente

Deberías descargar [Estudio Android](https://developer.android.com/studio) (o [IntelliJ IDEA](https://www.jetbrains.com/idea/)
con los plugins de Android pertinentes). Todo lo demás será un dolor para ti. Hazte un favor. 😀

### Echa un vistazo al código

Primero echa un vistazo al repositorio:

\=== "vía HTTPS"
` shell
    git clone https://github.com/binwiederhier/ntfy-android.git
    cd ntfy-android
    `

\=== "vía SSH"
` shell
    git clone git@github.com:binwiederhier/ntfy-android.git
    cd ntfy-android
    `

A continuación, sigue los pasos para construir con o sin Firebase.

### Construir sabor F-Droid (sin FCM)

!!! información
Construyo la aplicación ntfy para Android usando IntelliJ IDEA (Android Studio), así que no sé si estos comandos de Gradle lo harán.
trabajar sin problemas. Por favor, dame tu opinión si funciona o no funciona para ti.

Sin Firebase, es posible que desees seguir cambiando el valor predeterminado `app_base_url` en [valores.xml](https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/res/values/values.xml)
si está autohospedando el servidor. Luego corre:

    # Remove Google dependencies (FCM)
    sed -i -e '/google-services/d' build.gradle
    sed -i -e '/google-services/d' app/build.gradle

    # To build an unsigned .apk (app/build/outputs/apk/fdroid/*.apk)
    ./gradlew assembleFdroidRelease

    # To build a bundle .aab (app/fdroid/release/*.aab)
    ./gradlew bundleFdroidRelease

### Sabor Build Play (FCM)

!!! información
Construyo la aplicación ntfy para Android usando IntelliJ IDEA (Android Studio), así que no sé si estos comandos de Gradle lo harán.
trabajar sin problemas. Por favor, dame tu opinión si funciona o no funciona para ti.

Para crear tu propia versión con Firebase, debes:

*   Crear una cuenta de Firebase/FCM
*   Coloque el archivo de su cuenta en `app/google-services.json`
*   Y cambio `app_base_url` en [valores.xml](https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/res/values/values.xml)
*   Luego corre:

<!---->

    # To build an unsigned .apk (app/build/outputs/apk/play/*.apk)
    ./gradlew assemblePlayRelease

    # To build a bundle .aab (app/play/release/*.aab)
    ./gradlew bundlePlayRelease

## Aplicación iOS

El código fuente de la aplicación ntfy iOS está disponible [en GitHub](https://github.com/binwiederhier/ntfy-ios).

!!! información
No he tenido tiempo de mover las instrucciones de compilación aquí. Por favor, echa un vistazo al repositorio en su lugar.
