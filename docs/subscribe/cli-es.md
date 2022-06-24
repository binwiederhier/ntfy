# Suscríbete a través de ntfy CLI

Además de suscribirse a través del [interfaz de usuario web](web.md)el [aplicación de teléfono](phone.md), o el [API](api.md), puedes suscribirte
a los temas a través de la CLI de ntfy. La CLI está incluida en la misma `ntfy` binario que se puede utilizar para [autohospedar un servidor](../install.md).

!!! información
El **La CLI de Ntfy no es necesaria para enviar o recibir mensajes**. En su lugar, puede [enviar mensajes con curl](../publish.md),
e incluso usarlo para [suscribirse a temas](api.md). Puede ser un poco más conveniente usar la CLI ntfy que escribir
tu propio guión. Todo depende del caso de uso. 😀

## Instalar + configurar

Para instalar la CLI de ntfy, simplemente **Siga los pasos descritos en el cuadro de diálogo [página de instalación](../install.md)**. El servidor ntfy y
los clientes son el mismo binario, por lo que todo es muy conveniente. Después de la instalación, puede (opcionalmente) configurar el cliente
creando `~/.config/ntfy/client.yml` (para el usuario no root), o `/etc/ntfy/client.yml` (para el usuario root). Tú
puede encontrar un [configuración de esqueleto](https://github.com/binwiederhier/ntfy/blob/main/client/client.yml) en GitHub.

Si solo quieres usar [ntfy.sh](https://ntfy.sh), no tienes que cambiar nada. Si usted **autohospede su propio servidor**,
Es posible que desee modificar el cuadro de diálogo `default-host` opción:

```yaml
# Base URL used to expand short topic names in the "ntfy publish" and "ntfy subscribe" commands.
# If you self-host a ntfy server, you'll likely want to change this.
#
default-host: https://ntfy.myhost.com
```

## Publicar mensajes

Puede enviar mensajes con la CLI de ntfy mediante el `ntfy publish` (o cualquiera de sus alias) `pub`, `send` o
`trigger`). Hay muchos ejemplos en la página sobre [publicar mensajes](../publish.md), pero aquí hay algunos
rápidos:

\=== "Envío simple"
`     ntfy publish mytopic This is a message
    ntfy publish mytopic "This is a message"
    ntfy pub mytopic "This is a message" 
    `

\=== "Enviar con título, prioridad y etiquetas"
`     ntfy publish \         --title="Thing sold on eBay" \         --priority=high \         --tags=partying_face \
        mytopic \
        "Somebody just bought the thing that you sell"
    `

\=== "Enviar a las 8:30am"
`     ntfy pub --at=8:30am delayed_topic Laterzz
    `

\=== "Triggering a webhook"
`     ntfy trigger mywebhook
    ntfy pub mywebhook
    `

### Adjuntar un archivo local

Puede cargar y adjuntar fácilmente un archivo local a una notificación:

    $ ntfy pub --file README.md mytopic | jq .
    {
      "id": "meIlClVLABJQ",
      "time": 1655825460,
      "event": "message",
      "topic": "mytopic",
      "message": "You received a file: README.md",
      "attachment": {
        "name": "README.md",
        "type": "text/plain; charset=utf-8",
        "size": 2892,
        "expires": 1655836260,
        "url": "https://ntfy.sh/file/meIlClVLABJQ.txt"
      }
    }

### Espere a PID/comando

Si tiene un comando de ejecución prolongada y desea **Publicar una notificación cuando se complete el comando**,
puede envolverlo con `ntfy publish --wait-cmd` (alias: `--cmd`, `--done`). O, si olvidó envolverlo, y el
El comando ya se está ejecutando, puede esperar a que se complete el proceso con `ntfy publish --wait-pid` (alias: `--pid`).

Ejecute un comando y espere a que se complete (aquí: `rsync ...`):

    $ ntfy pub --wait-cmd mytopic rsync -av ./ root@example.com:/backups/ | jq .
    {
      "id": "Re0rWXZQM8WB",
      "time": 1655825624,
      "event": "message",
      "topic": "mytopic",
      "message": "Command succeeded after 56.553s: rsync -av ./ root@example.com:/backups/"
    }

O bien, si ya inició el proceso de ejecución prolongada y desea esperarlo con su ID de proceso (PID), puede hacer lo siguiente:

\=== "Usando un PID directamente"
`     $ ntfy pub --wait-pid 8458 mytopic | jq .
    {
      "id": "orM6hJKNYkWb",
      "time": 1655825827,
      "event": "message",
      "topic": "mytopic",
      "message": "Process with PID 8458 exited after 2.003s"
    }
    `

\=== "Usando un `pidof`"
`     $ ntfy pub --wait-pid $(pidof rsync) mytopic | jq .
    {
      "id": "orM6hJKNYkWb",
      "time": 1655825827,
      "event": "message",
      "topic": "mytopic",
      "message": "Process with PID 8458 exited after 2.003s"
    }
    `

## Suscríbete a los temas

Puede suscribirse a temas usando `ntfy subscribe`. Dependiendo de cómo se llame, este comando
imprimirá o ejecutará un comando para cada mensaje que llegue. Hay algunas maneras diferentes
en el que se puede ejecutar el comando:

### Transmitir mensajes como JSON

    ntfy subscribe TOPIC

Si ejecuta el comando de esta manera, imprime la representación JSON de cada mensaje entrante. Esto es útil
cuando tiene un comando que desea transmitir y leer mensajes JSON entrantes. A menos que `--poll` se pasa, este comando
permanece abierto para siempre.

    $ ntfy sub mytopic
    {"id":"nZ8PjH5oox","time":1639971913,"event":"message","topic":"mytopic","message":"hi there"}
    {"id":"sekSLWTujn","time":1639972063,"event":"message","topic":"mytopic",priority:5,"message":"Oh no!"}
    ...

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-1.mp4"></video>
  <figcaption>Subscribe in JSON mode</figcaption>
</figure>

### Ejecutar comando para cada mensaje

    ntfy subscribe TOPIC COMMAND

Si lo ejecuta de esta manera, se ejecuta un COMANDO para cada mensaje entrante. Desplácese hacia abajo para ver una lista de disponibles
variables de entorno. Aquí hay algunos ejemplos:

    ntfy sub mytopic 'notify-send "$m"'
    ntfy sub topic1 /my/script.sh
    ntfy sub topic1 'echo "Message $m was received. Its title was $t and it had priority $p'

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-2.webm"></video>
  <figcaption>Execute command on incoming messages</figcaption>
</figure>

Los campos de mensaje se pasan al comando como variables de entorno y se pueden utilizar en scripts. Tenga en cuenta que desde
Estas son variables de entorno, normalmente no tiene que preocuparse por citar demasiado, siempre y cuando las incluya
entre comillas dobles, debería estar bien:

| | variable Alias | Descripción |
|------------------|----------------------------|----------------------------------------|
| `$NTFY_ID`       | `$id`                      | Identificador de mensaje único |
| `$NTFY_TIME`     | `$time`                    | Marca de tiempo Unix del | de entrega de mensajes
| `$NTFY_TOPIC`    | `$topic`                   | Nombre del tema |
| `$NTFY_MESSAGE`  | `$message`, `$m`           | | del cuerpo del mensaje
| `$NTFY_TITLE`    | `$title`, `$t`             | Título del mensaje |
| `$NTFY_PRIORITY` | `$priority`, `$prio`, `$p` | Prioridad del mensaje (1=min, 5=max) |
| `$NTFY_TAGS`     | `$tags`, `$tag`, `$ta`     | Etiquetas de mensaje (lista separada por comas) |
| `$NTFY_RAW`      | `$raw`                     | | de mensajes JSON sin procesar

### Suscríbete a varios temas

    ntfy subscribe --from-config

Para suscribirse a varios temas a la vez y ejecutar diferentes comandos para cada uno, puede usar `ntfy subscribe --from-config`,
que leerá el `subscribe` config desde el archivo de configuración. Por favor, echa un vistazo también a la [Servicio systemd ntfy-client](#using-the-systemd-service).

Aquí hay un archivo de configuración de ejemplo que se suscribe a tres temas diferentes, ejecutando un comando diferente para cada uno de ellos:

\=== "~/.config/ntfy/client.yml (Linux)"
` yaml
    subscribe:     - topic: echo-this
      command: 'echo "Message received: $message"'     - topic: alerts
      command: notify-send -i /usr/share/ntfy/logo.png "Important" "$m"
      if:
        priority: high,urgent     - topic: calc
      command: 'gnome-calculator 2>/dev/null &'     - topic: print-temp
      command: |
            echo "You can easily run inline scripts, too."
            temp="$(sensors | awk '/Pack/ { print substr($4,2,2) }')"
            if [ $temp -gt 80 ]; then
              echo "Warning: CPU temperature is $temp. Too high."
            else
              echo "CPU temperature is $temp. That's alright."
            fi
     `

\=== "~/Library/Application Support/ntfy/client.yml (macOS)"
` yaml
    subscribe:       - topic: echo-this
        command: 'echo "Message received: $message"'       - topic: alerts
        command: osascript -e "display notification \"$message\""
        if:
          priority: high,urgent       - topic: calc
        command: open -a Calculator
     `

\=== "%AppData%\ntfy\client.yml (Windows)"
` yaml
    subscribe:     - topic: echo-this
      command: 'echo Message received: %message%'     - topic: alerts
      command: |
        notifu /m "%NTFY_MESSAGE%"
        exit 0
      if:
        priority: high,urgent     - topic: calc
      command: calc
     `

En este ejemplo, cuando `ntfy subscribe --from-config` se ejecuta:

*   Mensajes a `echo-this` simplemente hace eco a la salida estándar
*   Mensajes a `alerts` Mostrar como notificación de escritorio para mensajes de alta prioridad mediante [notificar-enviar](https://manpages.ubuntu.com/manpages/focal/man1/notify-send.1.html) (Linux),
    [notifu](https://www.paralint.com/projects/notifu/) (Ventanas) o `osascript` (macOS)
*   Mensajes a `calc` abra la calculadora 😀 (*porque, ¿por qué no?*)
*   Mensajes a `print-temp` Ejecute un script en línea e imprima la temperatura de la CPU (solo versión de Linux)

Espero que esto muestre cuán poderoso es este comando. Aquí hay un breve video que demuestra el ejemplo anterior:

<figure>
  <video controls muted autoplay loop width="650" src="../../static/img/cli-subscribe-video-3.webm"></video>
  <figcaption>Execute all the things</figcaption>
</figure>

### Uso del servicio systemd

Puede utilizar el `ntfy-client` servicio systemd (consulte [ntfy-client.service](https://github.com/binwiederhier/ntfy/blob/main/client/ntfy-client.service))
para suscribirse a varios temas como en el ejemplo anterior. El servicio se instala automáticamente (pero no se inicia)
si instala el paquete deb/rpm. Para configurarlo, simplemente edite `/etc/ntfy/client.yml` y ejecutar `sudo systemctl restart ntfy-client`.

!!! información
El `ntfy-client.service` se ejecuta como usuario `ntfy`, lo que significa que se aplican restricciones de permisos típicas de Linux. Vea a continuación
para saber cómo solucionarlo.

Si el servicio se ejecuta en el equipo de escritorio personal, es posible que desee invalidar el usuario/grupo de servicio (`User=` y `Group=`), y
ajustar el `DISPLAY` y `DBUS_SESSION_BUS_ADDRESS` variables de entorno. Esto le permitirá ejecutar comandos en su sesión X
como usuario principal de la máquina.

Puede invalidar manualmente estas entradas de servicio systemd con `sudo systemctl edit ntfy-client`, y agregue esto
(suponiendo que su usuario es `phil`). No te olvides de correr `sudo systemctl daemon-reload` y `sudo systemctl restart ntfy-client`
después de editar el archivo de servicio:

\=== "/etc/systemd/system/ntfy-client.service.d/override.conf"
`     [Service]
    User=phil
    Group=phil
    Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"
    `
O bien, puede ejecutar el siguiente script que crea esta configuración de anulación por usted:

    sudo sh -c 'cat > /etc/systemd/system/ntfy-client.service.d/override.conf' <<EOF
    [Service]
    User=$USER
    Group=$USER
    Environment="DISPLAY=:0" "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$(id -u)/bus"
    EOF

    sudo systemctl daemon-reload
    sudo systemctl restart ntfy-client

### Autenticación

Dependiendo de si el servidor está configurado para admitir [control de acceso](../config.md#access-control), algunos temas
puede estar protegido contra lectura/escritura para que solo los usuarios con las credenciales correctas puedan suscribirse o publicar en ellos.
Para publicar/suscribirse a temas protegidos, puede utilizar [Autenticación básica](https://en.wikipedia.org/wiki/Basic_access_authentication)
con un nombre de usuario/contraseña válido. Para su servidor autohospedado, **Asegúrese de usar HTTPS para evitar escuchas** y exponer
su contraseña.

Puede agregar su nombre de usuario y contraseña al archivo de configuración:
\=== "~/.config/ntfy/client.yml"
`yaml 	 - topic: secret
	   command: 'notify-send "$m"'
	   user: phill
	   password: mypass
	`

O con el `ntfy subscibe` mandar:

    ntfy subscribe \
      -u phil:mypass \
      ntfy.example.com/mysecrets
