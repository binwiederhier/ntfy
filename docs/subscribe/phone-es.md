# Suscríbete desde tu teléfono

Puedes usar el ntfy [Aplicación android](https://play.google.com/store/apps/details?id=io.heckel.ntfy) o [Aplicación iOS](https://apps.apple.com/us/app/ntfy/id1625396347)
para recibir notificaciones directamente en tu teléfono. Al igual que el servidor, esta aplicación también es de código abierto y el código está disponible.
en GitHub ([Androide](https://github.com/binwiederhier/ntfy-android), [Ios](https://github.com/binwiederhier/ntfy-ios)). Siéntase libre de
contribuir, o [construye el tuyo propio](../develop.md).

<a href="https://play.google.com/store/apps/details?id=io.heckel.ntfy"><img src="../../static/img/badge-googleplay.png"></a> <a href="https://f-droid.org/en/packages/io.heckel.ntfy/"><img src="../../static/img/badge-fdroid.png"></a> <a href="https://apps.apple.com/us/app/ntfy/id1625396347"><img src="../../static/img/badge-appstore.png"></a>

Puede obtener la aplicación de Android desde ambos [Google Play](https://play.google.com/store/apps/details?id=io.heckel.ntfy) y
De [F-Droide](https://f-droid.org/en/packages/io.heckel.ntfy/). Ambos son en gran medida idénticos, con la única excepción de que
el sabor F-Droid no utiliza Firebase. La aplicación iOS se puede descargar desde el [Tienda de aplicaciones](https://apps.apple.com/us/app/ntfy/id1625396347).

## Visión general

Una imagen vale más que mil palabras. Aquí hay algunas capturas de pantalla que muestran cómo se ve la aplicación. Todo es bonito
Sencillo. Puede agregar temas y tan pronto como los agregue, puede [publicar mensajes](../publish.md) a ellos.

<div id="android-screenshots" class="screenshots">
    <a href="../../static/img/android-screenshot-main.png"><img src="../../static/img/android-screenshot-main.png"/></a>
    <a href="../../static/img/android-screenshot-detail.png"><img src="../../static/img/android-screenshot-detail.png"/></a>
    <a href="../../static/img/android-screenshot-pause.png"><img src="../../static/img/android-screenshot-pause.png"/></a>
    <a href="../../static/img/android-screenshot-add.png"><img src="../../static/img/android-screenshot-add.png"/></a>
    <a href="../../static/img/android-screenshot-add-instant.png"><img src="../../static/img/android-screenshot-add-instant.png"/></a>
    <a href="../../static/img/android-screenshot-add-other.png"><img src="../../static/img/android-screenshot-add-other.png"/></a>
</div>

Si esas capturas de pantalla aún no son suficientes, aquí hay un video:

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/android-video-overview.mp4"></video>
  <figcaption>Sending push notifications to your Android phone</figcaption>
</figure>

## Prioridad del mensaje

*Soportado en:* :material-android: :material-manzana:

Cuando [publicar mensajes](../publish.md#message-priority) a un tema, puedes **definir una prioridad**. Esta prioridad define
con qué urgencia Android le notificará sobre la notificación y si hacen un sonido y / o vibran.

De forma predeterminada, los mensajes con prioridad predeterminada o superior (> = 3) vibrarán y emitirán un sonido. Mensajes con alto o urgente
la prioridad (> = 4) también se mostrará como pop-over, así:

<figure markdown>
  ![priority notification](../static/img/priority-notification.png){ width=500 }
  <figcaption>High and urgent notifications show as pop-over</figcaption>
</figure>

Puede cambiar esta configuración en Android presionando prolongadamente la aplicación y tocando "Notificaciones" o desde "Configuración"
en "Configuración del canal". Hay un canal de notificación para cada prioridad:

<figure markdown>
  ![notification settings](../static/img/android-screenshot-notification-settings.png){ width=500 }
  <figcaption>Per-priority channels</figcaption>
</figure>

Por canal de notificación, puede configurar un **sonido específico del canal**, si se debe **Anular el DND de No molestar (DND)**
configuración y otras configuraciones como popover o punto de notificación:

<figure markdown>
  ![channel details](../static/img/android-screenshot-notification-details.jpg){ width=500 }
  <figcaption>Per-priority sound/vibration settings</figcaption>
</figure>

## Entrega instantánea

*Soportado en:* :material-androide:

La entrega instantánea le permite recibir mensajes en su teléfono al instante, **incluso cuando el teléfono está en modo dormitado**, es decir,
cuando la pantalla se apaga y la dejas en el escritorio por un tiempo. Esto se logra con un servicio en primer plano, que
verás como una notificación permanente que se ve así:

<figure markdown>
  ![foreground service](../static/img/foreground-service.png){ width=500 }
  <figcaption>Instant delivery foreground notification</figcaption>
</figure>

Android no le permite descartar esta notificación, a menos que desactive el canal de notificación en la configuración.
Para hacerlo, mantenga presionada la notificación en primer plano (captura de pantalla anterior) y navegue hasta la configuración. A continuación, cambie el botón
"Servicio de suscripción" desactivado:

<figure markdown>
  ![foreground service](../static/img/notification-settings.png){ width=500 }
  <figcaption>Turning off the persistent instant delivery notification</figcaption>
</figure>

**Limitaciones sin entrega instantánea**: Sin entrega instantánea, **Los mensajes pueden llegar con un retraso significativo**
(a veces muchos minutos, o incluso horas después). Si alguna vez has cogido el teléfono y
de repente tuve 10 mensajes que se enviaron mucho antes de que supieras de lo que estoy hablando.

La razón de esto es [Mensajería en la nube de Firebase (FCM)](https://firebase.google.com/docs/cloud-messaging). FCM es el
*solamente* Google aprobó la forma de enviar mensajes push a dispositivos Android, y es lo que casi todas las aplicaciones usan para entregar push
notificaciones. Firebase es en general bastante malo para entregar mensajes a tiempo, pero en Android, la mayoría de las aplicaciones están atascadas con él.

La aplicación ntfy para Android usa Firebase solo para el host principal `ntfy.sh`, y solo en el sabor Google Play de la aplicación.
No usará Firebase para ningún servidor autohospedado, y no en absoluto en el sabor F-Droid.

## Compartir en el tema

*Soportado en:* :material-androide:

Puede compartir archivos en un tema utilizando la función "Compartir" de Android. Esto funciona en casi cualquier aplicación que admita compartir archivos
o texto, y es útil para enviarse enlaces, archivos u otras cosas. La función recuerda algunos de los últimos temas
compartiste contenido y los enumeras en la parte inferior.

La característica es bastante autoexplicativa, y una imagen dice más de mil palabras. Así que aquí hay dos imágenes:

<div id="share-to-topic-screenshots" class="screenshots">
    <a href="../../static/img/android-screenshot-share-1.jpg"><img src="../../static/img/android-screenshot-share-1.jpg"/></a>
    <a href="../../static/img/android-screenshot-share-2.jpg"><img src="../../static/img/android-screenshot-share-2.jpg"/></a>
</div>

## ntfy:// enlaces

*Soportado en:* :material-androide:

La aplicación ntfy para Android admite enlaces profundos directamente a los temas. Esto es útil cuando se integra con [aplicaciones de automatización](#automation-apps)
como [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid) o [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm),
o simplemente para vincular directamente a un tema desde un sitio web móvil.

!!! información
La vinculación profunda de Android de enlaces http / https es muy frágil y limitada, por lo que algo como `https://<host>/<topic>/subscribe` es
**no es posible**, y en su lugar `ntfy://` hay que utilizar enlaces. Más detalles en [número #20](https://github.com/binwiederhier/ntfy/issues/20).

**Formatos de enlace compatibles:**

| Formato de enlace | Ejemplo | Descripción                                                                                                                                                                                         |
|-------------------------------------------------------------------------------|-------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| <span style="white-space: nowrap">`ntfy://<host>/<topic>`</span>              | `ntfy://ntfy.sh/mytopic`                  | Abre directamente la vista de detalles de la aplicación de Android para el tema y el servidor dados. Se suscribe al tema si aún no está suscrito. Esto es equivalente a la vista web `https://ntfy.sh/mytopic` (¡HTTPS!) |
| <span style="white-space: nowrap">`ntfy://<host>/<topic>?secure=false`</span> | `ntfy://example.com/mytopic?secure=false` | Igual que el anterior, excepto que esto usará HTTP en lugar de HTTPS como URL del tema. Esto es equivalente a la vista web `http://example.com/mytopic` (¡HTTP!)                                                |

## Integraciones

### UnifiedPush

*Soportado en:* :material-androide:

[UnifiedPush](https://unifiedpush.org) es un estándar para recibir notificaciones push sin usar la propiedad de Google
[Mensajería en la nube de Firebase (FCM)](https://firebase.google.com/docs/cloud-messaging) servicio. Pone notificaciones push
en el control del usuario. ntfy puede actuar como un **Distribuidor de UnifiedPush**, reenviar mensajes a aplicaciones que lo admitan.

Para utilizar ntfy como distribuidor, simplemente selecciónelo en uno de los [aplicaciones compatibles](https://unifiedpush.org/users/apps/).
Eso es todo. Es una instalación 😀 de un solo paso. Si lo desea, puede seleccionar el suyo propio [servidor ntfy autohospedado](../install.md)
para manejar mensajes. Aquí hay un ejemplo con [FluffyChat](https://fluffychat.im/):

<div id="unifiedpush-screenshots" class="screenshots">
    <a href="../../static/img/android-screenshot-unifiedpush-fluffychat.jpg"><img src="../../static/img/android-screenshot-unifiedpush-fluffychat.jpg"/></a>
    <a href="../../static/img/android-screenshot-unifiedpush-subscription.jpg"><img src="../../static/img/android-screenshot-unifiedpush-subscription.jpg"/></a>
    <a href="../../static/img/android-screenshot-unifiedpush-settings.jpg"><img src="../../static/img/android-screenshot-unifiedpush-settings.jpg"/></a>
</div>

### Aplicaciones de automatización

*Soportado en:* :material-androide:

La aplicación ntfy para Android se integra muy bien con aplicaciones de automatización como [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid)
o [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm). Usando las intenciones de Android, puedes
**reaccionar a los mensajes entrantes**, así como **enviar mensajes**.

#### Reaccionar a los mensajes entrantes

Para reaccionar a las notificaciones entrantes, debe registrarse en intents con el `io.heckel.ntfy.MESSAGE_RECEIVED` acción (véase
[código para más detalles](https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/java/io/heckel/ntfy/msg/BroadcastService.kt)).
Aquí hay un ejemplo usando [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid)
y [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm), pero cualquier aplicación que pueda atrapar
se admiten transmisiones:

<div id="integration-screenshots-receive" class="screenshots">
    <a href="../../static/img/android-screenshot-macrodroid-overview.png"><img src="../../static/img/android-screenshot-macrodroid-overview.png"/></a>
    <a href="../../static/img/android-screenshot-macrodroid-trigger.png"><img src="../../static/img/android-screenshot-macrodroid-trigger.png"/></a>
    <a href="../../static/img/android-screenshot-macrodroid-action.png"><img src="../../static/img/android-screenshot-macrodroid-action.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-profiles.png"><img src="../../static/img/android-screenshot-tasker-profiles.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-event-edit.png"><img src="../../static/img/android-screenshot-tasker-event-edit.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-task-edit.png"><img src="../../static/img/android-screenshot-tasker-task-edit.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-action-edit.png"><img src="../../static/img/android-screenshot-tasker-action-edit.png"/></a>
</div>

Para MacroDroid, asegúrese de escribir el nombre del paquete `io.heckel.ntfy`, de lo contrario, las intenciones pueden tragarse en silencio.
Si usas temas para impulsar la automatización, es probable que desees silenciar el tema en la aplicación ntfy. Esto evitará
ventanas emergentes de notificación:

<figure markdown>
  ![muted subscription](../static/img/android-screenshot-muted.png){ width=500 }
  <figcaption>Muting notifications to prevent popups</figcaption>
</figure>

Aquí hay una lista de extras a los que puede acceder. Lo más probable es que desee filtrar por `topic` y reaccionar en `message`:

| Nombre adicional | Tipo | Ejemplo | Descripción |
|----------------------|------------------------------|------------------------------------------|------------------------------------------------------------------------------------|
| `id`                 | *Cuerda*                     | `bP8dMjO8ig`                             | Identificador de mensaje elegido al azar (probablemente no muy útil para la automatización de tareas) |
| `base_url`           | *Cuerda*                     | `https://ntfy.sh`                        | URL raíz del servidor ntfy este mensaje proviene de |
| `topic` ❤️           | *Cuerda*                     | `mytopic`                                | Nombre del tema; **Es probable que desee filtrar por un tema específico**                  |
| `muted`              | *Booleano*                    | `true`                                   | Indica si la suscripción se silenció en la aplicación |
| `muted_str`          | *Cadena (`true` o `false`)* | `true`                                   | Igual que `muted`, pero como cadena `true` o `false`                                   |
| `time`               | *Int*                        | `1635528741`                             | Hora de la fecha del mensaje, como | de la marca de tiempo de Unix
| `title`              | *Cuerda*                     | `Some title`                             | Mensaje [título](../publish.md#message-title); puede estar vacío si no está configurado |
| `message` ❤️         | *Cuerda*                     | `Some message`                           | Cuerpo del mensaje; **Esto es probablemente lo que te interesa**                         |
| `message_bytes`      | *ByteArray*                  | `(binary data)`                          | Cuerpo del mensaje como | de datos binarios
| `encoding`️          | *Cuerda*                     | -                                        | Codificación de mensajes (vacía o "base64") |
| `tags`               | *Cuerda*                     | `tag1,tag2,..`                           | Lista separada por comas de [Etiquetas](../publish.md#tags-emojis)                          |
| `tags_map`           | *Cuerda*                     | `0=tag1,1=tag2,..`                       | Mapa de etiquetas para facilitar el mapeo primero, segundo, ... | de etiquetas
| `priority`           | *Int (entre 1-5)*          | `4`                                      | Mensaje [prioridad](../publish.md#message-priority) con 1=min, 3=default y 5=max |
| `click`              | *Cuerda*                     | `https://google.com`                     | [Haga clic en la acción](../publish.md#click-action) URL, o vacía si no está establecida |
| `attachment_name`    | *Cuerda*                     | `attachment.jpg`                         | Nombre de archivo del archivo adjunto; puede estar vacío si no está configurado |
| `attachment_type`    | *Cuerda*                     | `image/jpeg`                             | Tipo mimo del accesorio; puede estar vacío si no está configurado |
| `attachment_size`    | *Largo*                       | `9923111`                                | Tamaño en bytes del archivo adjunto; puede ser cero si no se establece |
| `attachment_expires` | *Largo*                       | `1655514244`                             | Fecha de caducidad como marca de tiempo Unix de la URL adjunta; puede ser cero si no se establece |
| `attachment_url`     | *Cuerda*                     | `https://ntfy.sh/file/afUbjadfl7ErP.jpg` | URL del archivo adjunto; puede estar vacío si no está configurado |

#### Enviar mensajes mediante intenciones

Para enviar mensajes desde otras aplicaciones (como [Macrodroid](https://play.google.com/store/apps/details?id=com.arlosoft.macrodroid)
y [Tasker](https://play.google.com/store/apps/details?id=net.dinglisch.android.taskerm)), puede
transmitir una intención con el `io.heckel.ntfy.SEND_MESSAGE` acción. La aplicación ntfy para Android reenviará la intención como un HTTP
SOLICITUD POST a [publicar un mensaje](../publish.md). Esto es principalmente útil para aplicaciones que no admiten HTTP POST/PUT
(como MacroDroid). En Tasker, simplemente puede usar la acción "Solicitud HTTP", que es un poco más fácil y también funciona si
ntfy no está instalado.

Así es como se ve:

<div id="integration-screenshots-send" class="screenshots">
    <a href="../../static/img/android-screenshot-macrodroid-send-macro.png"><img src="../../static/img/android-screenshot-macrodroid-send-macro.png"/></a>
    <a href="../../static/img/android-screenshot-macrodroid-send-action.png"><img src="../../static/img/android-screenshot-macrodroid-send-action.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-profile-send.png"><img src="../../static/img/android-screenshot-tasker-profile-send.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-task-edit-post.png"><img src="../../static/img/android-screenshot-tasker-task-edit-post.png"/></a>
    <a href="../../static/img/android-screenshot-tasker-action-http-post.png"><img src="../../static/img/android-screenshot-tasker-action-http-post.png"/></a>
</div>

Los siguientes extras de intención son compatibles cuando para la intención con el `io.heckel.ntfy.SEND_MESSAGE` acción:

| Nombre adicional | | requerido Tipo | Ejemplo | Descripción |
|--------------|----------|-------------------------------|-------------------|------------------------------------------------------------------------------------|
| `base_url`   | -        | *Cuerda*                      | `https://ntfy.sh` | La dirección URL raíz del servidor ntfy del que procede este mensaje, de forma predeterminada es `https://ntfy.sh`  |
| `topic` ❤️   | ✔        | *Cuerda*                      | `mytopic`         | Nombre del tema; **Debe establecer esto**                                                  |
| `title`      | -        | *Cuerda*                      | `Some title`      | Mensaje [título](../publish.md#message-title); puede estar vacío si no está configurado |
| `message` ❤️ | ✔        | *Cuerda*                      | `Some message`    | Cuerpo del mensaje; **Debe establecer esto**                                                |
| `tags`       | -        | *Cuerda*                      | `tag1,tag2,..`    | Lista separada por comas de [Etiquetas](../publish.md#tags-emojis)                          |
| `priority`   | -        | *String o Int (entre 1-5)* | `4`               | Mensaje [prioridad](../publish.md#message-priority) con 1=min, 3=default y 5=max |
