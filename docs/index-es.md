# Empezar

ntfy te permite **enviar notificaciones push a su teléfono o escritorio a través de scripts desde cualquier computadora**, utilizando HTTP PUT simple
o solicitudes POST. Lo uso para notificarme a mí mismo cuando los scripts fallan o se completan los comandos de ejecución prolongada.

## Paso 1: Obtén la aplicación

<a href="https://play.google.com/store/apps/details?id=io.heckel.ntfy"><img src="../../static/img/badge-googleplay.png"></a> <a href="https://f-droid.org/en/packages/io.heckel.ntfy/"><img src="../../static/img/badge-fdroid.png"></a> <a href="https://apps.apple.com/us/app/ntfy/id1625396347"><img src="../../static/img/badge-appstore.png"></a>

Para [recibir notificaciones en el teléfono](subscribe/phone.md), instale la aplicación, ya sea a través de Google Play o F-Droid.
Una vez instalado, ábralo y suscríbase a un tema de su elección. Los temas no tienen que crearse explícitamente, así que solo
elija un nombre y utilícelo más tarde cuando [publicar un mensaje](publish.md). Tenga en cuenta que **los nombres de los temas son públicos, por lo que es prudente
para elegir algo que no se puede adivinar fácilmente.**

Para esta guía, solo usaremos `mytopic` como nombre de nuestro tema:

<figure markdown>
  ![adding a topic](static/img/getting-started-add.png){ width=500 }
  <figcaption>Creating/adding your first topic</figcaption>
</figure>

Eso es todo. Después de tocar "Suscribirse", la aplicación está escuchando nuevos mensajes sobre ese tema.

## Paso 2: Enviar un mensaje

Ahora vamos a [enviar un mensaje](publish.md) a nuestro tema. Es fácil en todos los idiomas, ya que solo estamos usando HTTP PUT / POST,
o con el [CLI ntfy](install.md). El mensaje está en el cuerpo de la solicitud. Aquí hay un ejemplo que muestra cómo publicar un
mensaje simple utilizando una solicitud POST:

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

Esto creará una notificación que se ve así:

<figure markdown>
  ![basic notification](static/img/android-screenshot-basic-notification.png){ width=500 }
  <figcaption>Android notification</figcaption>
</figure>

Eso es todo. Ya está todo listo. Ve a jugar y lee el resto de los documentos. Recomiendo encarecidamente leer al menos la página en
[publicar mensajes](publish.md), así como la página detallada en el [Aplicación Android/iOS](subscribe/phone.md).

Aquí hay otro video que muestra todo el proceso:

<figure>
  <video controls muted autoplay loop width="650" src="static/img/android-video-overview.mp4"></video>
  <figcaption>Sending push notifications to your Android phone</figcaption>
</figure>
