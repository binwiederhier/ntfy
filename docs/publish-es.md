# Editorial

La publicación de mensajes se puede hacer a través de HTTP PUT / POST o a través del [CLI ntfy](install.md). Los temas son creados sobre la marcha por
suscribirse o publicar en ellos. Debido a que no hay registro, **El tema es esencialmente una contraseña**, así que elige
algo que no es fácil de adivinar.

Aquí hay un ejemplo que muestra cómo publicar un mensaje simple usando una solicitud POST:

\=== "Línea de comandos (curl)"
`     curl -d "Backup successful 😀" ntfy.sh/mytopic
    `

\=== "ntfy CLI"
`     ntfy publish mytopic "Backup successful 😀"
    `

\=== "HTTP"
''' http
POST /mytopic HTTP/1.1
Anfitrión: ntfy.sh

    Backup successful 😀
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mytopic', {
      method: 'POST', // PUT works too
      body: 'Backup successful 😀'
    })
    `

\=== "Go"
` go
    http.Post("https://ntfy.sh/mytopic", "text/plain",
        strings.NewReader("Backup successful 😀"))
    `

\=== "PowerShell"
` powershell
    Invoke-RestMethod -Method 'Post' -Uri https://ntfy.sh/mytopic -Body "Backup successful" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/mytopic", 
        data="Backup successful 😀".encode(encoding='utf-8'))
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' => 'Content-Type: text/plain',
            'content' => 'Backup successful 😀'
        ]
    ]));
    `

Si tiene el [Aplicación para Android](subscribe/phone.md) instalado en su teléfono, esto creará una notificación que se ve así:

<figure markdown>
  ![basic notification](static/img/android-screenshot-basic-notification.png){ width=500 }
  <figcaption>Android notification</figcaption>
</figure>

Hay más características relacionadas con la publicación de mensajes: puede establecer un [prioridad de notificación](#message-priority),
un [título](#message-title)y [mensajes de etiqueta](#tags-emojis) 🥳 🎉. Aquí hay un ejemplo que usa algunos de ellos juntos:

\=== "Línea de comandos (curl)"
`     curl \       -H "Title: Unauthorized access detected" \       -H "Priority: urgent" \       -H "Tags: warning,skull" \       -d "Remote access to phils-laptop detected. Act right away." \
      ntfy.sh/phil_alerts
    `

\=== "ntfy CLI"
`     ntfy publish \         --title "Unauthorized access detected" \         --tags warning,skull \         --priority urgent \
        mytopic \
        "Remote access to phils-laptop detected. Act right away."
    `

\=== "HTTP"
''' http
POST /phil_alerts HTTP/1.1
Anfitrión: ntfy.sh
Título: Acceso no autorizado detectado
Prioridad: urgente
Etiquetas: advertencia,cráneo

    Remote access to phils-laptop detected. Act right away.
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/phil_alerts', {
        method: 'POST', // PUT works too
        body: 'Remote access to phils-laptop detected. Act right away.',
        headers: {
            'Title': 'Unauthorized access detected',
            'Priority': 'urgent',
            'Tags': 'warning,skull'
        }
    })
    `

\=== "Go"
` go
	req, _ := http.NewRequest("POST", "https://ntfy.sh/phil_alerts",
		strings.NewReader("Remote access to phils-laptop detected. Act right away."))
	req.Header.Set("Title", "Unauthorized access detected")
	req.Header.Set("Priority", "urgent")
	req.Header.Set("Tags", "warning,skull")
	http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/phil_alerts"
    $headers = @{ Title="Unauthorized access detected"
                  Priority="urgent"
                  Tags="warning,skull" }
    $body = "Remote access to phils-laptop detected. Act right away."              
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/phil_alerts",
        data="Remote access to phils-laptop detected. Act right away.",
        headers={
            "Title": "Unauthorized access detected",
            "Priority": "urgent",
            "Tags": "warning,skull"
        })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/phil_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Title: Unauthorized access detected\r\n" .
                "Priority: urgent\r\n" .
                "Tags: warning,skull",
            'content' => 'Remote access to phils-laptop detected. Act right away.'
        ]
    ]));
    `

<figure markdown>
  ![priority notification](static/img/priority-notification.png){ width=500 }
  <figcaption>Urgent notification with tags and title</figcaption>
</figure>

También puede hacer mensajes de varias líneas. Aquí hay un ejemplo usando un [Haga clic en acción](#click-action)un [botón de acción](#action-buttons),
un [adjunto de imagen externa](#attach-file-from-a-url) y [publicación de correo electrónico](#e-mail-publishing):

\=== "Línea de comandos (curl)"
\`\`\`
rizo \
\-H "Clic: https://home.nest.com/" \
\-H "Adjuntar: https://nest.com/view/yAxkasd.jpg" \
\-H "Acciones: http, Open door, https://api.nest.com/open/yAxkasd, clear=true" \
\-H "Email: phil@example.com" \
\-d "Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell." \
      ntfy.sh/mydoorbell
    ```

\=== "ntfy CLI"
\`\`\`
ntfy publicar \
\--click="https://home.nest.com/" \
\--attach="https://nest.com/view/yAxkasd.jpg" \
\--actions="http, Abrir puerta, https://api.nest.com/open/yAxkasd, clear=true" \
\--email="phil@example.com" \
mydoorbell \
"Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell."
    ```

\=== "HTTP"
''' http
POST /mydoorbell HTTP/1.1
Anfitrión: ntfy.sh
Clic: https://home.nest.com/
Adjuntar: https://nest.com/view/yAxkasd.jpg
Acciones: http, Open door, https://api.nest.com/open/yAxkasd, clear=true
Correo electrónico: phil@example.com

    There's someone at the door. 🐶

    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.
    ```

\=== "JavaScript"
''' javascript
fetch('https://ntfy.sh/mydoorbell', {
método: 'POST', // PUT también funciona
encabezados: {
'Clic': 'https://home.nest.com/',
'Adjuntar': 'https://nest.com/view/yAxkasd.jpg',
'Acciones': 'http, Open door, https://api.nest.com/open/yAxkasd, clear=true',
'Correo electrónico': 'phil@example.com'
},
Cuerpo: 'Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.`,
    })
    ```

\=== "Go"
''' ir
req, \_ := http. NewRequest("POST", "https://ntfy.sh/mydoorbell",
instrumentos de cuerda. NewReader('Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman. 
    Doggies have been known to ring the doorbell.`))
    req.Header.Set("Click", "https://home.nest.com/")
    req.Header.Set("Attach", "https://nest.com/view/yAxkasd.jpg")
    req.Header.Set("Actions", "http, Open door, https://api.nest.com/open/yAxkasd, clear=true")
    req.Header.Set("Email", "phil@example.com")
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
Powershell '''
$uri = "https://ntfy.sh/mydoorbell"
$headers = @{ Click="https://home.nest.com/"
Attach="https://nest.com/view/yAxkasd.jpg"
Acciones="http, Puerta abierta, https://api.nest.com/open/yAxkasd, clear=true"
Email="phil@example.com" }
$body = @'
Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman.
    Doggies have been known to ring the doorbell.
    '@
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    ```

\=== "Python"
''' python
requests.post("https://ntfy.sh/mydoorbell",
data="""Hay alguien en la puerta. 🐶

    Please check if it's a good boy or a hooman.
    Doggies have been known to ring the doorbell.""".encode('utf-8'),
        headers={
            "Click": "https://home.nest.com/",
            "Attach": "https://nest.com/view/yAxkasd.jpg",
            "Actions": "http, Open door, https://api.nest.com/open/yAxkasd, clear=true",
            "Email": "phil@example.com"
        })
    ```

\=== "PHP"
''' php-inline
file_get_contents('https://ntfy.sh/mydoorbell', false, stream_context_create(\[
'http' => \[
'método' = > 'POST', // PUT también funciona
'encabezado' = >
"Content-Type: text/plain\r\n" .
"Haga clic: https://home.nest.com/\r\n" .
"Adjuntar: https://nest.com/view/yAxkasd.jpg\r\n" .
"Acciones": "http, Open door, https://api.nest.com/open/yAxkasd, clear=true\r\n" .
"Correo electrónico": "phil@example.com\r\n",
'contenido' = > 'Hay alguien en la puerta. 🐶

    Please check if it\'s a good boy or a hooman.
    Doggies have been known to ring the doorbell.'
        ]
    ]));
    ```

<figure markdown>
  ![priority notification](static/img/android-screenshot-notification-multiline.jpg){ width=500 }
  <figcaption>Notification using a click action, a user action, with an external image attachment and forwarded via email</figcaption>
</figure>

## Título del mensaje

*Soportado en:* :material-android: :material-manzana: :material-firefox:

El título de la notificación normalmente se establece en la URL corta del tema (por ejemplo, `ntfy.sh/mytopic`). Para anular el título,
Puede establecer el `X-Title` encabezado (o cualquiera de sus alias: `Title`, `ti`o `t`).

\=== "Línea de comandos (curl)"
`     curl -H "X-Title: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    curl -H "Title: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    curl -H "t: Dogs are better than cats" -d "Oh my ..." ntfy.sh/controversial
    `

\=== "ntfy CLI"
`     ntfy publish \         -t "Dogs are better than cats" \
        controversial "Oh my ..."
    `

\=== "HTTP"
''' http
POST /controversial HTTP/1.1
Anfitrión: ntfy.sh
Título: Los perros son mejores que los gatos

    Oh my ...
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/controversial', {
        method: 'POST',
        body: 'Oh my ...',
        headers: { 'Title': 'Dogs are better than cats' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/controversial", strings.NewReader("Oh my ..."))
    req.Header.Set("Title", "Dogs are better than cats")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/controversial"
    $headers = @{ Title="Dogs are better than cats" }
    $body = "Oh my ..."
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/controversial",
        data="Oh my ...",
        headers={ "Title": "Dogs are better than cats" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/controversial', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Title: Dogs are better than cats",
            'content' => 'Oh my ...'
        ]
    ]));
    `

<figure markdown>
  ![notification with title](static/img/notification-with-title.png){ width=500 }
  <figcaption>Detail view of notification with title</figcaption>
</figure>

## Prioridad del mensaje

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Todos los mensajes tienen una prioridad, que define la urgencia con la que su teléfono le notifica. En Android, puedes configurar la configuración personalizada
sonidos de notificación y patrones de vibración en el teléfono para asignarlos a estas prioridades (consulte [Configuración de Android](subscribe/phone.md)).

Existen las siguientes prioridades:

| Prioridad | | de iconos | de identificación Nombre | Descripción |
|----------------------|--------------------------------------------|-----|----------------|--------------------------------------------------------------------------------------------------------|
| Prioridad máxima | ![min priority](static/img/priority-5.svg) | `5` | `max`/`urgent` | Ráfagas de vibración realmente largas, sonido de notificación predeterminado con una notificación emergente.                 |
| | de alta prioridad ![min priority](static/img/priority-4.svg) | `4` | `high`         | Ráfaga de vibración larga, sonido de notificación predeterminado con una notificación emergente.                         |
| **Prioridad predeterminada** | *(ninguno)*                                   | `3` | `default`      | Vibración y sonido cortos por defecto. Comportamiento de notificación predeterminado.                                      |
| | de baja prioridad ![min priority](static/img/priority-2.svg) | `2` | `low`          | Sin vibración ni sonido. La notificación no aparecerá visiblemente hasta que se baje el cajón de notificaciones. |
| Prioridad mínima | ![min priority](static/img/priority-1.svg) | `1` | `min`          | Sin vibración ni sonido. La notificación estará bajo el pliegue en "Otras notificaciones".               |

Puede establecer la prioridad con el encabezado `X-Priority` (o cualquiera de sus alias: `Priority`, `prio`o `p`).

\=== "Línea de comandos (curl)"
`     curl -H "X-Priority: 5" -d "An urgent message" ntfy.sh/phil_alerts
    curl -H "Priority: low" -d "Low priority message" ntfy.sh/phil_alerts
    curl -H p:4 -d "A high priority message" ntfy.sh/phil_alerts
    `

\=== "ntfy CLI"
`     ntfy publish \          -p 5 \
        phil_alerts An urgent message
    `

\=== "HTTP"
''' http
POST /phil_alerts HTTP/1.1
Anfitrión: ntfy.sh
Prioridad: 5

    An urgent message
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/phil_alerts', {
        method: 'POST',
        body: 'An urgent message',
        headers: { 'Priority': '5' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/phil_alerts", strings.NewReader("An urgent message"))
    req.Header.Set("Priority", "5")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/phil_alerts"
    $headers = @{ Priority="5" }
    $body = "An urgent message"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/phil_alerts",
        data="An urgent message",
        headers={ "Priority": "5" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/phil_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Priority: 5",
            'content' => 'An urgent message'
        ]
    ]));
    `

<figure markdown>
  ![priority notification](static/img/priority-detail-overview.png){ width=500 }
  <figcaption>Detail view of priority notifications</figcaption>
</figure>

## Etiquetas y emojis 🥳 🎉

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puedes etiquetar mensajes con emojis y otras cadenas relevantes:

*   **Emojis**: Si una etiqueta coincide con un [emoji código corto](emojis.md), se convertirá en un emoji y se antepondrá
    al título o mensaje.
*   **Otras etiquetas:** Si una etiqueta no coincide, aparecerá debajo de la notificación.

Esta función es útil para cosas como advertencias (⚠️, ️🚨, o 🚩 ), pero también para simplemente etiquetar mensajes de lo contrario (por ejemplo, script
nombres, nombres de host, etc.). Uso [la lista de códigos cortos de emoji](emojis.md) para averiguar qué etiquetas se pueden convertir en emojis.
Aquí hay un **extracto de emojis** He encontrado muy útil en los mensajes de alerta:

<table class="remove-md-box"><tr>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody>
    <tr><td><code>+1</code></td><td>👍</td></tr>
    <tr><td><code>partying_face</code></td><td>🥳</td></tr>
    <tr><td><code>tada</code></td><td>🎉</td></tr>
    <tr><td><code>heavy_check_mark</code></td><td>✔️</td></tr>
    <tr><td><code>loudspeaker</code></td><td>📢</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody> 
    <tr><td><code>-1</code></td><td>👎️</td></tr>
    <tr><td><code>warning</code></td><td>⚠️</td></tr>
    <tr><td><code>rotating_light</code></td><td>️🚨</td></tr>
    <tr><td><code>triangular_flag_on_post</code></td><td>🚩</td></tr>
    <tr><td><code>skull</code></td><td>💀</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
<td>
    <table><thead><tr><th>Tag</th><th>Emoji</th></tr></thead><tbody>
    <tr><td><code>facepalm</code></td><td>🤦</td></tr>
    <tr><td><code>no_entry</code></td><td>⛔</td></tr>
    <tr><td><code>no_entry_sign</code></td><td>🚫</td></tr>
    <tr><td><code>cd</code></td><td>💿</td></tr> 
    <tr><td><code>computer</code></td><td>💻</td></tr>
    <tr><td>...</td><td>...</td></tr>
    </tbody></table>
</td>
</tr></table>

Puede establecer etiquetas con el botón `X-Tags` encabezado (o cualquiera de sus alias: `Tags`, `tag`o `ta`). Especifique varias etiquetas separando
ellos con una coma, por ejemplo, `tag1,tag2,tag3`.

\=== "Línea de comandos (curl)"
`     curl -H "X-Tags: warning,mailsrv13,daily-backup" -d "Backup of mailsrv13 failed" ntfy.sh/backups
    curl -H "Tags: horse,unicorn" -d "Unicorns are just horses with unique horns" ntfy.sh/backups
    curl -H ta:dog -d "Dogs are awesome" ntfy.sh/backups
    `

\=== "ntfy CLI"
`     ntfy publish \         --tags=warning,mailsrv13,daily-backup \
        backups "Backup of mailsrv13 failed"
    `

\=== "HTTP"
''' http
POST /backups HTTP/1.1
Anfitrión: ntfy.sh
Etiquetas: warning,mailsrv13,daily-backup

    Backup of mailsrv13 failed
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/backups', {
        method: 'POST',
        body: 'Backup of mailsrv13 failed',
        headers: { 'Tags': 'warning,mailsrv13,daily-backup' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/backups", strings.NewReader("Backup of mailsrv13 failed"))
    req.Header.Set("Tags", "warning,mailsrv13,daily-backup")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/backups"
    $headers = @{ Tags="warning,mailsrv13,daily-backup" }
    $body = "Backup of mailsrv13 failed"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/backups",
        data="Backup of mailsrv13 failed",
        headers={ "Tags": "warning,mailsrv13,daily-backup" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/backups', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Tags: warning,mailsrv13,daily-backup",
            'content' => 'Backup of mailsrv13 failed'
        ]
    ]));
    `

<figure markdown>
  ![priority notification](static/img/notification-with-tags.png){ width=500 }
  <figcaption>Detail view of notifications with tags</figcaption>
</figure>

## Entrega programada

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puede retrasar la entrega de mensajes y dejar que ntfy los envíe en una fecha posterior. Esto se puede utilizar para enviarse a sí mismo
recordatorios o incluso para ejecutar comandos en una fecha posterior (si su suscriptor actúa sobre los mensajes).

El uso es bastante sencillo. Puede establecer el tiempo de entrega utilizando el botón `X-Delay` encabezado (o cualquiera de sus alias: `Delay`,
`X-At`, `At`, `X-In` o `In`), ya sea especificando una marca de tiempo Unix (por ejemplo, `1639194738`), una duración (por ejemplo, `30m`,
`3h`, `2 days`), o una cadena de tiempo de lenguaje natural (por ejemplo, `10am`, `8:30pm`, `tomorrow, 3pm`, `Tuesday, 7am`,
[y más](https://github.com/olebedev/when)).

A partir de hoy, el retraso mínimo que puede establecer es **10 segundos** y el retraso máximo es **3 días**. Esto puede actualmente
no se configurará de otra manera ([hágamelo saber](https://github.com/binwiederhier/ntfy/issues) si quieres cambiar
estos límites).

A los efectos de [almacenamiento en caché de mensajes](config.md#message-cache), los mensajes programados se mantienen en la memoria caché hasta 12 horas
después de que se entregaron (o cualquiera que sea la duración de la memoria caché del lado del servidor establecida en). Por ejemplo, si un mensaje está programado
para ser entregado en 3 días, permanecerá en la memoria caché durante 3 días y 12 horas. También tenga en cuenta que, naturalmente,
[Desactivar el almacenamiento en caché del lado del servidor](#message-caching) no es posible en combinación con esta característica.

\=== "Línea de comandos (curl)"
`     curl -H "At: tomorrow, 10am" -d "Good morning" ntfy.sh/hello
    curl -H "In: 30min" -d "It's 30 minutes later now" ntfy.sh/reminder
    curl -H "Delay: 1639194738" -d "Unix timestamps are awesome" ntfy.sh/itsaunixsystem
    `

\=== "ntfy CLI"
`     ntfy publish \         --at="tomorrow, 10am" \
        hello "Good morning"
    `

\=== "HTTP"
''' http
POST /hola HTTP/1.1
Anfitrión: ntfy.sh
A: mañana, 10am

    Good morning
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/hello', {
        method: 'POST',
        body: 'Good morning',
        headers: { 'At': 'tomorrow, 10am' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/hello", strings.NewReader("Good morning"))
    req.Header.Set("At", "tomorrow, 10am")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/hello"
    $headers = @{ At="tomorrow, 10am" }
    $body = "Good morning"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/hello",
        data="Good morning",
        headers={ "At": "tomorrow, 10am" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/backups', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "At: tomorrow, 10am",
            'content' => 'Good morning'
        ]
    ]));
    `

Aquí hay algunos ejemplos (suponiendo que la fecha de hoy sea **12/10/2021, 9am, zona horaria del este**):

<table class="remove-md-box"><tr>
<td>
    <table><thead><tr><th><code>Delay/At/In</code> header</th><th>Message will be delivered at</th><th>Explanation</th></tr></thead><tbody>
    <tr><td><code>30m</code></td><td>12/10/2021, 9:<b>30</b>am</td><td>30 minutes from now</td></tr>
    <tr><td><code>2 hours</code></td><td>12/10/2021, <b>11:30</b>am</td><td>2 hours from now</td></tr>
    <tr><td><code>1 day</code></td><td>12/<b>11</b>/2021, 9am</td><td>24 hours from now</td></tr>
    <tr><td><code>10am</code></td><td>12/10/2021, <b>10am</b></td><td>Today at 10am (same day, because it's only 9am)</td></tr>
    <tr><td><code>8am</code></td><td>12/<b>11</b>/2021, <b>8am</b></td><td>Tomorrow at 8am (because it's 9am already)</td></tr>
    <tr><td><code>1639152000</code></td><td>12/10/2021, 11am (EST)</td><td> Today at 11am (EST)</td></tr>
    </tbody></table>
</td>
</tr></table>

## Webhooks (publicar a través de GET)

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Además de usar PUT/POST, también puede enviar a los temas a través de simples solicitudes HTTP GET. Esto hace que sea fácil de usar
un tema ntfy como un [webhook](https://en.wikipedia.org/wiki/Webhook), o si su cliente tiene soporte HTTP limitado (por ejemplo,
como el [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid) Aplicación android).

Para enviar mensajes a través de HTTP GET, simplemente llame al `/publish` endpoint (o sus alias) `/send` y `/trigger`). Sin
cualquier argumento, esto enviará el mensaje `triggered` al tema. Sin embargo, puede proporcionar todos los argumentos que son
también se admiten como encabezados HTTP como argumentos codificados en URL. Asegúrese de verificar la lista de todos
[parámetros y encabezados admitidos](#list-of-all-parameters) para más detalles.

Por ejemplo, asumiendo que su tema es `mywebhook`, simplemente puede llamar `/mywebhook/trigger` Para enviar un mensaje
(también conocido como activar el webhook):

\=== "Línea de comandos (curl)"
`     curl ntfy.sh/mywebhook/trigger
    `

\=== "ntfy CLI"
`     ntfy trigger mywebhook
    `

\=== "HTTP"
` http
    GET /mywebhook/trigger HTTP/1.1
    Host: ntfy.sh
    `

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mywebhook/trigger')
    `

\=== "Go"
` go
    http.Get("https://ntfy.sh/mywebhook/trigger")
    `

\=== "PowerShell"
` powershell
    Invoke-RestMethod -Method 'Get' -Uri "ntfy.sh/mywebhook/trigger"
    `

\=== "Python"
` python
    requests.get("https://ntfy.sh/mywebhook/trigger")
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mywebhook/trigger');
    `

Para agregar un mensaje personalizado, simplemente agregue el `message=` Parámetro URL. Y, por supuesto, puede configurar el
[prioridad del mensaje](#message-priority)el [título del mensaje](#message-title)y [Etiquetas](#tags-emojis) También.
Para obtener una lista completa de los posibles parámetros, consulte la lista de [parámetros y encabezados admitidos](#list-of-all-parameters).

Aquí hay un ejemplo con un mensaje personalizado, etiquetas y una prioridad:

\=== "Línea de comandos (curl)"
`     curl "ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull"
    `

\=== "ntfy CLI"
`     ntfy publish \         -p 5 --tags=warning,skull \
        mywebhook "Webhook triggered"
    `

\=== "HTTP"
` http
    GET /mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull HTTP/1.1
    Host: ntfy.sh
    `

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull')
    `

\=== "Go"
` go
    http.Get("https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull")
    `

\=== "PowerShell"
` powershell
    Invoke-RestMethod -Method 'Get' -Uri "ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull"
    `

\=== "Python"
` python
    requests.get("https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull")
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mywebhook/publish?message=Webhook+triggered&priority=high&tags=warning,skull');
    `

## Publicar como JSON

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Para algunas integraciones con otras herramientas (por ejemplo, [Jellyfin](https://jellyfin.org/), [supervisor](https://overseerr.dev/)),
Agregar encabezados personalizados a las solicitudes HTTP puede ser complicado o imposible, por lo que Ntfy también permite publicar todo el mensaje
como JSON en el cuerpo de la solicitud.

Para publicar como JSON, simplemente PONGA/POST el objeto JSON directamente en la URL raíz de ntfy. El formato del mensaje se describe a continuación
el ejemplo.

!!! información
Para publicar como JSON, debe **PUT/POST a la URL raíz de ntfy**, no a la dirección URL del tema. Asegúrate de comprobar que estás
POST-ing a `https://ntfy.sh/` (correcto), y no `https://ntfy.sh/mytopic` (incorrecto).

Aquí hay un ejemplo que usa la mayoría de los parámetros compatibles. Consulte la tabla a continuación para obtener una lista completa. El `topic` parámetro
es el único requerido:

\=== "Línea de comandos (curl)"
`     curl ntfy.sh \       -d '{
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
      }'
    `

\=== "HTTP"
''' http
POST / HTTP/1.1
Anfitrión: ntfy.sh

    {
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
    }
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            "topic": "mytopic",
            "message": "Disk space is low at 5.1 GB",
            "title": "Low disk space alert",
            "tags": ["warning","cd"],
            "priority": 4,
            "attach": "https://filesrv.lan/space.jpg",
            "filename": "diskspace.jpg",
            "click": "https://homecamera.lan/xasds1h2xsSsa/",
            "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
        })
    })
    `

\=== "Go"
''' ir
Probablemente deberías usar json. Marshal() en su lugar y hacer una estructura adecuada,
o incluso simplemente usar req. Header.Set() como en los otros ejemplos, pero para el
Por ejemplo, esto es más fácil.

    body := `{
        "topic": "mytopic",
        "message": "Disk space is low at 5.1 GB",
        "title": "Low disk space alert",
        "tags": ["warning","cd"],
        "priority": 4,
        "attach": "https://filesrv.lan/space.jpg",
        "filename": "diskspace.jpg",
        "click": "https://homecamera.lan/xasds1h2xsSsa/",
        "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh"
    $body = @{
            "topic"="powershell"
            "title"="Low disk space alert"
            "message"="Disk space is low at 5.1 GB"
            "priority"=4
            "attach"="https://filesrv.lan/space.jpg"
            "filename"="diskspace.jpg"
            "tags"=@("warning","cd")
            "click"= "https://homecamera.lan/xasds1h2xsSsa/"
            "actions"=@[@{ "action"="view", "label"="Admin panel", "url"="https://filesrv.lan/admin" }]
          } | ConvertTo-Json
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -ContentType "application/json" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "mytopic",
            "message": "Disk space is low at 5.1 GB",
            "title": "Low disk space alert",
            "tags": ["warning","cd"],
            "priority": 4,
            "attach": "https://filesrv.lan/space.jpg",
            "filename": "diskspace.jpg",
            "click": "https://homecamera.lan/xasds1h2xsSsa/",
            "actions": [{ "action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" }]
        })
    )
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "mytopic",
                "message": "Disk space is low at 5.1 GB",
                "title": "Low disk space alert",
                "tags": ["warning","cd"],
                "priority": 4,
                "attach": "https://filesrv.lan/space.jpg",
                "filename": "diskspace.jpg",
                "click": "https://homecamera.lan/xasds1h2xsSsa/",
                "actions": [["action": "view", "label": "Admin panel", "url": "https://filesrv.lan/admin" ]]
            ])
        ]
    ]));
    `

El formato de mensaje JSON refleja de cerca el formato del mensaje que puede consumir cuando [suscribirse a través de la API](subscribe/api.md)
(ver [Formato de mensaje JSON](subscribe/api.md#json-message-format) para más detalles), pero no es exactamente idéntico. Aquí hay una descripción general de
todos los campos admitidos:

| | de campo | requerido Tipo | Ejemplo | Descripción |
|------------|----------|----------------------------------|-------------------------------------------|-----------------------------------------------------------------------|
| `topic`    | ✔️       | *cuerda*                         | `topic1`                                  | Nombre del tema de destino |
| `message`  | -        | *cuerda*                         | `Some message`                            | Cuerpo del mensaje; establecer en `triggered` si está vacío o no se pasa |
| `title`    | -        | *cuerda*                         | `Some title`                              | Mensaje [título](#message-title)                                       |
| `tags`     | -        | *matriz de cadenas*                   | `["tag1","tag2"]`                         | Lista de [Etiquetas](#tags-emojis) que pueden o no asignarse a emojis |
| `priority` | -        | *int (uno de: 1, 2, 3, 4 o 5)* | `4`                                       | Mensaje [prioridad](#message-priority) con 1=min, 3=default y 5=max |
| `actions`  | -        | *Matriz JSON*                     | *(ver [botones de acción](#action-buttons))* | Costumbre [botones de acción del usuario](#action-buttons) para notificaciones |
| `click`    | -        | *URL*                            | `https://example.com`                     | Sitio web abierto cuando se notifica [Clic](#click-action)          |
| `attach`   | -        | *URL*                            | `https://example.com/file.jpg`            | URL de un archivo adjunto, consulte [adjuntar a través de URL](#attach-file-from-url)     |
| `filename` | -        | *cuerda*                         | `file.jpg`                                | Nombre de archivo del | adjunto
| `delay`    | -        | *cuerda*                         | `30min`, `9am`                            | Marca de tiempo o duración para la entrega retrasada |
| `email`    | -        | *dirección de correo electrónico*                 | `phil@example.com`                        | Dirección de correo electrónico para notificaciones por correo electrónico |

## Botones de acción

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puede agregar botones de acción a las notificaciones para poder reaccionar directamente a una notificación. Esto es increíblemente
útil y tiene innumerables aplicaciones.

Puede controlar sus electrodomésticos (abrir / cerrar la puerta del garaje, cambiar la temperatura en el termostato, ...), reaccionar a los comunes
alertas de monitoreo (borrar registros cuando el disco está lleno, ...), y muchas otras cosas. El cielo es el límite.

A partir de hoy, se admiten las siguientes acciones:

*   [`view`](#open-websiteapp): Abre un sitio web o una aplicación cuando se pulsa el botón de acción
*   [`broadcast`](#send-android-broadcast): Envía un [Transmisión de Android](https://developer.android.com/guide/components/broadcasts) intento
    cuando se toca el botón de acción (solo compatible con Android)
*   [`http`](#send-http-request): Envía la solicitud HTTP POST/GET/PUT cuando se pulsa el botón de acción

Aquí hay un ejemplo de cómo puede verse una notificación con acciones:

<figure markdown>
  ![notification with actions](static/img/android-screenshot-notification-actions.png){ width=500 }
  <figcaption>Notification with two user actions</figcaption>
</figure>

### Definición de acciones

Puede definir **hasta tres acciones de usuario** en las notificaciones, utilizando cualquiera de los métodos siguientes:

*   En [`X-Actions` encabezado](#using-a-header), utilizando un formato simple separado por comas
*   Como [Matriz JSON](#using-a-json-array) En `actions` clave, cuando [publicar como JSON](#publish-as-json)

#### Uso de un encabezado

Para definir acciones mediante el `X-Actions` encabezado (o cualquiera de sus alias: `Actions`, `Action`), utilice el siguiente formato:

\=== "Formato de cabecera (largo)"
`     action=<action1>, label=<label1>, paramN=... [; action=<action2>, label=<label2>, ...]
    `

\=== "Formato de cabecera (corto)"
`     <action1>, <label1>, paramN=... [; <action2>, <label2>, ...]
    `

Varias acciones están separadas por un punto y coma (`;`), y los pares clave/valor están separados por comas (`,`). Los valores pueden ser
citado con comillas dobles (`"`) o comillas simples (`'`) si el propio valor contiene comas o punto y coma.

El `action=` y `label=` el prefijo es opcional en todas las acciones, y el `url=` El prefijo es opcional en el cuadro de diálogo `view` y
`http` acción. La única limitación de este formato es que, dependiendo de su idioma/biblioteca, es posible que los caracteres UTF-8 no
trabajo. Si no lo hacen, use el botón [Formato de matriz JSON](#using-a-json-array) en lugar de.

Como ejemplo, así es como puede crear la notificación anterior utilizando este formato. Consulte el [`view` acción](#open-websiteapp) y
[`http` acción](#send-http-request) para obtener detalles sobre las acciones específicas:

\=== "Línea de comandos (curl)"
`     body='{"temperature": 65}'
    curl \         -d "You left the house. Turn down the A/C?" \         -H "Actions: view, Open portal, https://home.nest.com/, clear=true; \
                     http, Turn down, https://api.nest.com/, body='$body'" \
        ntfy.sh/myhome
    `

\=== "ntfy CLI"
`     body='{"temperature": 65}'
    ntfy publish \         --actions="view, Open portal, https://home.nest.com/, clear=true; \
                   http, Turn down, https://api.nest.com/, body='$body'" \
        myhome \
        "You left the house. Turn down the A/C?"
    `

\=== "HTTP"
''' http
POST /myhome HTTP/1.1
Anfitrión: ntfy.sh
Acciones: view, Open portal, https://home.nest.com/, clear=true; http, Bajar, https://api.nest.com/, cuerpo='{"temperatura": 65}'

    You left the house. Turn down the A/C?
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'You left the house. Turn down the A/C?',
        headers: { 
            'Actions': 'view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body=\'{"temperature": 65}\'' 
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("You left the house. Turn down the A/C?"))
    req.Header.Set("Actions", "view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/myhome"
    $headers = @{ Actions="view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'" }
    $body = "You left the house. Turn down the A/C?"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/myhome",
        data="You left the house. Turn down the A/C?",
        headers={ "Actions": "view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: view, Open portal, https://home.nest.com/, clear=true; http, Turn down, https://api.nest.com/, body='{\"temperature\": 65}'",
            'content' => 'You left the house. Turn down the A/C?'
        ]
    ]));
    `

#### Uso de una matriz JSON

Alternativamente, las mismas acciones se pueden definir como **Matriz JSON**, si la notificación se define como parte del cuerpo JSON
(ver [publicar como JSON](#publish-as-json)):

\=== "Línea de comandos (curl)"
`     curl ntfy.sh \       -d '{
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
      }'
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions '[
            {
                "action": "view",
                "label": "Open portal",
                "url": "https://home.nest.com/",
                "clear": true
            },
            {
                "action": "http",
                "label": "Turn down",
                "url": "https://api.nest.com/",
                "body": "{\"temperature\": 65}"
            }
        ]' \
        myhome \
        "You left the house. Turn down the A/C?"
    `

\=== "HTTP"
''' http
POST / HTTP/1.1
Anfitrión: ntfy.sh

    {
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
    }
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message": "You left the house. Turn down the A/C?",
            actions: [
                {
                    action: "view",
                    label: "Open portal",
                    url: "https://home.nest.com/",
                    clear: true
                },
                {
                    action: "http",
                    label: "Turn down",
                    url: "https://api.nest.com/",
                    body: "{\"temperature\": 65}"
                }
            ]
        })
    })
    `

\=== "Go"
''' ir
Probablemente deberías usar json. Marshal() en su lugar y hacer una estructura adecuada,
pero por el bien del ejemplo, esto es más fácil.

    body := `{
        "topic": "myhome",
        "message": "You left the house. Turn down the A/C?",
        "actions": [
          {
            "action": "view",
            "label": "Open portal",
            "url": "https://home.nest.com/",
            "clear": true
          },
          {
            "action": "http",
            "label": "Turn down",
            "url": "https://api.nest.com/",
            "body": "{\"temperature\": 65}"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh"
    $body = @{
        "topic"="myhome"
        "message"="You left the house. Turn down the A/C?"
        "actions"=@(
            @{
                "action"="view"
                "label"="Open portal"
                "url"="https://home.nest.com/"
                "clear"=true
            },
            @{
                "action"="http",
                "label"="Turn down"
                "url"="https://api.nest.com/"
                "body"="{\"temperature\": 65}"
            }
        )
    } | ConvertTo-Json
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -ContentType "application/json" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "You left the house. Turn down the A/C?",
            "actions": [
                {
                    "action": "view",
                    "label": "Open portal",
                    "url": "https://home.nest.com/",
                    "clear": true
                },
                {
                    "action": "http",
                    "label": "Turn down",
                    "url": "https://api.nest.com/",
                    "body": "{\"temperature\": 65}"
                }
            ]
        })
    )
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "You left the house. Turn down the A/C?",
                "actions": [                     [
                        "action": "view",
                        "label": "Open portal",
                        "url": "https://home.nest.com/",
                        "clear": true
                    ],                     [
                        "action": "http",
                        "label": "Turn down",
                        "url": "https://api.nest.com/",
                        "headers": [
                            "Authorization": "Bearer ..."
                        ],
                        "body": "{\"temperature\": 65}"
                    ]
                ]
            ])
        ]
    ]));
    `

Los campos obligatorios/opcionales para cada acción dependen del tipo de acción en sí. Por favor refiérase a
[`view` acción](#open-websiteapp), [`broadcasst` acción](#send-android-broadcast)y [`http` acción](#send-http-request)
para más detalles.

### Abrir sitio web/aplicación

*Soportado en:* :material-android: :material-manzana: :material-firefox:

El `view` acción **abre un sitio web o una aplicación cuando se pulsa el botón de acción**, por ejemplo, un navegador, una ubicación de Google Maps, o
incluso un enlace profundo a Twitter o un tema de show ntfy. La forma exacta en que se maneja la acción depende de cómo Android y su
navegador de escritorio tratar los enlaces. Normalmente solo abrirá un enlace en el navegador.

Ejemplos:

*   `http://` o `https://` abrirá su navegador (o una aplicación si se registró para una URL)
*   `mailto:` los enlaces abrirán su aplicación de correo, por ejemplo. `mailto:phil@example.com`
*   `geo:` los enlaces abrirán Google Maps, por ejemplo. `geo:0,0?q=1600+Amphitheatre+Parkway,+Mountain+View,+CA`
*   `ntfy://` los enlaces se abrirán ntfy (consulte [ntfy:// enlaces](subscribe/phone.md#ntfy-links)), por ejemplo, `ntfy://ntfy.sh/stats`
*   `twitter://` los enlaces abrirán Twitter, por ejemplo. `twitter://user?screen_name=..`
*   ...

Aquí hay un ejemplo usando el [`X-Actions` encabezado](#using-a-header):

\=== "Línea de comandos (curl)"
`     curl \         -d "Somebody retweetet your tweet." \         -H "Actions: view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" \
    ntfy.sh/myhome
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions="view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" \
        myhome \
        "Somebody retweetet your tweet."
    `

\=== "HTTP"
''' http
POST /myhome HTTP/1.1
Anfitrión: ntfy.sh
Acciones: ver, abrir Twitter https://twitter.com/binwiederhier/status/1467633927951163392

    Somebody retweetet your tweet.
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'Somebody retweetet your tweet.',
        headers: { 
            'Actions': 'view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392' 
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("Somebody retweetet your tweet."))
    req.Header.Set("Actions", "view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/myhome"
    $headers = @{ Actions="view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" }
    $body = "Somebody retweetet your tweet."
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/myhome",
        data="Somebody retweetet your tweet.",
        headers={ "Actions": "view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: view, Open Twitter, https://twitter.com/binwiederhier/status/1467633927951163392",
            'content' => 'Somebody retweetet your tweet.'
        ]
    ]));
    `

Y el mismo ejemplo usando [Publicación JSON](#publish-as-json):

\=== "Línea de comandos (curl)"
`     curl ntfy.sh \       -d '{
        "topic": "myhome",
        "message": "Somebody retweetet your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
      }'
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions '[
            {
                "action": "view",
                "label": "Open Twitter",
                "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
            }
        ]' \
        myhome \
        "Somebody retweetet your tweet."
    `

\=== "HTTP"
''' http
POST / HTTP/1.1
Anfitrión: ntfy.sh

    {
        "topic": "myhome",
        "message": "Somebody retweetet your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
    }
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message": "Somebody retweetet your tweet.",
            actions: [
                {
                    action: "view",
                    label: "Open Twitter",
                    url: "https://twitter.com/binwiederhier/status/1467633927951163392"
                }
            ]
        })
    })
    `

\=== "Go"
''' ir
Probablemente deberías usar json. Marshal() en su lugar y hacer una estructura adecuada,
pero por el bien del ejemplo, esto es más fácil.

    body := `{
        "topic": "myhome",
        "message": "Somebody retweetet your tweet.",
        "actions": [
          {
            "action": "view",
            "label": "Open Twitter",
            "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh"
    $body = @{
        "topic"="myhome"
        "message"="Somebody retweetet your tweet."
        "actions"=@(
            @{
                "action"="view"
                "label"="Open Twitter"
                "url"="https://twitter.com/binwiederhier/status/1467633927951163392"
            }
        )
    } | ConvertTo-Json
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -ContentType "application/json" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "Somebody retweetet your tweet.",
            "actions": [
                {
                    "action": "view",
                    "label": "Open Twitter",
                    "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
                }
            ]
        })
    )
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "Somebody retweetet your tweet.",
                "actions": [                     [
                        "action": "view",
                        "label": "Open Twitter",
                        "url": "https://twitter.com/binwiederhier/status/1467633927951163392"
                    ]
                ]
            ])
        ]
    ]));
    `

El `view` la acción admite los siguientes campos:

| | de campo | requerido Tipo | | predeterminada Ejemplo | Descripción |
|----------|----------|-----------|---------|-----------------------|--------------------------------------------------|
| `action` | ✔️       | *cuerda*  | -       | `view`                | Tipo de acción (**debe ser `view`**)                 |
| `label`  | ✔️       | *cuerda*  | -       | `Turn on light`       | Etiqueta del botón de acción en el | de notificación
| `url`    | ✔️       | *URL*     | -       | `https://example.com` | URL que se abrirá cuando se toque la acción |
| `clear`  | -️       | *booleano* | `false` | `true`                | Borrar notificación después de tocar el botón de acción |

### Enviar difusión de Android

*Soportado en:* :material-androide:

El `broadcast` acción **envía un [Transmisión de Android](https://developer.android.com/guide/components/broadcasts) intento
Cuando se pulsa el botón de acción**. Esto permite la integración en aplicaciones de automatización como [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid)
o [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm), que básicamente significa
puedes hacer todo lo que tu teléfono es capaz de hacer. Los ejemplos incluyen tomar fotos, iniciar / matar aplicaciones, cambiar de dispositivo
configuración, archivos de escritura/lectura, etc.

De forma predeterminada, la acción de intención **`io.heckel.ntfy.USER_ACTION`** se transmite, aunque esto se puede cambiar con el `intent` parámetro (véase más adelante).
Para enviar extras, utilice el botón `extras` parámetro. Actualmente **Solo se admiten extras de cadena**.

!!! información
Si no tienes idea de qué es esto, echa un vistazo a la [aplicaciones de automatización](subscribe/phone.md#automation-apps) sección, que muestra
cómo integrar Tasker y MacroDroid **con capturas de pantalla**. La integración del botón de acción es idéntica, excepto que
tienes que usar **la acción de intención `io.heckel.ntfy.USER_ACTION`** en lugar de.

Aquí hay un ejemplo usando el [`X-Actions` encabezado](#using-a-header):

\=== "Línea de comandos (curl)"
`     curl \         -d "Your wife requested you send a picture of yourself." \         -H "Actions: broadcast, Take picture, extras.cmd=pic, extras.camera=front" \
    ntfy.sh/wifey
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions="broadcast, Take picture, extras.cmd=pic, extras.camera=front" \
        wifey \
        "Your wife requested you send a picture of yourself."
    `

\=== "HTTP"
''' http
POST /wifey HTTP/1.1
Anfitrión: ntfy.sh
Acciones: broadcast, Take picture, extras.cmd=pic, extras.camera=front

    Your wife requested you send a picture of yourself.
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/wifey', {
        method: 'POST',
        body: 'Your wife requested you send a picture of yourself.',
        headers: { 
            'Actions': 'broadcast, Take picture, extras.cmd=pic, extras.camera=front' 
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/wifey", strings.NewReader("Your wife requested you send a picture of yourself."))
    req.Header.Set("Actions", "broadcast, Take picture, extras.cmd=pic, extras.camera=front")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/wifey"
    $headers = @{ Actions="broadcast, Take picture, extras.cmd=pic, extras.camera=front" }
    $body = "Your wife requested you send a picture of yourself."
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/wifey",
        data="Your wife requested you send a picture of yourself.",
        headers={ "Actions": "broadcast, Take picture, extras.cmd=pic, extras.camera=front" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/wifey', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: broadcast, Take picture, extras.cmd=pic, extras.camera=front",
            'content' => 'Your wife requested you send a picture of yourself.'
        ]
    ]));
    `

Y el mismo ejemplo usando [Publicación JSON](#publish-as-json):

\=== "Línea de comandos (curl)"
`     curl ntfy.sh \       -d '{
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
      }'
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions '[
            {
                "action": "broadcast",
                "label": "Take picture",
                "extras": {
                    "cmd": "pic",
                    "camera": "front"
                }
            }
        ]' \
        wifey \
        "Your wife requested you send a picture of yourself."
    `

\=== "HTTP"
''' http
POST / HTTP/1.1
Anfitrión: ntfy.sh

    {
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
    }
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "wifey",
            message": "Your wife requested you send a picture of yourself.",
            actions: [
                {
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": {
                        "cmd": "pic",
                        "camera": "front"
                    }
                }
            ]
        })
    })
    `

\=== "Go"
''' ir
Probablemente deberías usar json. Marshal() en su lugar y hacer una estructura adecuada,
pero por el bien del ejemplo, esto es más fácil.

    body := `{
        "topic": "wifey",
        "message": "Your wife requested you send a picture of yourself.",
        "actions": [
          {
            "action": "broadcast",
            "label": "Take picture",
            "extras": {
                "cmd": "pic",
                "camera": "front"
            }
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh"
    $body = @{
        "topic"="wifey"
        "message"="Your wife requested you send a picture of yourself."
        "actions"=@(
            @{
                "action"="broadcast"
                "label"="Take picture"
                "extras"=@{
                    "cmd"="pic"
                    "camera"="front"
                }
            }
        )
    } | ConvertTo-Json
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -ContentType "application/json" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "wifey",
            "message": "Your wife requested you send a picture of yourself.",
            "actions": [
                {
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": {
                        "cmd": "pic",
                        "camera": "front"
                    }
                }
            ]
        })
    )
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "wifey",
                "message": "Your wife requested you send a picture of yourself.",
                "actions": [                     [
                    "action": "broadcast",
                    "label": "Take picture",
                    "extras": [
                        "cmd": "pic",
                        "camera": "front"
                    ]
                ]
            ])
        ]
    ]));
    `

El `broadcast` la acción admite los siguientes campos:

| | de campo | requerido Tipo | | predeterminada Ejemplo | Descripción |
|----------|----------|------------------|------------------------------|-------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `action` | ✔️       | *cuerda*         | -                            | `broadcast`             | Tipo de acción (**debe ser `broadcast`**)                                                                                                                                                  |
| `label`  | ✔️       | *cuerda*         | -                            | `Turn on light`         | Etiqueta del botón de acción en el | de notificación
| `intent` | -️       | *cuerda*         | `io.heckel.ntfy.USER_ACTION` | `com.example.AN_INTENT` | Nombre de intención de Android, **el valor predeterminado es `io.heckel.ntfy.USER_ACTION`**                                                                                                                       |
| `extras` | -️       | *mapa de cadenas* | -                            | *ver más arriba*             | Extras de intención de Android. Actualmente, solo se admiten extras de cadena. Al publicar como JSON, los extras se pasan como un mapa. Cuando se utiliza el formato simple, use `extras.<param>=<value>`. |
| `clear`  | -️       | *booleano*        | `false`                      | `true`                  | Borrar notificación después de tocar el botón de acción |

### Enviar solicitud HTTP

*Soportado en:* :material-android: :material-manzana: :material-firefox:

El `http` acción **envía una solicitud HTTP cuando se pulsa el botón de acción**. Puede usar esto para desencadenar las API de REST
para cualquier sistema que tenga, por ejemplo, abrir la puerta del garaje o encender / apagar las luces.

De forma predeterminada, esta acción envía un **Solicitud POST** (¡no GET!), aunque esto se puede cambiar con el `method` parámetro.
El único parámetro requerido es `url`. Los encabezados se pueden pasar a lo largo utilizando el `headers` parámetro.

Aquí hay un ejemplo usando el [`X-Actions` encabezado](#using-a-header):

\=== "Línea de comandos (curl)"
`     curl \         -d "Garage door has been open for 15 minutes. Close it?" \         -H "Actions: http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" \
        ntfy.sh/myhome
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions="http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" \
        myhome \
        "Garage door has been open for 15 minutes. Close it?"
    `

\=== "HTTP"
''' http
POST /myhome HTTP/1.1
Anfitrión: ntfy.sh
Acciones: http, Cerrar puerta, https://api.mygarage.lan/, método=PUT, encabezados. Authorization=Bearer zAzsx1sk.., body={"action": "close"}

    Garage door has been open for 15 minutes. Close it?
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/myhome', {
        method: 'POST',
        body: 'Garage door has been open for 15 minutes. Close it?',
        headers: { 
            'Actions': 'http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}' 
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/myhome", strings.NewReader("Garage door has been open for 15 minutes. Close it?"))
    req.Header.Set("Actions", "http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/myhome"
    $headers = @{ Actions="http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" }
    $body = "Garage door has been open for 15 minutes. Close it?"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/myhome",
        data="Garage door has been open for 15 minutes. Close it?",
        headers={ "Actions": "http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Actions: http, Close door, https://api.mygarage.lan/, method=PUT, headers.Authorization=Bearer zAzsx1sk.., body={\"action\": \"close\"}",
            'content' => 'Garage door has been open for 15 minutes. Close it?'
        ]
    ]));
    `

Y el mismo ejemplo usando [Publicación JSON](#publish-as-json):

\=== "Línea de comandos (curl)"
`     curl ntfy.sh \       -d '{
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "url": "https://api.mygarage.lan/",
            "method": "PUT",
            "headers": {
                "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
      }'
    `

\=== "ntfy CLI"
`     ntfy publish \         --actions '[
            {
              "action": "http",
              "label": "Close door",
              "url": "https://api.mygarage.lan/",
              "method": "PUT",
              "headers": {
                "Authorization": "Bearer zAzsx1sk.."
              },
              "body": "{\"action\": \"close\"}"
            }
        ]' \
        myhome \
        "Garage door has been open for 15 minutes. Close it?"
    `

\=== "HTTP"
''' http
POST / HTTP/1.1
Anfitrión: ntfy.sh

    {
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "url": "https://api.mygarage.lan/",
            "method": "PUT",
            "headers": {
              "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
    }
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh', {
        method: 'POST',
        body: JSON.stringify({
            topic: "myhome",
            message": "Garage door has been open for 15 minutes. Close it?",
            actions: [
              {
                "action": "http",
                "label": "Close door",
                "url": "https://api.mygarage.lan/",
                "method": "PUT",
                "headers": {
                  "Authorization": "Bearer zAzsx1sk.."
                },
                "body": "{\"action\": \"close\"}"
              }
            ]
        })
    })
    `

\=== "Go"
''' ir
Probablemente deberías usar json. Marshal() en su lugar y hacer una estructura adecuada,
pero por el bien del ejemplo, esto es más fácil.

    body := `{
        "topic": "myhome",
        "message": "Garage door has been open for 15 minutes. Close it?",
        "actions": [
          {
            "action": "http",
            "label": "Close door",
            "method": "PUT",
            "url": "https://api.mygarage.lan/",
            "headers": {
              "Authorization": "Bearer zAzsx1sk.."
            },
            "body": "{\"action\": \"close\"}"
          }
        ]
    }`
    req, _ := http.NewRequest("POST", "https://ntfy.sh/", strings.NewReader(body))
    http.DefaultClient.Do(req)
    ```

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh"
    $body = @{
        "topic"="myhome"
        "message"="Garage door has been open for 15 minutes. Close it?"
        "actions"=@(
            @{
                "action"="http",
                "label"="Close door"
                "url"="https://api.mygarage.lan/"
                "method"="PUT"
                "headers"=@{
                  "Authorization"="Bearer zAzsx1sk.."
                }
                "body"="{\"action\": \"close\"}"
            }
          }
        )
    } | ConvertTo-Json
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -ContentType "application/json" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/",
        data=json.dumps({
            "topic": "myhome",
            "message": "Garage door has been open for 15 minutes. Close it?",
            "actions": [
                {
                  "action": "http",
                  "label": "Close door",
                  "url": "https://api.mygarage.lan/",
                  "method": "PUT",
                  "headers": {
                    "Authorization": "Bearer zAzsx1sk.."
                  },
                  "body": "{\"action\": \"close\"}"
                }
            ]
        })
    )
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json",
            'content' => json_encode([
                "topic": "myhome",
                "message": "Garage door has been open for 15 minutes. Close it?",
                "actions": [                     [
                        "action": "http",
                        "label": "Close door",
                        "url": "https://api.mygarage.lan/",
                        "method": "PUT",
                        "headers": [
                            "Authorization": "Bearer zAzsx1sk.."
                         ],
                        "body": "{\"action\": \"close\"}"
                    ]
                ]
            ])
        ]
    ]));
    `

El `http` la acción admite los siguientes campos:

| | de campo | requerido Tipo | | predeterminada Ejemplo | Descripción |
|-----------|----------|--------------------|-----------|---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `action`  | ✔️       | *cuerda*           | -         | `http`                    | Tipo de acción (**debe ser `http`**)                                                                                                                        |
| `label`   | ✔️       | *cuerda*           | -         | `Open garage door`        | Etiqueta del botón de acción en el | de notificación
| `url`     | ✔️       | *cuerda*           | -         | `https://ntfy.sh/mytopic` | URL a la que se enviará la solicitud HTTP |
| `method`  | -️       | *GET/POST/PUT/...* | `POST` ⚠️ | `GET`                     | Método HTTP para usar para la solicitud, **el valor predeterminado es POST** ⚠️                                                                                                  |
| `headers` | -️       | *mapa de cadenas*   | -         | *ver más arriba*               | Encabezados HTTP para pasar la solicitud. Al publicar como JSON, los encabezados se pasan como un mapa. Cuando se utiliza el formato simple, use `headers.<header1>=<value>`. |
| `body`    | -️       | *cuerda*           | *vacío*   | `some body, somebody?`    | | de cuerpo HTTP
| `clear`   | -️       | *booleano*          | `false`   | `true`                    | Borrar notificación después de que la solicitud HTTP se realice correctamente. Si la solicitud falla, la notificación no se borra.                                                  |

## Haga clic en la acción

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puede definir qué URL abrir cuando se hace clic en una notificación. Esto puede ser útil si su notificación está relacionada
a una alerta de Zabbix o una transacción para la que le gustaría proporcionar el enlace profundo. Al tocar la notificación se abrirá
el navegador web (o la aplicación) y abra el sitio web.

Para definir una acción de clic para la notificación, pase una dirección URL como el valor de la `X-Click` encabezado (o su alias `Click`).
Si pasa la URL de un sitio web (`http://` o `https://`) se abrirá el navegador web. Si pasa otro URI que se puede controlar
por otra aplicación, la aplicación responsable puede abrirse.

Ejemplos:

*   `http://` o `https://` abrirá su navegador (o una aplicación si se registró para una URL)
*   `mailto:` los enlaces abrirán su aplicación de correo, por ejemplo. `mailto:phil@example.com`
*   `geo:` los enlaces abrirán Google Maps, por ejemplo. `geo:0,0?q=1600+Amphitheatre+Parkway,+Mountain+View,+CA`
*   `ntfy://` los enlaces se abrirán ntfy (consulte [ntfy:// enlaces](subscribe/phone.md#ntfy-links)), por ejemplo, `ntfy://ntfy.sh/stats`
*   `twitter://` los enlaces abrirán Twitter, por ejemplo. `twitter://user?screen_name=..`
*   ...

Aquí hay un ejemplo que abrirá Reddit cuando se haga clic en la notificación:

\=== "Línea de comandos (curl)"
`     curl \         -d "New messages on Reddit" \         -H "Click: https://www.reddit.com/message/messages" \
        ntfy.sh/reddit_alerts
    `

\=== "ntfy CLI"
`     ntfy publish \         --click="https://www.reddit.com/message/messages" \
        reddit_alerts "New messages on Reddit"
    `

\=== "HTTP"
''' http
POST /reddit_alerts HTTP/1.1
Anfitrión: ntfy.sh
Clic: https://www.reddit.com/message/messages

    New messages on Reddit
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/reddit_alerts', {
        method: 'POST',
        body: 'New messages on Reddit',
        headers: { 'Click': 'https://www.reddit.com/message/messages' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/reddit_alerts", strings.NewReader("New messages on Reddit"))
    req.Header.Set("Click", "https://www.reddit.com/message/messages")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/reddit_alerts"
    $headers = @{ Click="https://www.reddit.com/message/messages" }
    $body = "New messages on Reddit"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/reddit_alerts",
        data="New messages on Reddit",
        headers={ "Click": "https://www.reddit.com/message/messages" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/reddit_alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Click: https://www.reddit.com/message/messages",
            'content' => 'New messages on Reddit'
        ]
    ]));
    `

## Accesorios

*Soportado en:* :material-android: :material-firefox:

Puedes **enviar imágenes y otros archivos a tu teléfono** como datos adjuntos a una notificación. A continuación, se descargan los archivos adjuntos
en su teléfono (dependiendo del tamaño y la configuración automáticamente), y se puede usar desde la carpeta Descargas.

Hay dos formas diferentes de enviar archivos adjuntos:

*   envío [un archivo local](#attach-local-file) a través de PUT, por ejemplo, desde `~/Flowers/flower.jpg` o `ringtone.mp3`
*   o por [pasar una URL externa](#attach-file-from-a-url) como archivo adjunto, por ejemplo, `https://f-droid.org/F-Droid.apk`

### Adjuntar archivo local

Para **enviar un archivo desde el equipo** como archivo adjunto, puede enviarlo como el cuerpo de solicitud PUT. Si un mensaje es mayor
que el tamaño máximo del mensaje (4.096 bytes) o que consta de caracteres que no sean UTF-8, el servidor ntfy se activará automáticamente
detecte el tipo y el tamaño mime y envíe el mensaje como un archivo adjunto. Para enviar mensajes o archivos de solo texto más pequeños
Como datos adjuntos, debe pasar un nombre de archivo pasando el `X-Filename` encabezado o parámetro de consulta (o cualquiera de sus alias
`Filename`, `File` o `f`).

De forma predeterminada, y cómo se configura ntfy.sh, el **El tamaño máximo de los archivos adjuntos es de 15 MB** (con 100 MB en total por visitante).
Accesorios **caducan después de 3 horas**, que normalmente es mucho tiempo para que el usuario lo descargue, o para la aplicación de Android
para descargarlo automáticamente. Por favor, echa un vistazo también a la [otros límites a continuación](#limitations).

Aquí hay un ejemplo que muestra cómo cargar una imagen:

\=== "Línea de comandos (curl)"
`     curl \         -T flower.jpg \         -H "Filename: flower.jpg" \
        ntfy.sh/flowers
    `

\=== "ntfy CLI"
`     ntfy publish \         --file=flower.jpg \
        flowers
    `

\=== "HTTP"
''' http
PUT /flores HTTP/1.1
Anfitrión: ntfy.sh
Nombre del archivo: flor.jpg
Tipo de contenido: 52312

    <binary JPEG data>
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/flowers', {
        method: 'PUT',
        body: document.getElementById("file").files[0],
        headers: { 'Filename': 'flower.jpg' }
    })
    `

\=== "Go"
` go
    file, _ := os.Open("flower.jpg")
    req, _ := http.NewRequest("PUT", "https://ntfy.sh/flowers", file)
    req.Header.Set("Filename", "flower.jpg")
    http.DefaultClient.Do(req)
    `

\=== "Python"
` python
    requests.put("https://ntfy.sh/flowers",
        data=open("flower.jpg", 'rb'),
        headers={ "Filename": "flower.jpg" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/flowers', false, stream_context_create([
        'http' => [
            'method' => 'PUT',
            'header' =>
                "Content-Type: application/octet-stream\r\n" . // Does not matter
                "Filename: flower.jpg",
            'content' => file_get_contents('flower.jpg') // Dangerous for large files 
        ]
    ]));
    `

Así es como se ve en Android:

<figure markdown>
  ![image attachment](static/img/android-screenshot-attachment-image.png){ width=500 }
  <figcaption>Image attachment sent from a local file</figcaption>
</figure>

### Adjuntar archivo desde una URL

En lugar de enviar un archivo local a su teléfono, puede usar **una URL externa** para especificar dónde se hospedan los datos adjuntos.
Esto podría ser un enlace de Dropbox, un archivo de las redes sociales o cualquier otra URL disponible públicamente. Ya que los archivos son
alojado externamente, los límites de caducidad o tamaño de arriba no se aplican aquí.

Para adjuntar un archivo externo, simplemente pase el `X-Attach` encabezado o parámetro de consulta (o cualquiera de sus alias `Attach` o `a`)
para especificar la dirección URL de los datos adjuntos. Puede ser cualquier tipo de archivo.

Ntfy intentará derivar automáticamente el nombre del archivo de la URL (por ejemplo, `https://example.com/flower.jpg` producirá un
Nombre `flower.jpg`). Para invalidar este nombre de archivo, puede enviar el `X-Filename` encabezado o parámetro de consulta (o cualquiera de sus
Alias `Filename`, `File` o `f`).

Aquí hay un ejemplo que muestra cómo adjuntar un archivo APK:

\=== "Línea de comandos (curl)"
`     curl \         -X POST \         -H "Attach: https://f-droid.org/F-Droid.apk" \
        ntfy.sh/mydownloads
    `

\=== "ntfy CLI"
`     ntfy publish \         --attach="https://f-droid.org/F-Droid.apk" \
        mydownloads
    `

\=== "HTTP"
` http
    POST /mydownloads HTTP/1.1
    Host: ntfy.sh
    Attach: https://f-droid.org/F-Droid.apk
    `

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mydownloads', {
        method: 'POST',
        headers: { 'Attach': 'https://f-droid.org/F-Droid.apk' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mydownloads", file)
    req.Header.Set("Attach", "https://f-droid.org/F-Droid.apk")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/mydownloads"
    $headers = @{ Attach="https://f-droid.org/F-Droid.apk" }
    Invoke-RestMethod -Method 'Post' -Uri $uri -Headers $headers -UseBasicParsing
    `

\=== "Python"
` python
    requests.put("https://ntfy.sh/mydownloads",
        headers={ "Attach": "https://f-droid.org/F-Droid.apk" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mydownloads', false, stream_context_create([
        'http' => [
        'method' => 'PUT',
        'header' =>
            "Content-Type: text/plain\r\n" . // Does not matter
            "Attach: https://f-droid.org/F-Droid.apk",
        ]
    ]));
    `

<figure markdown>
  ![file attachment](static/img/android-screenshot-attachment-file.png){ width=500 }
  <figcaption>File attachment sent from an external URL</figcaption>
</figure>

## Notificaciones por correo electrónico

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puede reenviar mensajes al correo electrónico especificando una dirección en el encabezado. Esto puede ser útil para mensajes que
le gustaría persistir más tiempo o notificarse a sí mismo en todos los canales posibles.

El uso es fácil: simplemente pase el `X-Email` encabezado (o cualquiera de sus alias: `X-E-mail`, `Email`, `E-mail`, `Mail`o `e`).
Solo se admite una dirección de correo electrónico.

Dado que ntfy no proporciona autenticación (todavía), la limitación de velocidad es bastante estricta (consulte [Limitaciones](#limitations)). En
configuración predeterminada, obtienes **16 correos electrónicos por visitante** (dirección IP) y luego de eso uno por hora. Además de
es decir, su dirección IP aparece en el cuerpo del correo electrónico. Esto es para prevenir el abuso.

\=== "Línea de comandos (curl)"
`     curl \         -H "Email: phil@example.com" \         -H "Tags: warning,skull,backup-host,ssh-login" \         -H "Priority: high" \         -d "Unknown login from 5.31.23.83 to backups.example.com" \
        ntfy.sh/alerts
    curl -H "Email: phil@example.com" -d "You've Got Mail" 
    curl -d "You've Got Mail" "ntfy.sh/alerts?email=phil@example.com"
    `

\=== "ntfy CLI"
`     ntfy publish \         --email=phil@example.com \         --tags=warning,skull,backup-host,ssh-login \         --priority=high \
        alerts "Unknown login from 5.31.23.83 to backups.example.com"
    `

\=== "HTTP"
''' http
POST /alertas HTTP/1.1
Anfitrión: ntfy.sh
Correo electrónico: phil@example.com
Etiquetas: warning,skull,backup-host,ssh-login
Prioridad: alta

    Unknown login from 5.31.23.83 to backups.example.com
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/alerts', {
        method: 'POST',
        body: "Unknown login from 5.31.23.83 to backups.example.com",
        headers: { 
            'Email': 'phil@example.com',
            'Tags': 'warning,skull,backup-host,ssh-login',
            'Priority': 'high'
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/alerts", 
        strings.NewReader("Unknown login from 5.31.23.83 to backups.example.com"))
    req.Header.Set("Email", "phil@example.com")
    req.Header.Set("Tags", "warning,skull,backup-host,ssh-login")
    req.Header.Set("Priority", "high")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/alerts"
    $headers = @{ Title"="Low disk space alert"
                  Priority="high"
                  Tags="warning,skull,backup-host,ssh-login")
                  Email="phil@example.com" }
    $body = "Unknown login from 5.31.23.83 to backups.example.com"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/alerts",
        data="Unknown login from 5.31.23.83 to backups.example.com",
        headers={ 
            "Email": "phil@example.com",
            "Tags": "warning,skull,backup-host,ssh-login",
            "Priority": "high"
        })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/alerts', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Email: phil@example.com\r\n" .
                "Tags: warning,skull,backup-host,ssh-login\r\n" .
                "Priority: high",
            'content' => 'Unknown login from 5.31.23.83 to backups.example.com'
        ]
    ]));
    `

Así es como se ve en Google Mail:

<figure markdown>
  ![e-mail notification](static/img/screenshot-email.png){ width=600 }
  <figcaption>E-mail notification</figcaption>
</figure>

## Publicación de correo electrónico

*Soportado en:* :material-android: :material-manzana: :material-firefox:

Puede publicar mensajes en un tema por correo electrónico, es decir, enviando un correo electrónico a una dirección específica. Por ejemplo, puede
Publicar un mensaje en el tema `sometopic` enviando un correo electrónico a `ntfy-sometopic@ntfy.sh`. Esto es útil para el correo electrónico
integraciones basadas, como para statuspage.io (aunque en estos días la mayoría de los servicios también admiten webhooks y llamadas HTTP).

Dependiendo de la [configuración del servidor](config.md#e-mail-publishing), el formato de dirección de correo electrónico puede tener un prefijo para
evitar el spam en los temas. Para ntfy.sh, el prefijo se configura en `ntfy-`, lo que significa que la dirección de correo electrónico general
el formato es:

    ntfy-$topic@ntfy.sh

A partir de hoy, la publicación de correo electrónico solo admite la adición de un [título del mensaje](#message-title) (el asunto del correo electrónico). Etiquetas, prioridad,
retraso y otras características no son compatibles (todavía). Aquí hay un ejemplo que publicará un mensaje con el
título `You've Got Mail` al tema `sometopic` (ver [ntfy.sh/sometopic](https://ntfy.sh/sometopic)):

<figure markdown>
  ![e-mail publishing](static/img/screenshot-email-publishing-gmail.png){ width=500 }
  <figcaption>Publishing a message via e-mail</figcaption>
</figure>

## Funciones avanzadas

### Autenticación

Dependiendo de si el servidor está configurado para admitir [control de acceso](config.md#access-control), algunos temas
puede estar protegido contra lectura/escritura para que solo los usuarios con las credenciales correctas puedan suscribirse o publicar en ellos.
Para publicar/suscribirse a temas protegidos, puede utilizar [Autenticación básica](https://en.wikipedia.org/wiki/Basic_access_authentication)
con un nombre de usuario/contraseña válido. Para su servidor autohospedado, **Asegúrese de usar HTTPS para evitar escuchas** y exponer
su contraseña.

Aquí hay un ejemplo simple:

\=== "Línea de comandos (curl)"
`     curl \       -u phil:mypass \       -d "Look ma, with auth" \
      https://ntfy.example.com/mysecrets
    `

\=== "ntfy CLI"
`     ntfy publish \       -u phil:mypass \
      ntfy.example.com/mysecrets \
      "Look ma, with auth"
    `

\=== "HTTP"
''' http
POST /mysecrets HTTP/1.1
Anfitrión: ntfy.example.com
Autorización: Basic cGhpbDpteXBhc3M=

    Look ma, with auth
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.example.com/mysecrets', {
        method: 'POST', // PUT works too
        body: 'Look ma, with auth',
        headers: {
            'Authorization': 'Basic cGhpbDpteXBhc3M='
        }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.example.com/mysecrets",
    strings.NewReader("Look ma, with auth"))
    req.Header.Set("Authorization", "Basic cGhpbDpteXBhc3M=")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.example.com/mysecrets"
    $credentials = 'username:password'
    $encodedCredentials = [convert]::ToBase64String([text.Encoding]::UTF8.GetBytes($credentials))
    $headers = @{Authorization="Basic $encodedCredentials"}
    $message = "Look ma, with auth"
    Invoke-RestMethod -Uri $uri -Body $message -Headers $headers -Method "Post" -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.example.com/mysecrets",
    data="Look ma, with auth",
    headers={
        "Authorization": "Basic cGhpbDpteXBhc3M="
    })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.example.com/mysecrets', false, stream_context_create([
        'http' => [
            'method' => 'POST', // PUT also works
            'header' =>
                'Content-Type: text/plain\r\n' .
                'Authorization: Basic cGhpbDpteXBhc3M=',
            'content' => 'Look ma, with auth'
        ]
    ]));
    `

### Almacenamiento en caché de mensajes

!!! información
Si `Cache: no` se utiliza, los mensajes solo se entregarán a los suscriptores conectados y no se volverán a entregar si un
el cliente se vuelve a conectar. Si un suscriptor tiene problemas de red (temporales) o se está reconectando momentáneamente,
**Es posible que se pierdan mensajes**.

De forma predeterminada, el servidor ntfy almacena en caché los mensajes en el disco durante 12 horas (consulte [almacenamiento en caché de mensajes](config.md#message-cache)), por lo que
todos los mensajes que publique se almacenan en el lado del servidor durante un tiempo. La razón de esto es superar lo temporal.
interrupciones de la red del lado del cliente, pero podría decirse que esta característica también puede plantear problemas de privacidad.

Para evitar que los mensajes se almacenen en caché por completo en el lado del servidor, puede configurar `X-Cache` encabezado (o su alias: `Cache`) a `no`.
Esto asegurará que su mensaje no esté almacenado en caché en el servidor, incluso si el almacenamiento en caché del lado del servidor está habilitado. Mensajes
se siguen entregando a los suscriptores conectados, pero [`since=`](subscribe/api.md#fetch-cached-messages) y
[`poll=1`](subscribe/api.md#poll-for-messages) ya no devolverá el mensaje.

\=== "Línea de comandos (curl)"
`     curl -H "X-Cache: no" -d "This message won't be stored server-side" ntfy.sh/mytopic
    curl -H "Cache: no" -d "This message won't be stored server-side" ntfy.sh/mytopic
    `

\=== "ntfy CLI"
`     ntfy publish \         --no-cache \
        mytopic "This message won't be stored server-side"
    `

\=== "HTTP"
''' http
POST /mytopic HTTP/1.1
Anfitrión: ntfy.sh
Caché: no

    This message won't be stored server-side
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mytopic', {
        method: 'POST',
        body: 'This message won't be stored server-side',
        headers: { 'Cache': 'no' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mytopic", strings.NewReader("This message won't be stored server-side"))
    req.Header.Set("Cache", "no")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/mytopic"
    $headers = @{ Cache="no" }
    $body = "This message won't be stored server-side"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -Headers $headers -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/mytopic",
        data="This message won't be stored server-side",
        headers={ "Cache": "no" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Cache: no",
            'content' => 'This message won't be stored server-side'
        ]
    ]));
    `

### Desactivar Firebase

!!! información
Si `Firebase: no` se utiliza y [entrega instantánea](subscribe/phone.md#instant-delivery) no está habilitado en Android
aplicación (solo variante de Google Play), **la entrega de mensajes se retrasará significativamente (hasta 15 minutos)**. Para superar
este retraso, simplemente permite la entrega instantánea.

El servidor ntfy se puede configurar para utilizar [Mensajería en la nube de Firebase (FCM)](https://firebase.google.com/docs/cloud-messaging)
(ver [Configuración de Firebase](config.md#firebase-fcm)) para la entrega de mensajes en Android (para minimizar la huella de la batería de la aplicación).
El servidor ntfy.sh está configurado de esta manera, lo que significa que todos los mensajes publicados en ntfy.sh también se publican en los correspondientes
Temas FCM.

Si quieres evitar reenviar mensajes a Firebase, puedes configurar el `X-Firebase` encabezado (o su alias: `Firebase`)
Para `no`. Esto indicará al servidor que no reenvíe mensajes a Firebase.

\=== "Línea de comandos (curl)"
`     curl -H "X-Firebase: no" -d "This message won't be forwarded to FCM" ntfy.sh/mytopic
    curl -H "Firebase: no" -d "This message won't be forwarded to FCM" ntfy.sh/mytopic
    `

\=== "ntfy CLI"
`     ntfy publish \         --no-firebase \
        mytopic "This message won't be forwarded to FCM"
    `

\=== "HTTP"
''' http
POST /mytopic HTTP/1.1
Anfitrión: ntfy.sh
Base de fuego: no

    This message won't be forwarded to FCM
    ```

\=== "JavaScript"
` javascript
    fetch('https://ntfy.sh/mytopic', {
        method: 'POST',
        body: 'This message won't be forwarded to FCM',
        headers: { 'Firebase': 'no' }
    })
    `

\=== "Go"
` go
    req, _ := http.NewRequest("POST", "https://ntfy.sh/mytopic", strings.NewReader("This message won't be forwarded to FCM"))
    req.Header.Set("Firebase", "no")
    http.DefaultClient.Do(req)
    `

\=== "PowerShell"
` powershell
    $uri = "https://ntfy.sh/mytopic"
    $headers = @{ Firebase="no" }
    $body = "This message won't be forwarded to FCM"
    Invoke-RestMethod -Method 'Post' -Uri $uri -Body $body -Headers $headers -UseBasicParsing
    `

\=== "Python"
` python
    requests.post("https://ntfy.sh/mytopic",
        data="This message won't be forwarded to FCM",
        headers={ "Firebase": "no" })
    `

\=== "PHP"
` php-inline
    file_get_contents('https://ntfy.sh/mytopic', false, stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' =>
                "Content-Type: text/plain\r\n" .
                "Firebase: no",
            'content' => 'This message won't be stored server-side'
        ]
    ]));
    `

### UnifiedPush

!!! información
Esta configuración no es relevante para los usuarios, solo para los desarrolladores de aplicaciones y las personas interesadas en [UnifiedPush](https://unifiedpush.org).

[UnifiedPush](https://unifiedpush.org) es un estándar para recibir notificaciones push sin usar la propiedad de Google
[Mensajería en la nube de Firebase (FCM)](https://firebase.google.com/docs/cloud-messaging) servicio. Pone notificaciones push
en el control del usuario. ntfy puede actuar como un **Distribuidor de UnifiedPush**, reenviar mensajes a aplicaciones que lo admitan.

Al publicar mensajes en un tema, las aplicaciones que usan ntfy como distribuidor de UnifiedPush pueden establecer el `X-UnifiedPush` encabezado o consulta
parámetro (o cualquiera de sus alias `unifiedpush` o `up`) a `1` Para [deshabilitar Firebase](#disable-firebase). A partir de hoy, este
la opción es en su mayoría equivalente a `Firebase: no`, pero se introdujo para permitir la flexibilidad futura. La bandera adicionalmente
permite la detección automática de la codificación de mensajes. Si el mensaje es binario, se codificará como base64.

### Puerta de enlace Matrix

El servidor ntfy implementa un [Puerta de enlace push de matriz](https://spec.matrix.org/v1.2/push-gateway-api/) (en combinación con
[UnifiedPush](https://unifiedpush.org) como el [Protocolo push del proveedor](https://unifiedpush.org/developers/gateway/)). Esto facilita la integración
con autohospedado [Matriz](https://matrix.org/) servidores (como [sinapsis](https://github.com/matrix-org/synapse)), ya que
No es necesario configurar un proxy de inserción independiente (como [proxies comunes](https://github.com/UnifiedPush/common-proxies)).

En resumen, ntfy acepta mensajes de Matrix en el `/_matrix/push/v1/notify` punto final (consulte [API de puerta de enlace push](https://spec.matrix.org/v1.2/push-gateway-api/)),
y los reenvía al tema ntfy definido en el cuadro `pushkey` del mensaje. A continuación, el mensaje se reenviará al
ntfy aplicación de Android, y pasó al cliente Matrix allí.

Hay un buen diagrama en el [Documentos de Push Gateway](https://spec.matrix.org/v1.2/push-gateway-api/). En este diagrama, el
El servidor ntfy desempeña el papel de la puerta de enlace push, así como el proveedor de inserción. UnifiedPush es el protocolo push del proveedor.

!!! información
Esta no es una puerta de enlace push de matriz genérica. Solo funciona en combinación con UnifiedPush y ntfy.

## Temas públicos

Obviamente, todos los temas de ntfy.sh son públicos, pero hay algunos temas designados que se utilizan en ejemplos y temas.
que puedes usar para probar qué [autenticación y control de acceso](#authentication) Parece.

| Tema | | de usuario Permisos | Descripción |
|------------------------------------------------|-----------------------------------|------------------------------------------------------|--------------------------------------|
| [Anuncios](https://ntfy.sh/announcements) | `*` | (no autenticado) Solo lectura para todos los | Anuncios de lanzamiento y |
| [Estadísticas](https://ntfy.sh/stats)                 | `*` | (no autenticado) Solo lectura para todos los | Estadísticas diarias sobre ntfy.sh uso |
| [mytopic-rw](https://ntfy.sh/mytopic-rw)       | `testuser` (contraseña: `testuser`) | Lectura-escritura para `testuser`, no hay acceso para nadie más | Tema de prueba |
| [mytopic-ro](https://ntfy.sh/mytopic-ro)       | `testuser` (contraseña: `testuser`) | Sólo lectura para `testuser`, no hay acceso para nadie más | Tema de prueba |
| [mytopic-wo](https://ntfy.sh/mytopic-wo)       | `testuser` (contraseña: `testuser`) | Solo escritura para `testuser`, no hay acceso para nadie más | Tema de prueba |

## Limitaciones

Hay algunas limitaciones en la API para evitar el abuso y mantener el servidor en buen estado. Casi todas estas configuraciones
se pueden configurar a través del lado del servidor [configuración de limitación de velocidad](config.md#rate-limiting). La mayoría de estos límites con los que no te encontrarás,
pero por si acaso, vamos a enumerarlos todos:

| Limitar | Descripción |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Longitud del mensaje**         | Cada mensaje puede tener una longitud de hasta 4.096 bytes. Los mensajes más largos se tratan como [Accesorios](#attachments).                                                                  |
| **Solicitudes**               | De forma predeterminada, el servidor está configurado para permitir 60 solicitudes por visitante a la vez y, a continuación, rellena el bucket de solicitudes permitidas a una velocidad de una solicitud por 5 segundos. |
| **Correos electrónicos**                | De forma predeterminada, el servidor está configurado para permitir el envío de 16 correos electrónicos por visitante a la vez y, a continuación, rellena el bucket de correo electrónico permitido a una velocidad de uno por hora.         |
| **Límite de suscripción**     | De forma predeterminada, el servidor permite a cada visitante mantener abiertas 30 conexiones al servidor.                                                                                    |
| **Límite de tamaño de datos adjuntos**  | De forma predeterminada, el servidor permite archivos adjuntos de hasta 15 MB de tamaño, hasta 100 MB en total por visitante y hasta 5 GB en todos los visitantes.                                     |
| **Caducidad de los datos adjuntos**      | De forma predeterminada, el servidor elimina los archivos adjuntos después de 3 horas y, por lo tanto, libera espacio del límite total de datos adjuntos de visitantes.                                             |
| **Ancho de banda de conexión**   | De forma predeterminada, el servidor permite 500 MB de tráfico GET/PUT/POST para archivos adjuntos por visitante en un período de 24 horas. Se rechaza el tráfico que exceda de eso.                        |
| **Número total de temas** | De forma predeterminada, el servidor está configurado para permitir 15.000 temas. Sin embargo, el servidor ntfy.sh tiene límites más altos.                                                                |

## Lista de todos los parámetros

A continuación se muestra una lista de todos los parámetros que se pueden pasar al publicar un mensaje. Los nombres de los parámetros son **no distingue entre mayúsculas y minúsculas**,
y se puede pasar como **Encabezados HTTP** o **parámetros de consulta en la dirección URL**. Se enumeran en la tabla en su forma canónica.

| | de parámetros Alias (sin distinción entre mayúsculas y minúsculas) | Descripción |
|-----------------|--------------------------------------------|-----------------------------------------------------------------------------------------------|
| `X-Message`     | `Message`, `m`                             | Cuerpo principal del mensaje como se muestra en el | de notificación
| `X-Title`       | `Title`, `t`                               | [Título del mensaje](#message-title)                                                               |
| `X-Priority`    | `Priority`, `prio`, `p`                    | [Prioridad del mensaje](#message-priority)                                                         |
| `X-Tags`        | `Tags`, `Tag`, `ta`                        | [Etiquetas y emojis](#tags-emojis)                                                               |
| `X-Delay`       | `Delay`, `X-At`, `At`, `X-In`, `In`        | Marca de tiempo o duración para [entrega retrasada](#scheduled-delivery)                             |
| `X-Actions`     | `Actions`, `Action`                        | Matriz JSON o formato corto de [acciones del usuario](#action-buttons)                                 |
| `X-Click`       | `Click`                                    | URL para abrir cuando [se hace clic en la notificación](#click-action)                                     |
| `X-Attach`      | `Attach`, `a`                              | URL para enviar como un [archivo adjunto](#attachments), como alternativa a PUT/POST-ing un archivo adjunto |
| `X-Filename`    | `Filename`, `file`, `f`                    | Opcional [archivo adjunto](#attachments) nombre de archivo, tal como aparece en el | de cliente
| `X-Email`       | `X-E-Mail`, `Email`, `E-Mail`, `mail`, `e` | Dirección de correo electrónico para [notificaciones por correo electrónico](#e-mail-notifications)                              |
| `X-Cache`       | `Cache`                                    | Permite deshabilitar [almacenamiento en caché de mensajes](#message-caching)                                          |
| `X-Firebase`    | `Firebase`                                 | Permite deshabilitar [enviar a Firebase](#disable-firebase)                                     |
| `X-UnifiedPush` | `UnifiedPush`, `up`                        | [UnifiedPush](#unifiedpush) opción de publicación, solo para ser utilizada por las aplicaciones de UnifiedPush |
| `X-Poll-ID`     | `Poll-ID`                                  | Parámetro interno, utilizado para [Notificaciones push de iOS](config.md#ios-instant-notifications)    |
| `Authorization` | -                                          | Si es compatible con el servidor, puede [iniciar sesión para acceder](#authentication) temas protegidos |
