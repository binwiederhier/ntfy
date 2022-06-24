# Ejemplos

Hay un millón de formas de usar ntfy, pero aquí hay algunas inspiraciones. Trato de coleccionar <a href="https://github.com/binwiederhier/ntfy/tree/main/examples">ejemplos en GitHub</a>, así que asegúrese de comprobar
los que están fuera, también.

!!! información
Muchos de estos ejemplos fueron aportados por usuarios de ntfy. Si tiene otros ejemplos de cómo usa ntfy, por favor
[Crear una solicitud de extracción](https://github.com/binwiederhier/ntfy/pulls), y felizmente lo incluiré. También tenga en cuenta que
No puedo garantizar que todos estos ejemplos sean funcionales. Muchos de ellos no los he probado yo mismo.

## Cronjobs

ntfy es perfecto para cualquier tipo de cronjobs o justo cuando se realizan procesos largos (copias de seguridad, pipelines, comandos de copia rsync, ...).

Comencé a agregar notificaciones en casi todos mis scripts. Por lo general, solo encadeno el <tt>rizo</tt> llamar
directamente al comando que estoy ejecutando. El siguiente ejemplo enviará <i>La copia de seguridad de la computadora portátil se realizó correctamente</i>
o ⚠️ <i>Error en la copia de seguridad de la computadora portátil</i> directamente a mi teléfono:

    rsync -a root@laptop /backups/laptop \
      && zfs snapshot ... \
      && curl -H prio:low -d "Laptop backup succeeded" ntfy.sh/backups \
      || curl -H tags:warning -H prio:high -d "Laptop backup failed" ntfy.sh/backups

Aquí hay uno para los libros de historia. Quiero desesperadamente el `github.com/ntfy` organización, pero todas mis entradas con
GitHub ha estado desesperado. En caso de que alguna vez esté disponible, quiero saberlo de inmediato.

```cron
# Check github/ntfy user
*/6 * * * * if curl -s https://api.github.com/users/ntfy | grep "Not Found"; then curl -d "github.com/ntfy is available" -H "Tags: tada" -H "Prio: high" ntfy.sh/my-alerts; fi
```

## Alertas de poco espacio en disco

Aquí hay un cronjob simple que uso para avisarme cuando el espacio en disco en el disco raíz se está agotando. Es simple, pero
eficaz.

```bash
#!/bin/bash

mingigs=10
avail=$(df | awk '$6 == "/" && $4 < '$mingigs' * 1024*1024 { print $4/1024/1024 }')
topicurl=https://ntfy.sh/mytopic

if [ -n "$avail" ]; then
  curl \
    -d "Only $avail GB available on the root disk. Better clean that up." \
    -H "Title: Low disk space alert on $(hostname)" \
    -H "Priority: high" \
    -H "Tags: warning,cd" \
    $topicurl
fi
```

## Alertas de inicio de sesión SSH

Hace años mi servidor doméstico fue allanado. Eso me sacudió con fuerza, así que cada vez que alguien inicia sesión en cualquier máquina que yo
Propio, ahora me envío un mensaje. Aquí hay un ejemplo de cómo usar <a href="https://en.wikipedia.org/wiki/Linux_PAM">PAM</a>
para notificarse a sí mismo en el inicio de sesión SSH.

\=== "/etc/pam.d/sshd"
`     # at the end of the file
    session optional pam_exec.so /usr/bin/ntfy-ssh-login.sh
    `

\=== "/usr/bin/ntfy-ssh-login.sh"
` bash     #!/bin/bash
    if [ "${PAM_TYPE}" = "open_session" ]; then
      curl \         -H prio:high \         -H tags:warning \         -d "SSH login: ${PAM_USER} from ${PAM_RHOST}" \
        ntfy.sh/alerts
    fi
     `

## Recopilar datos de varias máquinas

El otro día estaba ejecutando tareas en 20 servidores, y quería recopilar los resultados provisionales.
como csv en un solo lugar. Cada uno de los servidores se publicaba en un tema a medida que se completaban los resultados (`publish-result.sh`),
y tenía un colector central para obtener los resultados a medida que entraban (`collect-results.sh`).

Se veía algo como esto:

\=== "collect-results.sh"
` bash
    while read result; do       [ -n "$result" ] && echo "$result" >> results.csv
    done < <(stdbuf -i0 -o0 curl -s ntfy.sh/results/raw)
     `
\=== "publish-result.sh"
'''bash
Este script se ejecutó en cada uno de los 20 servidores. Estaba haciendo un procesamiento pesado ...

    // Publish script results
    curl -d "$(hostname),$count,$time" ntfy.sh/results
    ```

## Ansible, Sal y Títere

Puede integrar fácilmente ntfy en Ansible, Salt o Puppet para notificarle cuando se realizan o se realizan carreras.
Uno de mis compañeros de trabajo utiliza la siguiente tarea de Ansible para hacerle saber cuándo se hacen las cosas:

```yml
- name: Send ntfy.sh update
  uri:
    url: "https://ntfy.sh/{{ ntfy_channel }}"
    method: POST
    body: "{{ inventory_hostname }} reseeding complete"
```

## Atalaya (shoutrrr)

Puedes usar [gritarrr](https://github.com/containrrr/shoutrrr) Soporte genérico de webhook para enviar
[Atalaya](https://github.com/containrrr/watchtower/) notificaciones a su tema ntfy.

Ejemplo docker-compose.yml:

```yml
services:
  watchtower:
    image: containrrr/watchtower
    environment:
      - WATCHTOWER_NOTIFICATIONS=shoutrrr
      - WATCHTOWER_NOTIFICATION_URL=generic+https://ntfy.sh/my_watchtower_topic?title=WatchtowerUpdates
```

O, si solo desea enviar notificaciones usando shoutrrr:

    shoutrrr send -u "generic+https://ntfy.sh/my_watchtower_topic?title=WatchtowerUpdates" -m "testMessage"

## Sonarr, Radarr, Lidarr, Readarr, Prowlarr, SABnzbd

Es posible usar scripts personalizados para todos los servicios \*arr, además de SABnzbd. Notificaciones para descargas, advertencias, agarres, etc.
Algunos scripts bash simples para lograr esto se proporcionan amablemente en [Repositorio de nickexyz](https://github.com/nickexyz/ntfy-shellscripts).

## Nodo-ROJO

Puede utilizar el nodo de solicitud HTTP para enviar mensajes con [Nodo-ROJO](https://nodered.org), algunos ejemplos:

<details>
<summary>Example: Send a message (click to expand)</summary>

```json
[
    {
        "id": "c956e688cc74ad8e",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "ntfy.sh",
        "method": "POST",
        "ret": "txt",
        "paytoqs": "ignore",
        "url": "https://ntfy.sh/mytopic",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 590,
        "y": 3160,
        "wires":
        [
            []
        ]
    },
    {
        "id": "32ee1eade51fae50",
        "type": "function",
        "z": "fabdd7a3.4045a",
        "name": "data",
        "func": "msg.payload = \"Something happened\";\nmsg.headers = {};\nmsg.headers['tags'] = 'house';\nmsg.headers['X-Title'] = 'Home Assistant';\n\nreturn msg;",
        "outputs": 1,
        "noerr": 0,
        "initialize": "",
        "finalize": "",
        "libs": [],
        "x": 470,
        "y": 3160,
        "wires":
        [
            [
                "c956e688cc74ad8e"
            ]
        ]
    },
    {
        "id": "b287e59cd2311815",
        "type": "inject",
        "z": "fabdd7a3.4045a",
        "name": "Manual start",
        "props":
        [
            {
                "p": "payload"
            },
            {
                "p": "topic",
                "vt": "str"
            }
        ],
        "repeat": "",
        "crontab": "",
        "once": false,
        "onceDelay": "20",
        "topic": "",
        "payload": "",
        "payloadType": "date",
        "x": 330,
        "y": 3160,
        "wires":
        [
            [
                "32ee1eade51fae50"
            ]
        ]
    }
]
```

</details>

![Node red message flow](static/img/nodered-message.png)

<details>
<summary>Example: Send a picture (click to expand)</summary>

```json
[
    {
        "id": "d135a13eadeb9d6d",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "Download image",
        "method": "GET",
        "ret": "bin",
        "paytoqs": "ignore",
        "url": "https://www.google.com/images/branding/googlelogo/1x/googlelogo_color_272x92dp.png",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 490,
        "y": 3320,
        "wires":
        [
            [
                "6e75bc41d2ec4a03"
            ]
        ]
    },
    {
        "id": "6e75bc41d2ec4a03",
        "type": "function",
        "z": "fabdd7a3.4045a",
        "name": "data",
        "func": "msg.payload = msg.payload;\nmsg.headers = {};\nmsg.headers['tags'] = 'house';\nmsg.headers['X-Title'] = 'Home Assistant - Picture';\n\nreturn msg;",
        "outputs": 1,
        "noerr": 0,
        "initialize": "",
        "finalize": "",
        "libs": [],
        "x": 650,
        "y": 3320,
        "wires":
        [
            [
                "eb160615b6ceda98"
            ]
        ]
    },
    {
        "id": "eb160615b6ceda98",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "ntfy.sh",
        "method": "PUT",
        "ret": "bin",
        "paytoqs": "ignore",
        "url": "https://ntfy.sh/mytopic",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 770,
        "y": 3320,
        "wires":
        [
            []
        ]
    },
    {
        "id": "5b8dbf15c8a7a3a5",
        "type": "inject",
        "z": "fabdd7a3.4045a",
        "name": "Manual start",
        "props":
        [
            {
                "p": "payload"
            },
            {
                "p": "topic",
                "vt": "str"
            }
        ],
        "repeat": "",
        "crontab": "",
        "once": false,
        "onceDelay": "20",
        "topic": "",
        "payload": "",
        "payloadType": "date",
        "x": 310,
        "y": 3320,
        "wires":
        [
            [
                "d135a13eadeb9d6d"
            ]
        ]
    }
]
```

</details>

![Node red picture flow](static/img/nodered-picture.png)

## Gatú

Un ejemplo para una alerta personalizada con [Gatú](https://github.com/TwiN/gatus):

```yaml
alerting:
  custom:
    url: "https://ntfy.sh"
    method: "POST"
    body: |
      {
        "topic": "mytopic",
        "message": "[ENDPOINT_NAME] - [ALERT_DESCRIPTION]",
        "title": "Gatus",
        "tags": ["[ALERT_TRIGGERED_OR_RESOLVED]"],
        "priority": 3
      }
    default-alert:
      enabled: true
      description: "health check failed"
      send-on-resolved: true
      failure-threshold: 3
      success-threshold: 3
    placeholders:
      ALERT_TRIGGERED_OR_RESOLVED:
        TRIGGERED: "warning"
        RESOLVED: "white_check_mark"
```

## Jellyseerr/Overseerr webhook

Aquí hay un ejemplo para [medusa](https://github.com/Fallenbagel/jellyseerr)/[supervisor](https://overseerr.dev/) webhook
Carga útil JSON. Recuerde cambiar el `https://requests.example.com` a tu URL de jellyseerr/overseerr.

```json
{
    "topic": "requests",
    "title": "{{event}}",
    "message": "{{subject}}\n{{message}}\n\nRequested by: {{requestedBy_username}}\n\nStatus: {{media_status}}\nRequest Id: {{request_id}}",
    "priority": 4,
    "attach": "{{image}}",
    "click": "https://requests.example.com/{{media_type}}/{{media_tmdbid}}"
}
```

## Asistente de inicio

A continuación se muestra un ejemplo del archivo configuration.yml para configurar un componente de notificación de REST.
Dado que Home Assistant va a POST JSON, debe especificar la raíz de su recurso ntfy.

```yaml
notify:
  - name: ntfy
    platform: rest
    method: POST_JSON
    data:
      topic: YOUR_NTFY_TOPIC
    title_param_name: title
    message_param_name: message
    resource: https://ntfy.sh
```

Si necesita autenticarse en su recurso ntfy, defina la autenticación, el nombre de usuario y la contraseña como se indica a continuación:

```yaml
notify:
  - name: ntfy
    platform: rest
    method: POST_JSON
    authentication: basic
    username: YOUR_USERNAME
    password: YOUR_PASSWORD
    data:
      topic: YOUR_NTFY_TOPIC
    title_param_name: title
    message_param_name: message
    resource: https://ntfy.sh
```

Si necesita agregar cualquier otro [Parámetros específicos de ntfy](https://ntfy.sh/docs/publish/#publish-as-json) como prioridad, etiquetas, etc., agréguelas a la `data` en el ejemplo yml. Por ejemplo:

```yaml
notify:
  - name: ntfy
    platform: rest
    method: POST_JSON
    data:
      topic: YOUR_NTFY_TOPIC
      priority: 4
    title_param_name: title
    message_param_name: message
    resource: https://ntfy.sh
```

## Tiempo de actividad Kuma

Vaya a su [Tiempo de actividad Kuma](https://github.com/louislam/uptime-kuma) Configuración > Notificaciones, haga clic en **Notificación de configuración**.
A continuación, configure su deseo **título** (por ejemplo, "Uptime Kuma"), **Tema ntfy**, **URL del servidor** y **prioridad (1-5)**:

<div id="uptimekuma-screenshots" class="screenshots">
    <a href="../static/img/uptimekuma-settings.png"><img src="../static/img/uptimekuma-settings.png"/></a>
    <a href="../static/img/uptimekuma-setup.png"><img src="../static/img/uptimekuma-setup.png"/></a>
</div>

Ahora puede probar las notificaciones y aplicarlas a los monitores:

<div id="uptimekuma-monitor-screenshots" class="screenshots">
    <a href="../static/img/uptimekuma-ios-test.jpg"><img src="../static/img/uptimekuma-ios-test.jpg"/></a>
    <a href="../static/img/uptimekuma-ios-down.jpg"><img src="../static/img/uptimekuma-ios-down.jpg"/></a>
    <a href="../static/img/uptimekuma-ios-up.jpg"><img src="../static/img/uptimekuma-ios-up.jpg"/></a>
</div>

## Informar

ntfy se integra de forma nativa en [Informar](https://github.com/caronc/apprise) (también echa un vistazo a la
[Página wiki de Apprise/ntfy](https://github.com/caronc/apprise/wiki/Notify_ntfy)).

Puedes usarlo así:

    apprise -vv -t "Test Message Title" -b "Test Message Body" \
       ntfy://mytopic

O con su propio servidor de esta manera:

    apprise -vv -t "Test Message Title" -b "Test Message Body" \
       ntfy://ntfy.example.com/mytopic
