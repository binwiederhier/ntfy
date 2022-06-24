# Suscríbete a través de API

Puede crear y suscribirse a un tema en el [interfaz de usuario web](web.md), a través del [aplicación de teléfono](phone.md), a través del [CLI ntfy](cli.md),
o en su propia aplicación o script suscribiéndose a la API. Esta página describe cómo suscribirse a través de api. También es posible que desee
Consulte la página que describe cómo [publicar mensajes](../publish.md).

Puede consumir la API de suscripción como un **[flujo HTTP simple (JSON, SSE o raw)](#http-stream)**o
**[a través de WebSockets](#websockets)**. Ambos son increíblemente simples de usar.

## Flujo HTTP

La API basada en secuencias HTTP se basa en una simple solicitud GET con una respuesta HTTP de transmisión, es decir, **abre una solicitud GET y
La conexión permanece abierta para siempre**, enviando mensajes a medida que entran. Hay tres puntos de enlace de API diferentes, que
sólo difieren en el formato de respuesta:

*   [Flujo JSON](#subscribe-as-json-stream): `<topic>/json` devuelve una secuencia JSON, con un objeto de mensaje JSON por línea
*   [Flujo de ESS](#subscribe-as-sse-stream): `<topic>/sse` devuelve mensajes como [Eventos enviados por el servidor (SSE)](https://en.wikipedia.org/wiki/Server-sent_events)cuál
    se puede utilizar con [Fuente de eventos](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
*   [Flujo raw](#subscribe-as-raw-stream): `<topic>/raw` devuelve mensajes como texto sin formato, con una línea por mensaje

### Suscríbete como transmisión JSON

A continuación se muestran algunos ejemplos de cómo consumir el extremo JSON (`<topic>/json`). Para casi todos los idiomas, **este es el
forma recomendada de suscribirse a un tema**. La excepción notable es JavaScript, para el cual el
[Flujo SSE/EventSource](#subscribe-as-sse-stream) es mucho más fácil trabajar con él.

\=== "Línea de comandos (curl)"
`     $ curl -s ntfy.sh/disk-alerts/json
    {"id":"SLiKI64DOt","time":1635528757,"event":"open","topic":"mytopic"}
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    {"id":"DGUDShMCsc","time":1635528787,"event":"keepalive","topic":"mytopic"}
    ...
    `

\=== "ntfy CLI"
`     $ ntfy subcribe disk-alerts
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    ...
    `

\=== "HTTP"
''' http
GET /disk-alerts/json HTTP/1.1
Anfitrión: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: application/x-ndjson; charset=utf-8
    Transfer-Encoding: chunked

    {"id":"SLiKI64DOt","time":1635528757,"event":"open","topic":"mytopic"}
    {"id":"hwQ2YpKdmg","time":1635528741,"event":"message","topic":"mytopic","message":"Disk full"}
    {"id":"DGUDShMCsc","time":1635528787,"event":"keepalive","topic":"mytopic"}
    ...
    ```

\=== "Go"
` go
    resp, err := http.Get("https://ntfy.sh/disk-alerts/json")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        println(scanner.Text())
    }
    `

\=== "Python"
` python
    resp = requests.get("https://ntfy.sh/disk-alerts/json", stream=True)
    for line in resp.iter_lines():
      if line:
        print(line)
    `

\=== "PHP"
` php-inline
    $fp = fopen('https://ntfy.sh/disk-alerts/json', 'r');
    if (!$fp) die('cannot open stream');
    while (!feof($fp)) {
        echo fgets($fp, 2048);
        flush();
    }
    fclose($fp);
    `

### Suscríbete como SSE stream

Usando [Fuente de eventos](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) en JavaScript, puede consumir
notificaciones a través de un [Eventos enviados por el servidor (SSE)](https://en.wikipedia.org/wiki/Server-sent_events) corriente. Es increíblemente
fácil de usar. Así es como se ve. También es posible que desee consultar el [ejemplo completo en GitHub](https://github.com/binwiederhier/ntfy/tree/main/examples/web-example-eventsource).

\=== "Línea de comandos (curl)"
\`\`\`
$ curl -s ntfy.sh/mytopic/sse
evento: abierto
datos: {"id":"weSj9RtNkj","time":1635528898,"event":"open","topic":"mytopic"}

    data: {"id":"p0M5y6gcCY","time":1635528909,"event":"message","topic":"mytopic","message":"Hi!"}

    event: keepalive
    data: {"id":"VNxNIg5fpt","time":1635528928,"event":"keepalive","topic":"test"}
    ...
    ```

\=== "HTTP"
''' http
OBTENER /mytopic/sse HTTP/1.1
Anfitrión: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: text/event-stream; charset=utf-8
    Transfer-Encoding: chunked

    event: open
    data: {"id":"weSj9RtNkj","time":1635528898,"event":"open","topic":"mytopic"}

    data: {"id":"p0M5y6gcCY","time":1635528909,"event":"message","topic":"mytopic","message":"Hi!"}

    event: keepalive
    data: {"id":"VNxNIg5fpt","time":1635528928,"event":"keepalive","topic":"test"}
    ...
    ```

\=== "JavaScript"
` javascript
    const eventSource = new EventSource('https://ntfy.sh/mytopic/sse');
    eventSource.onmessage = (e) => {
      console.log(e.data);
    };
    `

### Suscríbete como flujo raw

El `/raw` El extremo generará una línea por mensaje, y **sólo incluirá el cuerpo del mensaje**. Es útil para extremadamente
scripts simples, y no incluye todos los datos. Campos adicionales como [prioridad](../publish.md#message-priority),
[Etiquetas](../publish.md#tags--emojis--) o [título del mensaje](../publish.md#message-title) no se incluyen en este resultado
formato. Los mensajes de Keepalive se envían como líneas vacías.

\=== "Línea de comandos (curl)"
\`\`\`
$ curl -s ntfy.sh/disk-alerts/raw

    Disk full
    ...
    ```

\=== "HTTP"
''' http
GET /disk-alerts/raw HTTP/1.1
Anfitrión: ntfy.sh

    HTTP/1.1 200 OK
    Content-Type: text/plain; charset=utf-8
    Transfer-Encoding: chunked

    Disk full
    ...
    ```

\=== "Go"
` go
    resp, err := http.Get("https://ntfy.sh/disk-alerts/raw")
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        println(scanner.Text())
    }
    `

\=== "Python"
` python 
    resp = requests.get("https://ntfy.sh/disk-alerts/raw", stream=True)
    for line in resp.iter_lines():
      if line:
        print(line)
    `

\=== "PHP"
` php-inline
    $fp = fopen('https://ntfy.sh/disk-alerts/raw', 'r');
    if (!$fp) die('cannot open stream');
    while (!feof($fp)) {
        echo fgets($fp, 2048);
        flush();
    }
    fclose($fp);
    `

## WebSockets

También puede suscribirse a temas a través de [WebSockets](https://en.wikipedia.org/wiki/WebSocket), que también es ampliamente
soportado en muchos idiomas. En particular, webSockets son compatibles de forma nativa en JavaScript. En la línea de comandos,
Recomiendo [websocat](https://github.com/vi/websocat), una herramienta fantástica similar a `socat` o `curl`, pero específicamente
para WebSockets.

El extremo WebSockets está disponible en `<topic>/ws` y devuelve mensajes como objetos JSON similares a los
[Extremo de transmisión JSON](#subscribe-as-json-stream).

\=== "Línea de comandos (websocat)"
`     $ websocat wss://ntfy.sh/mytopic/ws
    {"id":"qRHUCCvjj8","time":1642307388,"event":"open","topic":"mytopic"}
    {"id":"eOWoUBJ14x","time":1642307754,"event":"message","topic":"mytopic","message":"hi there"}
    `

\=== "HTTP"
''' http
GET /disk-alerts/ws HTTP/1.1
Anfitrión: ntfy.sh
Actualización: websocket
Conexión: Actualización

    HTTP/1.1 101 Switching Protocols
    Upgrade: websocket
    Connection: Upgrade
    ...
    ```

\=== "Go"
` go
    import "github.com/gorilla/websocket"
	ws, _, _ := websocket.DefaultDialer.Dial("wss://ntfy.sh/mytopic/ws", nil)
	messageType, data, err := ws.ReadMessage()
    ...
    `

\=== "JavaScript"
` javascript
    const socket = new WebSocket('wss://ntfy.sh/mytopic/ws');
    socket.addEventListener('message', function (event) {
        console.log(event.data);
    });
    `

## Funciones avanzadas

### Sondeo para mensajes

También puede sondear mensajes si no le gusta la conexión de larga data utilizando el `poll=1`
parámetro de consulta. La conexión finalizará después de que se hayan leído todos los mensajes disponibles. Este parámetro puede ser
combinado con `since=` (el valor predeterminado es `since=all`).

    curl -s "ntfy.sh/mytopic/json?poll=1"

### Recuperar mensajes almacenados en caché

Los mensajes pueden almacenarse en caché durante un par de horas (consulte [almacenamiento en caché de mensajes](../config.md#message-cache)) para dar cuenta de la red
interrupciones de suscriptores. Si el servidor ha configurado el almacenamiento en caché de mensajes, puede volver a leer lo que se perdió mediante el uso de
el `since=` parámetro de consulta. Toma una duración (por ejemplo, `10m` o `30s`), una marca de tiempo Unix (por ejemplo, `1635528757`),
un ID de mensaje (por ejemplo, `nFS3knfcQ1xe`), o `all` (todos los mensajes almacenados en caché).

    curl -s "ntfy.sh/mytopic/json?since=10m"
    curl -s "ntfy.sh/mytopic/json?since=1645970742"
    curl -s "ntfy.sh/mytopic/json?since=nFS3knfcQ1xe"

### Recuperar mensajes programados

Mensajes que son [programado para ser entregado](../publish.md#scheduled-delivery) en una fecha posterior no suelen ser
devuelto al suscribirse a través de la API, lo que tiene sentido, porque después de todo, los mensajes técnicamente no han sido
entregado todavía. Para devolver también los mensajes programados de la API, puede utilizar el botón `scheduled=1` (alias: `sched=1`)
(tiene más sentido con el parámetro `poll=1` parámetro):

    curl -s "ntfy.sh/mytopic/json?poll=1&sched=1"

### Filtrar mensajes

Puede filtrar qué mensajes se devuelven en función de los campos de mensajes conocidos `id`, `message`, `title`, `priority` y
`tags`. Este es un ejemplo que solo devuelve mensajes de prioridad alta o urgente que contienen las dos etiquetas
"zfs-error" y "error". Tenga en cuenta que el `priority` el filtro es un QUI lógico y el `tags` el filtro es un AND lógico.

    $ curl "ntfy.sh/alerts/json?priority=high&tags=zfs-error"
    {"id":"0TIkJpBcxR","time":1640122627,"event":"open","topic":"alerts"}
    {"id":"X3Uzz9O1sM","time":1640122674,"event":"message","topic":"alerts","priority":4,
      "tags":["error", "zfs-error"], "message":"ZFS pool corruption detected"}

Filtros disponibles (todos sin distinción de mayúsculas y minúsculas):

| | de variables de filtro Alias | Ejemplo | Descripción |
|-----------------|---------------------------|-----------------------------------------------|-------------------------------------------------------------------------|
| `id`            | `X-ID`                    | `ntfy.sh/mytopic/json?poll=1&id=pbkiz8SD7ZxG` | Solo devuelve mensajes que coincidan con este ID de mensaje exacto |
| `message`       | `X-Message`, `m`          | `ntfy.sh/mytopic/json?message=lalala`         | Solo devuelve mensajes que coincidan con esta cadena de mensaje exacta |
| `title`         | `X-Title`, `t`            | `ntfy.sh/mytopic/json?title=some+title`       | Solo devuelve mensajes que coincidan con esta cadena de título exacta |
| `priority`      | `X-Priority`, `prio`, `p` | `ntfy.sh/mytopic/json?p=high,urgent`          | Solo devuelve mensajes que coinciden *cualquier prioridad enumerada* | (separados por comas)
| `tags`          | `X-Tags`, `tag`, `ta`     | `ntfy.sh/mytopic?/jsontags=error,alert`       | Solo devuelve mensajes que coinciden *todas las etiquetas enumeradas* | (separados por comas)

### Suscríbete a varios temas

Es posible suscribirse a varios temas en una llamada HTTP proporcionando una lista de temas separados por comas
en la URL. Esto le permite reducir el número de conexiones que debe mantener:

    $ curl -s ntfy.sh/mytopic1,mytopic2/json
    {"id":"0OkXIryH3H","time":1637182619,"event":"open","topic":"mytopic1,mytopic2,mytopic3"}
    {"id":"dzJJm7BCWs","time":1637182634,"event":"message","topic":"mytopic1","message":"for topic 1"}
    {"id":"Cm02DsxUHb","time":1637182643,"event":"message","topic":"mytopic2","message":"for topic 2"}

### Autenticación

Dependiendo de si el servidor está configurado para admitir [control de acceso](../config.md#access-control), algunos temas
puede estar protegido contra lectura/escritura para que solo los usuarios con las credenciales correctas puedan suscribirse o publicar en ellos.
Para publicar/suscribirse a temas protegidos, puede utilizar [Autenticación básica](https://en.wikipedia.org/wiki/Basic_access_authentication)
con un nombre de usuario/contraseña válido. Para su servidor autohospedado, **Asegúrese de usar HTTPS para evitar escuchas** y exponer
su contraseña.

    curl -u phil:mypass -s "https://ntfy.example.com/mytopic/json"

## Formato de mensaje JSON

Tanto el [`/json` Extremo](#subscribe-as-json-stream) y el [`/sse` Extremo](#subscribe-as-sse-stream) devolver un JSON
formato del mensaje. Es muy sencillo:

**Mensaje**:

| | de campo | requerido Tipo | Ejemplo | Descripción |
|--------------|----------|---------------------------------------------------|-------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `id`         | ✔️       | *cuerda*                                          | `hwQ2YpKdmg`                                          | Identificador de mensaje elegido al azar |
| `time`       | ✔️       | *número*                                          | `1635528741`                                          | Hora de la fecha del mensaje, como | de la marca de tiempo de Unix\
| `event`      | ✔️       | `open`, `keepalive`, `message`o `poll_request` | `message`                                             | Tipo de mensaje, normalmente solo te interesaría `message`                                                                        |
| `topic`      | ✔️       | *cuerda*                                          | `topic1,topic2`                                       | Lista separada por comas de los temas con los que está asociado el mensaje; sólo uno para todos `message` eventos, pero puede ser una lista en `open` | de eventos
| `message`    | -        | *cuerda*                                          | `Some message`                                        | Cuerpo del mensaje; siempre presente en `message` | de eventos
| `title`      | -        | *cuerda*                                          | `Some title`                                          | Mensaje [título](../publish.md#message-title); Si no, establezca los valores predeterminados en `ntfy.sh/<topic>`                                               |
| `tags`       | -        | *matriz de cadenas*                                    | `["tag1","tag2"]`                                     | Lista de [Etiquetas](../publish.md#tags-emojis) que pueden o no asignarse a emojis |
| `priority`   | -        | *1, 2, 3, 4 o 5*                                | `4`                                                   | Mensaje [prioridad](../publish.md#message-priority) con 1=min, 3=default y 5=max |
| `click`      | -        | *URL*                                             | `https://example.com`                                 | Sitio web abierto cuando se notifica [Clic](../publish.md#click-action)                                                            |
| `actions`    | -        | *Matriz JSON*                                      | *ver [botones de acciones](../publish.md#action-buttons)* | [Botones de acción](../publish.md#action-buttons) que se puede mostrar en el | de notificación
| `attachment` | -        | *Objeto JSON*                                     | *ver más abajo*                                           | Detalles sobre un archivo adjunto (nombre, URL, tamaño, ...)                                                                                   |

**Archivo adjunto** (parte del mensaje, véase [Accesorios](../publish.md#attachments) para más detalles):

| | de campo | requerido Tipo | Ejemplo | Descripción |
|-----------|----------|-------------|--------------------------------|-----------------------------------------------------------------------------------------------------------|
| `name`    | ✔️       | *cuerda*    | `attachment.jpg`               | Nombre del archivo adjunto, se puede anular con `X-Filename`ver [Accesorios](../publish.md#attachments) |
| `url`     | ✔️       | *URL*       | `https://example.com/file.jpg` | Dirección URL del | adjunto\
| `type`    | -️       | *tipo mime* | `image/jpeg`                   | Tipo mime de los datos adjuntos, solo definido si los datos adjuntos se cargaron en el servidor ntfy |
| `size`    | -️       | *número*    | `33848`                        | Tamaño de los datos adjuntos en bytes, solo definido si los datos adjuntos se cargaron en el servidor ntfy |
| `expires` | -️       | *número*    | `1635528741`                   | Fecha de caducidad de los datos adjuntos como marca de tiempo de Unix, solo definida si los datos adjuntos se cargaron en el servidor ntfy |

A continuación se muestra un ejemplo para cada tipo de mensaje:

\=== "Mensaje de notificación"
` json
    {
        "id": "sPs71M8A2T",
        "time": 1643935928,
        "event": "message",
        "topic": "mytopic",
        "priority": 5,
        "tags": [
            "warning",
            "skull"
        ],
        "click": "https://homecam.mynet.lan/incident/1234",
        "attachment": {
            "name": "camera.jpg",
            "type": "image/png",
            "size": 33848,
            "expires": 1643946728,
            "url": "https://ntfy.sh/file/sPs71M8A2T.png"
        },
        "title": "Unauthorized access detected",
        "message": "Movement detected in the yard. You better go check"
    }
    `

\=== "Mensaje de notificación (mínimo)"
` json
    {
        "id": "wze9zgqK41",
        "time": 1638542110,
        "event": "message",
        "topic": "phil_alerts",
        "message": "Remote access to phils-laptop detected. Act right away."
    }
    `

\=== "Mensaje abierto"
` json
    {
        "id": "2pgIAaGrQ8",
        "time": 1638542215,
        "event": "open",
        "topic": "phil_alerts"
    }
    `

\=== "Mensaje Keepalive"
` json
    {
        "id": "371sevb0pD",
        "time": 1638542275,
        "event": "keepalive",
        "topic": "phil_alerts"
    }
    `

\=== "Mensaje de solicitud de sondeo"
` json
    {
        "id": "371sevb0pD",
        "time": 1638542275,
        "event": "poll_request",
        "topic": "phil_alerts"
    }
    `

## Lista de todos los parámetros

La siguiente es una lista de todos los parámetros que se pueden pasar **al suscribirse a un mensaje**. Los nombres de los parámetros son **no distingue entre mayúsculas y minúsculas**,
y se puede pasar como **Encabezados HTTP** o **parámetros de consulta en la dirección URL**. Se enumeran en la tabla en su forma canónica.

| | de parámetros Alias (sin distinción entre mayúsculas y minúsculas) | Descripción |
|-------------|----------------------------|---------------------------------------------------------------------------------|
| `poll`      | `X-Poll`, `po`             | Devolver mensajes almacenados en caché y cerrar | de conexión
| `since`     | `X-Since`, `si`            | Devolver mensajes almacenados en caché desde la marca de tiempo, la duración o el ID del mensaje |
| `scheduled` | `X-Scheduled`, `sched`     | Incluir mensajes programados/retrasados en la lista de |
| `id`        | `X-ID`                     | Filtro: solo devuelve mensajes que coincidan con este ID de mensaje exacto |
| `message`   | `X-Message`, `m`           | Filtro: solo devuelve mensajes que coincidan con esta cadena de mensaje exacta |
| `title`     | `X-Title`, `t`             | Filtro: solo devuelve mensajes que coincidan con esta cadena de título exacta |
| `priority`  | `X-Priority`, `prio`, `p`  | Filtro: solo devuelve mensajes que coinciden *cualquier prioridad enumerada* | (separados por comas)
| `tags`      | `X-Tags`, `tag`, `ta`      | Filtro: solo devuelve mensajes que coinciden *todas las etiquetas enumeradas* | (separados por comas)
