# Configuración del servidor ntfy

El servidor ntfy se puede configurar de tres maneras: utilizando un archivo de configuración (normalmente en `/etc/ntfy/server.yml`,
ver [servidor.yml](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml)), a través de argumentos de línea de comandos
o utilizando variables de entorno.

## Inicio rápido

De forma predeterminada, simplemente ejecutando `ntfy serve` iniciará el servidor en el puerto 80. No se necesita configuración. Baterías incluidas 😀.
Si todo funciona como debería, verás algo como esto:

    $ ntfy serve
    2021/11/30 19:59:08 Listening on :80

Puede comenzar inmediatamente [publicar mensajes](publish.md), o suscríbase a través del [Aplicación para Android](subscribe/phone.md),
[la interfaz de usuario web](subscribe/web.md), o simplemente a través de [curl o tu cliente HTTP favorito](subscribe/api.md). Para configurar
el servidor además, echa un vistazo a la [Tabla de opciones de configuración](#config-options) o simplemente escriba `ntfy serve --help` Para
obtener una lista de [opciones de línea de comandos](#command-line-options).

## Ejemplo de configuración

!!! información
Definitivamente echa un vistazo al **[servidor.yml](https://github.com/binwiederhier/ntfy/blob/main/server/server.yml)** archivo.
Contiene ejemplos y descripciones detalladas de todas las configuraciones.

Los ajustes más básicos son `base-url` (la URL externa del servidor ntfy), la dirección de escucha HTTP/HTTPS (`listen-http`
y `listen-https`), y ruta de acceso de socket (`listen-unix`). Todas las demás cosas son características adicionales.

Aquí hay algunas configuraciones de ejemplo que funcionan:

\=== "server.yml (sólo HTTP, con caché + archivos adjuntos)"
` yaml
    base-url: "http://ntfy.example.com"
    cache-file: "/var/cache/ntfy/cache.db"
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    `

\=== "server.yml (HTTP+HTTPS, con caché + adjuntos)"
` yaml
    base-url: "http://ntfy.example.com"
    listen-http: ":80"
    listen-https: ":443"
    key-file: "/etc/letsencrypt/live/ntfy.example.com.key"
    cert-file: "/etc/letsencrypt/live/ntfy.example.com.crt"
    cache-file: "/var/cache/ntfy/cache.db"
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    `

\=== "server.yml (ntfy.sh config)"
''' yaml
\# Todas las cosas: Detrás de un proxy, Firebase, caché, archivos adjuntos,
\# Publicación y recepción SMTP

    base-url: "https://ntfy.sh"
    listen-http: "127.0.0.1:2586"
    firebase-key-file: "/etc/ntfy/firebase.json"
    cache-file: "/var/cache/ntfy/cache.db"
    behind-proxy: true
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    smtp-sender-addr: "email-smtp.us-east-2.amazonaws.com:587"
    smtp-sender-user: "AKIDEADBEEFAFFE12345"
    smtp-sender-pass: "Abd13Kf+sfAk2DzifjafldkThisIsNotARealKeyOMG."
    smtp-sender-from: "ntfy@ntfy.sh"
    smtp-server-listen: ":25"
    smtp-server-domain: "ntfy.sh"
    smtp-server-addr-prefix: "ntfy-"
    keepalive-interval: "45s"
    ```

## Caché de mensajes

Si lo desea, ntfy puede mantener temporalmente las notificaciones en una memoria memoria o en una memoria caché en disco. Almacenamiento en caché de mensajes durante un período corto
del tiempo es importante permitir [Teléfonos](subscribe/phone.md) y otros dispositivos con conexiones a Internet frágiles para poder recuperar
notificaciones que pueden haberse perdido.

De forma predeterminada, ntfy mantiene los mensajes **en memoria durante 12 horas**, lo que significa que **Los mensajes almacenados en caché no sobreviven a una aplicación
reanudar**. Puede invalidar este comportamiento mediante la siguiente configuración:

*   `cache-file`: si se establece, ntfy almacenará los mensajes en una memoria caché basada en SQLite (el valor predeterminado es vacío, lo que significa caché en memoria).
    **Esto es necesario si desea que los mensajes se conserven durante los reinicios**.
*   `cache-duration`: define la duración durante la cual se almacenan los mensajes en la memoria caché (el valor predeterminado es `12h`).

También puede deshabilitar completamente la caché configurando `cache-duration` Para `0`. Cuando la memoria caché está deshabilitada, los mensajes son sólo
se transmite a los suscriptores conectados, pero nunca se almacena en el disco o incluso se mantiene en la memoria más tiempo del necesario para reenviar
el mensaje a los suscriptores.

Los suscriptores pueden recuperar mensajes almacenados en caché mediante el [`poll=1` parámetro](subscribe/api.md#poll-for-messages), así como el
[`since=` parámetro](subscribe/api.md#fetch-cached-messages).

## Accesorios

Si lo desea, puede permitir que los usuarios carguen y [adjuntar archivos a las notificaciones](publish.md#attachments). Para habilitar
En esta característica, simplemente debe configurar un directorio de caché de datos adjuntos y una URL base (`attachment-cache-dir`, `base-url`).
Una vez que se establecen estas opciones y el usuario del servidor puede escribir el directorio, puede cargar archivos adjuntos a través de PUT.

De forma predeterminada, los datos adjuntos se almacenan en la memoria caché de disco **por solo 3 horas**. La razón principal de esto es evitar problemas legales.
y tales cuando se aloja contenido controlado por el usuario. Por lo general, este es un tiempo más que suficiente para el usuario (o la descarga automática)
feature) para descargar el archivo. Las siguientes opciones de configuración son relevantes para los datos adjuntos:

*   `base-url` es la URL raíz del servidor ntfy; Esto es necesario para las direcciones URL de datos adjuntos generadas
*   `attachment-cache-dir` es el directorio de caché de los archivos adjuntos
*   `attachment-total-size-limit` es el límite de tamaño de la caché de datos adjuntos en disco (predeterminado: 5G)
*   `attachment-file-size-limit` es el límite de tamaño de datos adjuntos por archivo (por ejemplo, 300k, 2M, 100M, predeterminado: 15M)
*   `attachment-expiry-duration` es la duración después de la cual se eliminarán los archivos adjuntos cargados (por ejemplo, 3h, 20h, valor predeterminado: 3h)

Aquí hay un ejemplo de configuración que utiliza principalmente los valores predeterminados (excepto el directorio de caché, que está vacío de forma predeterminada):

\=== "/etc/ntfy/server.yml (mínimo)"
` yaml
    base-url: "https://ntfy.sh"
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    `

\=== "/etc/ntfy/server.yml (todas las opciones)"
` yaml
    base-url: "https://ntfy.sh"
    attachment-cache-dir: "/var/cache/ntfy/attachments"
    attachment-total-size-limit: "5G"
    attachment-file-size-limit: "15M"
    attachment-expiry-duration: "3h"
    visitor-attachment-total-size-limit: "100M"
    visitor-attachment-daily-bandwidth-limit: "500M"
    `

Por favor, consulte también el [limitación de velocidad](#rate-limiting) configuración a continuación, específicamente `visitor-attachment-total-size-limit`
y `visitor-attachment-daily-bandwidth-limit`. Establecerlos de manera conservadora es necesario para evitar el abuso.

## Control de acceso

De forma predeterminada, el servidor ntfy está abierto para todos, lo que significa **todo el mundo puede leer y escribir sobre cualquier tema** (así es como
ntfy.sh está configurado). Para restringir el acceso a su propio servidor, puede configurar opcionalmente la autenticación y la autorización.

La autenticación de ntfy se implementa con un simple [SQLite](https://www.sqlite.org/)-backend basado. Implementa dos roles
(`user` y `admin`) y por tema `read` y `write` permisos mediante un [lista de control de acceso (ACL)](https://en.wikipedia.org/wiki/Access-control_list).
Las entradas de control de acceso se pueden aplicar a los usuarios, así como al usuario especial de todos (`*`), que representa el acceso anónimo a la API.

Para configurar la autenticación, simplemente **Configure las dos opciones siguientes**:

*   `auth-file` es la base de datos de usuario/acceso; se crea automáticamente si aún no existe; propuesto
    ubicación `/var/lib/ntfy/user.db` (más fácil si se utiliza el paquete deb/rpm)
*   `auth-default-access` define el acceso predeterminado/de reserva si no se encuentra ninguna entrada de control de acceso; puede ser
    establecer en `read-write` (por defecto), `read-only`, `write-only` o `deny-all`.

Una vez configurado, puede utilizar el `ntfy user` comando a [agregar o modificar usuarios](#users-and-roles), y el `ntfy access` mandar
le permite [Modificar la lista de control de acceso](#access-control-list-acl) para usuarios específicos y patrones de temas. Ambos
Comandos **Edite directamente la base de datos de autenticación** (tal como se define en `auth-file`), por lo que solo funcionan en el servidor, y solo si el usuario
acceder a ellos tiene los permisos correctos.

### Usuarios y roles

El `ntfy user` Le permite agregar,quitar/cambiar usuarios en la base de datos de usuarios de Ntfy, así como cambiar
contraseñas o roles (`user` o `admin`). En la práctica, a menudo solo creará un administrador
usuario con `ntfy user add --role=admin ...` y hágase con todo esto (ver [ejemplo a continuación](#example-private-instance)).

**Papeles:**

*   Rol `user` (predeterminado): los usuarios con este rol no tienen permisos especiales. Administrar el acceso mediante `ntfy access`
    (ver [abajo](#access-control-list-acl)).
*   Rol `admin`: Los usuarios con este rol pueden leer/escribir en todos los temas. El control de acceso granular no es necesario.

**Comandos de ejemplo** (tipo `ntfy user --help` o `ntfy user COMMAND --help` para más detalles):

    ntfy user list                     # Shows list of users (alias: 'ntfy access')
    ntfy user add phil                 # Add regular user phil  
    ntfy user add --role=admin phil    # Add admin user phil
    ntfy user del phil                 # Delete user phil
    ntfy user change-pass phil         # Change password for user phil
    ntfy user change-role phil admin   # Make user phil an admin

### Lista de control de acceso (ACL)

La lista de control de acceso (ACL) **administra el acceso a los temas para los usuarios que no son administradores y para el acceso anónimo (`everyone`/`*`)**.
Cada entrada representa los permisos de acceso de un usuario a un tema o patrón de tema específico.

La ACL se puede mostrar o modificar con el `ntfy access` mandar:

    ntfy access                            # Shows access control list (alias: 'ntfy user list')
    ntfy access USERNAME                   # Shows access control entries for USERNAME
    ntfy access USERNAME TOPIC PERMISSION  # Allow/deny access for USERNAME to TOPIC

Un `USERNAME` es un usuario existente, tal como se creó con `ntfy user add` (ver [usuarios y roles](#users-and-roles)), o el
usuario anónimo `everyone` o `*`, que representa a los clientes que acceden a la API sin nombre de usuario/contraseña.

Un `TOPIC` es un nombre de tema específico (por ejemplo, `mytopic`o `phil_alerts`), o un patrón comodín que coincida con cualquier
número de temas (por ejemplo, `alerts_*` o `ben-*`). Sólo el carácter comodín `*` es compatible. Significa cero a cualquier
número de caracteres.

Un `PERMISSION` es cualquiera de los siguientes permisos admitidos:

*   `read-write` (alias: `rw`): Permite [publicar mensajes](publish.md) al tema dado, así como
    [Suscribirse](subscribe/api.md) y lectura de mensajes
*   `read-only` (alias: `read`, `ro`): Permite solo suscribirse y leer mensajes, pero no publicar en el tema
*   `write-only` (alias: `write`, `wo`): Permite publicar solo en el tema, pero no suscribirse a él
*   `deny` (alias: `none`): No permite publicar ni suscribirse a un tema

**Comandos de ejemplo** (tipo `ntfy access --help` para más detalles):

    ntfy access                        # Shows entire access control list
    ntfy access phil                   # Shows access for user phil
    ntfy access phil mytopic rw        # Allow read-write access to mytopic for user phil
    ntfy access everyone mytopic rw    # Allow anonymous read-write access to mytopic
    ntfy access everyone "up*" write   # Allow anonymous write-only access to topics "up..."
    ntfy access --reset                # Reset entire access control list
    ntfy access --reset phil           # Reset all access for user phil
    ntfy access --reset phil mytopic   # Reset access for user phil and topic mytopic

**Ejemplo de ACL:**

    $ ntfy access
    user phil (admin)
    - read-write access to all topics (admin role)
    user ben (user)
    - read-write access to topic garagedoor
    - read-write access to topic alerts*
    - read-only access to topic furnace
    user * (anonymous)
    - read-only access to topic announcements
    - read-only access to topic server-stats
    - no access to any (other) topics (server config)

En este ejemplo, `phil` tiene el rol `admin`, por lo que tiene acceso de lectura y escritura a todos los temas (no se necesitan entradas de ACL).
Usuario `ben` tiene tres entradas específicas del tema. Puede leer, pero no escribir sobre el tema `furnace`, y tiene acceso de lectura y escritura
al tema `garagedoor` y todos los temas que comienzan con la palabra `alerts` (comodines). Clientes que no están autenticados
(llamado `*`/`everyone`) sólo tienen acceso de lectura a la `announcements` y `server-stats` Temas.

### Ejemplo: Instancia privada

La forma más sencilla de configurar una instancia privada es establecer `auth-default-access` Para `deny-all` En `server.yml`:

\=== "/etc/ntfy/server.yml"
` yaml
    auth-file: "/var/lib/ntfy/user.db"
    auth-default-access: "deny-all"
    `

Después de eso, simplemente cree un `admin` usuario:

    $ ntfy user add --role=admin phil
    password: mypass
    confirm: mypass
    user phil added with role admin 

Una vez que haya hecho eso, puede publicar y suscribirse usando [Autenticación básica](https://en.wikipedia.org/wiki/Basic_access_authentication)
con el nombre de usuario/contraseña dado. Asegúrese de usar HTTPS para evitar espiar y exponer su contraseña. Aquí hay un ejemplo simple:

\=== "Línea de comandos (curl)"
`     curl \         -u phil:mypass \         -d "Look ma, with auth" \
        https://ntfy.example.com/mysecrets
    `

\=== "ntfy CLI"
`     ntfy publish \         -u phil:mypass \
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

## Notificaciones por correo electrónico

Para permitir el reenvío de mensajes por correo electrónico, puede configurar un **Servidor SMTP para mensajes salientes**. Una vez configurado,
Puede establecer el `X-Email` encabezado a [enviar mensajes por correo electrónico](publish.md#e-mail-notifications) (por ejemplo,
`curl -d "hi there" -H "X-Email: phil@example.com" ntfy.sh/mytopic`).

A partir de hoy, solo se admiten servidores SMTP con autenticación PLAIN y STARTLS. Para habilitar el envío de correo electrónico, debe establecer el
siguientes configuraciones:

*   `base-url` es la URL raíz del servidor ntfy; Esto es necesario para el pie de página del correo electrónico
*   `smtp-sender-addr` es el nombre de host:puerto del servidor SMTP
*   `smtp-sender-user` y `smtp-sender-pass` son el nombre de usuario y la contraseña del usuario SMTP
*   `smtp-sender-from` es la dirección de correo electrónico del remitente

Aquí hay un ejemplo de configuración usando [Amazon SES](https://aws.amazon.com/ses/) para el correo saliente (así es como es
configurado para `ntfy.sh`):

\=== "/etc/ntfy/server.yml"
` yaml
    base-url: "https://ntfy.sh"
    smtp-sender-addr: "email-smtp.us-east-2.amazonaws.com:587"
    smtp-sender-user: "AKIDEADBEEFAFFE12345"
    smtp-sender-pass: "Abd13Kf+sfAk2DzifjafldkThisIsNotARealKeyOMG."
    smtp-sender-from: "ntfy@ntfy.sh"
    `

Por favor, consulte también el [limitación de velocidad](#rate-limiting) configuración a continuación, específicamente `visitor-email-limit-burst`
y `visitor-email-limit-burst`. Establecerlos de manera conservadora es necesario para evitar el abuso.

## Publicación de correo electrónico

Para permitir la publicación de mensajes por correo electrónico, ntfy puede ejecutar un **Servidor SMTP para mensajes entrantes**. Una vez configurado,
los usuarios pueden [enviar correos electrónicos a una dirección de correo electrónico de tema](publish.md#e-mail-publishing) (por ejemplo, `mytopic@ntfy.sh` o
`myprefix-mytopic@ntfy.sh`) para publicar mensajes en un tema. Esto es útil para integraciones basadas en correo electrónico, como
statuspage.io (aunque en estos días la mayoría de los servicios también admiten webhooks y llamadas HTTP).

Para configurar el servidor SMTP, debe al menos establecer `smtp-server-listen` y `smtp-server-domain`:

*   `smtp-server-listen` define la dirección IP y el puerto en el que escuchará el servidor SMTP, por ejemplo. `:25` o `1.2.3.4:25`
*   `smtp-server-domain` es el dominio de correo electrónico, por ejemplo. `ntfy.sh` (debe ser idéntico al registro MX, ver más abajo)
*   `smtp-server-addr-prefix` es un prefijo opcional para las direcciones de correo electrónico para evitar el spam. Si se establece en `ntfy-`por ejemplo
    sólo envía correos electrónicos a `ntfy-$topic@ntfy.sh` será aceptado. Si esto no está configurado, todos los correos electrónicos a `$topic@ntfy.sh` será
    aceptado (que obviamente puede ser un problema de spam).

Aquí hay una configuración de ejemplo (así es como se configura para `ntfy.sh`):

\=== "/etc/ntfy/server.yml"
` yaml
    smtp-server-listen: ":25"
    smtp-server-domain: "ntfy.sh"
    smtp-server-addr-prefix: "ntfy-"
    `

Además de configurar el servidor ntfy, debe crear dos registros DNS (un [Registro MX](https://en.wikipedia.org/wiki/MX_record)
y un registro A correspondiente), por lo que el correo entrante llegará a su servidor. Aquí hay un ejemplo de cómo `ntfy.sh` es
configurado (en [Ruta de Amazon 53](https://aws.amazon.com/route53/)):

<figure markdown>
  ![DNS records for incoming mail](static/img/screenshot-email-publishing-dns.png){ width=600 }
  <figcaption>DNS records for incoming mail</figcaption>
</figure>

Puede verificar si todo funciona correctamente enviando un correo electrónico como SMTP sin procesar a través de `nc`. Crear un archivo de texto, por ejemplo.
`email.txt`

    EHLO example.com
    MAIL FROM: phil@example.com
    RCPT TO: ntfy-mytopic@ntfy.sh
    DATA
    Subject: Email for you
    Content-Type: text/plain; charset="UTF-8"

    Hello from 🇩🇪
    .

Y luego enviar el correo a través de `nc` Así. Si ve alguna línea que comience por `451`, esos son errores de la
Servidor ntfy. Léalos detenidamente.

    $ cat email.txt | nc -N ntfy.sh 25
    220 ntfy.sh ESMTP Service Ready
    250-Hello example.com
    ...
    250 2.0.0 Roger, accepting mail from <phil@example.com>
    250 2.0.0 I'll make sure <ntfy-mytopic@ntfy.sh> gets this

En cuanto a la configuración de DNS, asegúrese de verificar que `dig MX` y `dig A` están devolviendo resultados similares a este:

    $ dig MX ntfy.sh +short 
    10 mx1.ntfy.sh.
    $ dig A mx1.ntfy.sh +short 
    3.139.215.220

## Detrás de un proxy (TLS, etc.)

!!! advertencia
Si está ejecutando ntfy detrás de un proxy, debe establecer el `behind-proxy` bandera. De lo contrario, todos los visitantes son
[tarifa limitada](#rate-limiting) como si fueran uno.

Puede ser deseable ejecutar ntfy detrás de un proxy (por ejemplo, nginx, HAproxy o Apache), para que pueda proporcionar certificados TLS
usando Let's Encrypt usando certbot, o simplemente porque desea compartir los puertos (80/443) con otros servicios.
Cualesquiera que sean sus razones, hay algunas cosas a considerar.

Si está ejecutando ntfy detrás de un proxy, debe establecer el `behind-proxy` bandera. Esto instruirá al
[limitación de velocidad](#rate-limiting) lógica para utilizar el `X-Forwarded-For` encabezado como identificador principal para un visitante,
a diferencia de la dirección IP remota. Si el `behind-proxy` la bandera no está configurada, todos los visitantes lo harán
se cuentan como uno, porque desde la perspectiva del servidor ntfy, todos comparten la dirección IP del proxy.

\=== "/etc/ntfy/server.yml"
` yaml     # Tell ntfy to use "X-Forwarded-For" to identify visitors
    behind-proxy: true
    `

### TLS/SSL

Ntfy admite HTTPS/TLS estableciendo el `listen-https` [opción de configuración](#config-options). Sin embargo, si usted
están detrás de un proxy, se recomienda que la terminación TLS / SSL sea realizada por el propio proxy (ver más abajo).

Recomiendo encarecidamente usar [certbot](https://certbot.eff.org/). Lo uso con el [Complemento dns-route53](https://certbot-dns-route53.readthedocs.io/en/stable/),
que le permite usar [Ruta 53 de AWS](https://aws.amazon.com/route53/) como el reto. Eso es mucho más fácil que usar el
Desafío HTTP. He encontrado [esta guía](https://nandovieira.com/using-lets-encrypt-in-development-with-nginx-and-aws-route53) Para
ser increíblemente útil.

### nginx/Apache2/caddy

Para su comodidad, aquí hay una configuración de trabajo que ayudará a configurar las cosas detrás de un proxy. Asegúrese de **habilitar WebSockets**
reenviando el `Connection` y `Upgrade` encabezados en consecuencia.

En este ejemplo, ntfy se ejecuta en `:2586` y le enviamos el tráfico. También redirigimos HTTP a HTTPS para solicitudes GET contra un tema
o el dominio raíz:

\=== "nginx (/etc/nginx/sites-\*/ntfy)"
\`\`\`
servidor {
escuchar 80;
server_name ntfy.sh;

      location / {
        # Redirect HTTP to HTTPS, but only for GET topic addresses, since we want 
        # it to work with curl without the annoying https:// prefix
        set $redirect_https "";
        if ($request_method = GET) {
          set $redirect_https "yes";
        }
        if ($request_uri ~* "^/([-_a-z0-9]{0,64}$|docs/|static/)") {
          set $redirect_https "${redirect_https}yes";
        }
        if ($redirect_https = "yesyes") {
          return 302 https://$http_host$request_uri$is_args$query_string;
        }

        proxy_pass http://127.0.0.1:2586;
        proxy_http_version 1.1;

        proxy_buffering off;
        proxy_request_buffering off;
        proxy_redirect off;
     
        proxy_set_header Host $http_host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        proxy_connect_timeout 3m;
        proxy_send_timeout 3m;
        proxy_read_timeout 3m;

        client_max_body_size 20m; # Must be >= attachment-file-size-limit in /etc/ntfy/server.yml
      }
    }

    server {
      listen 443 ssl;
      server_name ntfy.sh;

      ssl_session_cache builtin:1000 shared:SSL:10m;
      ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
      ssl_ciphers HIGH:!aNULL:!eNULL:!EXPORT:!CAMELLIA:!DES:!MD5:!PSK:!RC4;
      ssl_prefer_server_ciphers on;

      ssl_certificate /etc/letsencrypt/live/ntfy.sh/fullchain.pem;
      ssl_certificate_key /etc/letsencrypt/live/ntfy.sh/privkey.pem;

      location / {
        proxy_pass http://127.0.0.1:2586;
        proxy_http_version 1.1;

        proxy_buffering off;
        proxy_request_buffering off;
        proxy_redirect off;
     
        proxy_set_header Host $http_host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        proxy_connect_timeout 3m;
        proxy_send_timeout 3m;
        proxy_read_timeout 3m;
        
        client_max_body_size 20m; # Must be >= attachment-file-size-limit in /etc/ntfy/server.yml
      }
    }
    ```

\=== "Apache2 (/etc/apache2/sites-\*/ntfy.conf)"
\`\`\`
\<VirtualHost \*:80>
ServerName ntfy.sh

        # Proxy connections to ntfy (requires "a2enmod proxy")
        ProxyPass / http://127.0.0.1:2586/
        ProxyPassReverse / http://127.0.0.1:2586/

        SetEnv proxy-nokeepalive 1
        SetEnv proxy-sendchunked 1

        # Higher than the max message size of 4096 bytes
        LimitRequestBody 102400

        # Enable mod_rewrite (requires "a2enmod rewrite")
        RewriteEngine on

        # WebSockets support (requires "a2enmod rewrite proxy_wstunnel")
        RewriteCond %{HTTP:Upgrade} websocket [NC]
        RewriteCond %{HTTP:Connection} upgrade [NC]
        RewriteRule ^/?(.*) "ws://127.0.0.1:2586/$1" [P,L]
        
        # Redirect HTTP to HTTPS, but only for GET topic addresses, since we want 
        # it to work with curl without the annoying https:// prefix 
        RewriteCond %{REQUEST_METHOD} GET
        RewriteRule ^/([-_A-Za-z0-9]{0,64})$ https://%{SERVER_NAME}/$1 [R,L]
    </VirtualHost>

    <VirtualHost *:443>
        ServerName ntfy.sh
        
        SSLEngine on
        SSLCertificateFile /etc/letsencrypt/live/ntfy.sh/fullchain.pem
        SSLCertificateKeyFile /etc/letsencrypt/live/ntfy.sh/privkey.pem
        Include /etc/letsencrypt/options-ssl-apache.conf

        # Proxy connections to ntfy (requires "a2enmod proxy")
        ProxyPass / http://127.0.0.1:2586/
        ProxyPassReverse / http://127.0.0.1:2586/

        SetEnv proxy-nokeepalive 1
        SetEnv proxy-sendchunked 1

        # Higher than the max message size of 4096 bytes 
        LimitRequestBody 102400

        # Enable mod_rewrite (requires "a2enmod rewrite")
        RewriteEngine on

        # WebSockets support (requires "a2enmod rewrite proxy_wstunnel")
        RewriteCond %{HTTP:Upgrade} websocket [NC]
        RewriteCond %{HTTP:Connection} upgrade [NC]
        RewriteRule ^/?(.*) "ws://127.0.0.1:2586/$1" [P,L] 
    </VirtualHost>
    ```

\=== "caddy"
\`\`\`
\# Tenga en cuenta que esta configuración es ciertamente incompleta. Por favor, ayúdame y hazme saber lo que falta
\# a través de Discord/Matrix o en un problema de GitHub.

    ntfy.sh, http://nfty.sh {
        reverse_proxy 127.0.0.1:2586

        # Redirect HTTP to HTTPS, but only for GET topic addresses, since we want
        # it to work with curl without the annoying https:// prefix
        @httpget {
            protocol http
            method GET
            path_regexp ^/([-_a-z0-9]{0,64}$|docs/|static/)
        }
        redir @httpget https://{host}{uri}
    }
    ```

## Base de fuego (FCM)

!!! información
El uso de Firebase es **opcional** y solo funciona si modifica y [crea tu propio .apk Android](develop.md#android-app).
Para una instancia autohospedada, es más fácil simplemente no molestarse con FCM.

[Mensajería en la nube de Firebase (FCM)](https://firebase.google.com/docs/cloud-messaging) es la forma aprobada por Google para enviar
enviar mensajes a dispositivos Android. FCM es el único método que una aplicación de Android puede recibir mensajes sin tener que ejecutar un
[servicio en primer plano](https://developer.android.com/guide/components/foreground-services).

Para el host principal [ntfy.sh](https://ntfy.sh)el [Aplicación ntfy para Android](subscribe/phone.md) utiliza Firebase para enviar mensajes
al dispositivo. Para otros hosts, se utiliza la entrega instantánea y FCM no está involucrado.

Para configurar FCM para la instancia autohospedada del servidor ntfy, siga estos pasos:

1.  Regístrese para un [Cuenta de Firebase](https://console.firebase.google.com/)
2.  Crea una aplicación de Firebase y descarga el archivo de claves (por ejemplo, `myapp-firebase-adminsdk-...json`)
3.  Coloque el archivo de clave en `/etc/ntfy`, establezca el `firebase-key-file` en `server.yml` en consecuencia, reinicie el servidor NTFY
4.  Crea tu propio Android .apk siguiente [Estas instrucciones](develop.md#android-app)

Ejemplo:

    # If set, also publish messages to a Firebase Cloud Messaging (FCM) topic for your app.
    # This is optional and only required to support Android apps (which don't allow background services anymore).
    #
    firebase-key-file: "/etc/ntfy/ntfy-sh-firebase-adminsdk-ahnce-9f4d6f14b5.json"

## Notificaciones instantáneas de iOS

A diferencia de Android, iOS restringe en gran medida el procesamiento en segundo plano, lo que lamentablemente hace que sea imposible implementar el procesamiento instantáneo.
notificaciones push sin un servidor central.

Para seguir admitiendo notificaciones instantáneas en iOS a través de su servidor ntfy autohospedado, debe reenviar los llamados `poll_request`
mensajes al servidor principal de ntfy.sh (o a cualquier servidor ascendente que esté conectado a APNS/Firebase, si creas tu propia aplicación iOS),
que luego lo reenviará a Firebase/APNS.

Para configurarlo, simplemente configure `upstream-base-url` de esta manera:

```yaml
upstream-base-url: "https://ntfy.sh"
```

Si se establece, todos los mensajes entrantes publicarán una solicitud de sondeo en el servidor ascendente configurado, que contiene
el identificador de mensaje del mensaje original, que indica a la aplicación iOS que sondee este servidor en busca del contenido real del mensaje.

Si `upstream-base-url` no está configurado, las notificaciones llegarán eventualmente a su dispositivo, pero la entrega puede tardar horas,
dependiendo del estado del teléfono. Sin embargo, si está usando su teléfono, no debería tomar más de 20-30 minutos.

En caso de que tengas curiosidad, aquí hay un ejemplo de todo el flujo:

*   En la aplicación iOS, te suscribes a `https://ntfy.example.com/mytopic`
*   La aplicación se suscribe al tema de Firebase `6de73be8dfb7d69e...` (el SHA256 de la URL del tema)
*   Al publicar un mensaje en `https://ntfy.example.com/mytopic`, el servidor NTFY publicará un
    solicitud de sondeo a `https://ntfy.sh/6de73be8dfb7d69e...`. La solicitud de su servidor al servidor ascendente
    contiene sólo el identificador de mensaje (en el cuadro de diálogo `X-Poll-ID` encabezado), y la suma de comprobación SHA256 de la dirección URL del tema (como tema ascendente).
*   El servidor de ntfy.sh publica el mensaje de solicitud de sondeo en Firebase, que lo reenvía a APNS, que lo reenvía a tu dispositivo iOS.
*   Su dispositivo iOS recibe la solicitud de sondeo y obtiene el mensaje real de su servidor y, a continuación, lo muestra

Aquí hay un ejemplo de lo que el servidor autohospedado reenvía al servidor ascendente. La solicitud es equivalente a este rizo:

    curl -X POST -H "X-Poll-ID: s4PdJozxM8na" https://ntfy.sh/6de73be8dfb7d69e32fb2c00c23fe7adbd8b5504406e3068c273aa24cef4055b
    {"id":"4HsClFEuCIcs","time":1654087955,"event":"poll_request","topic":"6de73be8dfb7d69e32fb2c00c23fe7adbd8b5504406e3068c273aa24cef4055b","message":"New message","poll_id":"s4PdJozxM8na"}

## Limitación de velocidad

!!! información
Tenga en cuenta que si está ejecutando ntfy detrás de un proxy, debe establecer el `behind-proxy` bandera.
De lo contrario, todos los visitantes están limitados por la tarifa como si fueran uno.

De forma predeterminada, ntfy se ejecuta sin autenticación, por lo que es de vital importancia que protejamos el servidor del abuso o la sobrecarga.
Existen varios límites y límites de velocidad que puede utilizar para configurar el servidor:

*   **Límite global**: Se aplica un límite global a todos los visitantes (IP, clientes, usuarios)
*   **Límite de visitantes**: Un límite de visitantes solo se aplica a un determinado visitante. Un **visitante** se identifica por su dirección IP
    (o el `X-Forwarded-For` encabezado si `behind-proxy` está configurado). Todas las opciones de configuración que comienzan con la palabra `visitor` aplicar
    solo por visitante.

Durante el uso normal, no debería encontrar estos límites en absoluto, e incluso si revienta algunas solicitudes o correos electrónicos.
(por ejemplo, cuando se vuelve a conectar después de una caída de la conexión), no debería tener ningún efecto.

### Límites generales

Hagamos primero los límites fáciles:

*   `global-topic-limit` define el número total de temas antes de que el servidor rechace nuevos temas. El valor predeterminado es 15.000.
*   `visitor-subscription-limit` es el número de suscripciones (conexiones abiertas) por visitante. Este valor predeterminado es 30.

### Límites de solicitud

Además de los límites anteriores, hay un límite de solicitudes / segundo por visitante para todas las solicitudes sensibles GET / PUT / POST.
Este límite utiliza un [bucket de tokens](https://en.wikipedia.org/wiki/Token_bucket) (usando Go's [paquete de tarifas](https://pkg.go.dev/golang.org/x/time/rate)):

Cada visitante tiene un bucket de 60 solicitudes que pueden disparar contra el servidor (definidas por `visitor-request-limit-burst`).
Después de los 60, las nuevas solicitudes se encontrarán con un `429 Too Many Requests` respuesta. El cubo de solicitud de visitante se rellena a una velocidad de uno
solicitud cada 5s (definida por `visitor-request-limit-replenish`)

*   `visitor-request-limit-burst` es el cubo inicial de solicitudes que tiene cada visitante. El valor predeterminado es 60.
*   `visitor-request-limit-replenish` es la velocidad a la que se rellena el bucket (una solicitud por x). El valor predeterminado es 5s.
*   `visitor-request-limit-exempt-hosts` es una lista separada por comas de nombres de host e IP que deben estar exentos de la tasa de solicitud
    limitante; los nombres de host se resuelven en el momento en que se inicia el servidor. El valor predeterminado es una lista vacía.

### Límites de datos adjuntos

Aparte del tamaño global del archivo y los límites totales de caché de datos adjuntos (consulte [encima](#attachments)), hay dos relevantes
Límites por visitante:

*   `visitor-attachment-total-size-limit` es el límite total de almacenamiento utilizado para los archivos adjuntos por visitante. El valor predeterminado es 100M.
    El almacenamiento por visitante se reduce automáticamente a medida que expiran los archivos adjuntos. Accesorios externos (conectados a través de `X-Attach`,
    ver [publicar documentos](publish.md#attachments)) no cuentan aquí.
*   `visitor-attachment-daily-bandwidth-limit` es el límite total diario de ancho de banda de descarga/carga de archivos adjuntos por visitante,
    incluidas las solicitudes PUT y GET. Esto es para proteger su valioso ancho de banda del abuso, ya que la salida cuesta dinero en
    la mayoría de los proveedores de nube. El valor predeterminado es 500M.

### Límites de correo electrónico

De manera similar al límite de solicitudes, también hay un límite de correo electrónico (solo relevante si [notificaciones por correo electrónico](#e-mail-notifications)
están habilitados):

*   `visitor-email-limit-burst` es el cubo inicial de correos electrónicos que tiene cada visitante. El valor predeterminado es 16.
*   `visitor-email-limit-replenish` es la velocidad a la que se rellena el bucket (un correo electrónico por x). El valor predeterminado es 1h.

### Límites de Firebase

Si [Firebase está configurado](#firebase-fcm), todos los mensajes también se publican en un tema de Firebase (a menos que `Firebase: no`
está configurado). Firebase aplica [sus propios límites](https://firebase.google.com/docs/cloud-messaging/concept-options#topics_throttling)
sobre cuántos mensajes se pueden publicar. Desafortunadamente, estos límites son un poco vagos y pueden cambiar dependiendo del tiempo.
del día. En la práctica, solo he observado `429 Quota exceeded` respuestas de Firebase si **Se publican demasiados mensajes en
el mismo tema**.

En ntfy, si Firebase responde con un 429 después de publicar en un tema, el visitante (= dirección IP) que publicó el mensaje
es **se prohibió publicar en Firebase durante 10 minutos** (no configurable). Debido a que la publicación en Firebase se realiza de forma asincrónica,
no hay ninguna indicación del usuario de que esto haya sucedido. Los suscriptores que no son de Firebase (WebSocket o HTTP stream) no se ven afectados.
Después de los 10 minutos, se reanuda el reenvío de mensajes a Firebase para este visitante.

Si esto sucede alguna vez, habrá un mensaje de registro que se ve algo como esto:

    WARN Firebase quota exceeded (likely for topic), temporarily denying Firebase access to visitor

## Ajuste para escala

Si está ejecutando ntfy para su servidor doméstico, probablemente no tenga que preocuparse por la escala en absoluto. En su configuración predeterminada,
Si no está detrás de un proxy, el servidor NTFY puede mantener aproximadamente **tantas conexiones como permita el límite de archivos abiertos**.
Este límite se suele denominar `nofile`. Aparte de eso, la RAM y la CPU son obviamente relevantes. También es posible que desee verificar
fuera [esta discusión en Reddit](https://www.reddit.com/r/golang/comments/r9u4ee/how_many_actively_connected_http_clients_can_a_go/).

Según *cómo lo ejecuta*, aquí hay algunos límites que son relevantes:

### WAL para caché de mensajes

De forma predeterminada, el [caché de mensajes](#message-cache) (definido por `cache-file`) utiliza la configuración predeterminada de SQLite, lo que significa que
se sincroniza con el disco en cada escritura. Para servidores personales, esto es perfectamente adecuado. Para instalaciones más grandes, como ntfy.sh,
el [registro de escritura anticipada (WAL)](https://sqlite.org/wal.html) debe estar habilitado y el modo de sincronización debe ajustarse.
Ver [este artículo](https://phiresky.github.io/blog/2020/sqlite-performance-tuning/) para más detalles.

Así es como se ha sintonizado ntfy.sh en el `server.yml` archivo:

```yaml
cache-startup-queries: |
    pragma journal_mode = WAL;
    pragma synchronous = normal;
    pragma temp_store = memory;
```

### Para servicios systemd

Si está ejecutando ntfy en un servicio systemd (por ejemplo, para paquetes de .deb/.rpm), el principal factor limitante es el
`LimitNOFILE` en la unidad systemd. El límite predeterminado de archivos abiertos para `ntfy.service` es 10.000. Puede anularlo
mediante la creación de un `/etc/systemd/system/ntfy.service.d/override.conf` archivo. Por lo que puedo decir, `/etc/security/limits.conf`
no es relevante.

\=== "/etc/systemd/system/ntfy.service.d/override.conf"
`     # Allow 20,000 ntfy connections (and give room for other file handles)     [Service]
    LimitNOFILE=20500
    `

### Fuera de systemd

Si está ejecutando fuera de systemd, es posible que desee ajustar su `/etc/security/limits.conf` archivo a
aumentar el `nofile` ajuste. Aquí hay un ejemplo que aumenta el límite a 5,000. Puede averiguar la configuración actual
corriendo `ulimit -n`, o anule manualmente temporalmente ejecutando `ulimit -n 50000`.

\=== "/etc/security/limits.conf"
`     # Increase open files limit globally     * hard nofile 20500
    `

### Límites de proxy (nginx, Apache2)

Si estás corriendo [detrás de un proxy](#behind-a-proxy-tls-etc) (por ejemplo, nginx, Apache), el límite de archivos abiertos del proxy también es
pertinente. Entonces, si su proxy se ejecuta dentro de systemd, aumente los límites en systemd para el proxy. Normalmente, el proxy
El límite de archivos abiertos tiene que ser **Duplique el número de conexiones que desea admitir**, porque el proxy tiene
para mantener la conexión del cliente y la conexión con ntfy.

\=== "/etc/nginx/nginx.conf"
`     events {       # Allow 40,000 proxy connections (2x of the desired ntfy connection count;       # and give room for other file handles)
      worker_connections 40500;
    }
    `

\=== "/etc/systemd/system/nginx.service.d/override.conf"
`     # Allow 40,000 proxy connections (2x of the desired ntfy connection count;     # and give room for other file handles)     [Service]
    LimitNOFILE=40500
    `

### Prohibir a los malos actores (fail2ban)

Si pones cosas en Internet, los malos actores intentarán romperlas o irrumpir. [fail2ban](https://www.fail2ban.org/)
y nginx's [ngx_http_limit_req_module módulo](http://nginx.org/en/docs/http/ngx_http_limit_req_module.html) se puede utilizar
prohibir las IP de los clientes si se comportan mal. Esto está en la parte superior de la [limitación de velocidad](#rate-limiting) dentro del servidor ntfy.

Aquí hay un ejemplo de cómo se configura ntfy.sh, siguiendo las instrucciones de dos tutoriales ([aquí](https://easyengine.io/tutorials/nginx/fail2ban/)
y [aquí](https://easyengine.io/tutorials/nginx/block-wp-login-php-bruteforce-attack/)):

\=== "/etc/nginx/nginx.conf"
`     http {
	  limit_req_zone $binary_remote_addr zone=one:10m rate=1r/s;
    }
    `

\=== "/etc/nginx/sites-enabled/ntfy.sh"
`     # For each server/location block
    server {
      location / {
        limit_req zone=one burst=1000 nodelay;
      }
    }    
    `

\=== "/etc/fail2ban/filter.d/nginx-req-limit.conf"
`     [Definition]
    failregex = limiting requests, excess:.* by zone.*client: <HOST>
    ignoreregex =
    `

\=== "/etc/fail2ban/jail.local"
`     [nginx-req-limit]
    enabled = true
    filter = nginx-req-limit
    action = iptables-multiport[name=ReqLimit, port="http,https", protocol=tcp]
    logpath = /var/log/nginx/error.log
    findtime = 600
    bantime = 7200
    maxretry = 10
    `

## Depuración/rastreo

Si algo no funciona correctamente, puede depurar / rastrear lo que está haciendo el servidor ntfy configurando el `log-level`
Para `DEBUG` o `TRACE`. El `DEBUG` Establecer generará información sobre cada mensaje publicado, pero no el mensaje
contenido. El `TRACE` también imprimirá el contenido del mensaje.

!!! advertencia
Ambas opciones son muy detalladas y solo deben habilitarse en producción durante cortos períodos de tiempo. De otra manera
te vas a quedar sin espacio en disco bastante rápido.

También puede recargar en caliente el `log-level` enviando el `SIGHUP` señal al proceso después de editar el `server.yml` archivo.
Puedes hacerlo llamando `systemctl reload ntfy` (si ntfy se ejecuta dentro de systemd), o llamando `kill -HUP $(pidof ntfy)`.
Si tiene éxito, verá algo como esto:

    $ ntfy serve
    2022/06/02 10:29:28 INFO Listening on :2586[http] :1025[smtp], log level is INFO
    2022/06/02 10:29:34 INFO Partially hot reloading configuration ...
    2022/06/02 10:29:34 INFO Log level is TRACE

## Opciones de configuración

Cada opción de configuración se puede establecer en el archivo de configuración `/etc/ntfy/server.yml` (por ejemplo, `listen-http: :80`) o como
Opción CLI (por ejemplo, `--listen-http :80`. Aquí hay una lista de todas las opciones disponibles. Como alternativa, puede establecer un entorno
antes de ejecutar el `ntfy` comando (por ejemplo, `export NTFY_LISTEN_HTTP=:80`).

!!! información
Todas las opciones de configuración también se pueden definir en el `server.yml` archivo que utiliza guiones bajos en lugar de guiones, por ejemplo.
`cache_duration` y `cache-duration` ambos son compatibles. Esto es para admitir analizadores YAML más estrictos que lo hacen.
no soporta guiones.

| | de opciones de configuración | de variables Env Formato | | predeterminada Descripción                                                                                                                                                                                                                     |
|--------------------------------------------|-------------------------------------------------|-----------------------------------------------------|-------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `base-url`                                 | `NTFY_BASE_URL`                                 | *URL*                                               | -                 | URL base pública del servicio (por ejemplo, `https://ntfy.sh`)                                                                                                                                                                  |
| `listen-http`                              | `NTFY_LISTEN_HTTP`                              | `[host]:port`                                       | `:80`             | Dirección de escucha para el servidor web HTTP |
| `listen-https`                             | `NTFY_LISTEN_HTTPS`                             | `[host]:port`                                       | -                 | Dirección de escucha para el servidor web HTTPS. Si está configurado, también debe configurar `key-file` y `cert-file`.                                                                                                                               |
| `listen-unix`                              | `NTFY_LISTEN_UNIX`                              | *Nombre*                                          | -                 | Ruta a un socket Unix para escuchar en |
| `key-file`                                 | `NTFY_KEY_FILE`                                 | *Nombre*                                          | -                 | Archivo de clave privada HTTPS/TLS, solo se usa si `listen-https` está configurado.                                                                                                                                                                 |
| `cert-file`                                | `NTFY_CERT_FILE`                                | *Nombre*                                          | -                 | Archivo de certificado HTTPS/TLS, solo se usa si `listen-https` está configurado.                                                                                                                                                                 |
| `firebase-key-file`                        | `NTFY_FIREBASE_KEY_FILE`                        | *Nombre*                                          | -                 | Si está configurado, publica también mensajes en un tema de Firebase Cloud Messaging (FCM) para tu aplicación. Esto es opcional y solo es necesario para ahorrar batería cuando se usa la aplicación de Android. Ver [Base de fuego (FCM)](#firebase-fcm).                        |
| `cache-file`                               | `NTFY_CACHE_FILE`                               | *Nombre*                                          | -                 | Si se establece, los mensajes se almacenan en caché en una base de datos SQLite local en lugar de solo en memoria. Esto permite reiniciar el servicio sin perder mensajes en apoyo del parámetro since=. Ver [caché de mensajes](#message-cache).             |
| `cache-duration`                           | `NTFY_CACHE_DURATION`                           | *duración*                                          | 12h | Duración durante la cual los mensajes se almacenarán en búfer antes de que se eliminen. Esto es necesario para apoyar el `since=...` y `poll=1` parámetro. Establezca esto en `0` para deshabilitar la memoria caché por completo.                                        |
| `cache-startup-queries`                    | `NTFY_CACHE_STARTUP_QUERIES`                    | *cadena (consultas SQL)*                              | -                 | Consultas SQL para ejecutar durante el inicio de la base de datos; Esto es útil para ajustar y [habilitar el modo WAL](#wal-for-message-cache)                                                                                                           |
| `auth-file`                                | `NTFY_AUTH_FILE`                                | *Nombre*                                          | -                 | Archivo de base de datos de autenticación utilizado para el control de acceso. Si se establece, habilita la autenticación y el control de acceso. Ver [control de acceso](#access-control).                                                                                           |
| `auth-default-access`                      | `NTFY_AUTH_DEFAULT_ACCESS`                      | `read-write`, `read-only`, `write-only`, `deny-all` | `read-write`      | Permisos predeterminados si no se encuentran entradas coincidentes en la base de datos de autenticación. El valor predeterminado es `read-write`.                                                                                                                             |
| `behind-proxy`                             | `NTFY_BEHIND_PROXY`                             | *Bool*                                              | | falso Si se establece, el encabezado X-Forwarded-For se utiliza para determinar la dirección IP del visitante en lugar de la dirección remota de la conexión.                                                                                                 |
| `attachment-cache-dir`                     | `NTFY_ATTACHMENT_CACHE_DIR`                     | *directorio*                                         | -                 | Directorio de caché para archivos adjuntos. Para habilitar los archivos adjuntos, esto debe establecerse.                                                                                                                                                  |
| `attachment-total-size-limit`              | `NTFY_ATTACHMENT_TOTAL_SIZE_LIMIT`              | *tamaño*                                              | | 5G Límite del directorio de caché de datos adjuntos en disco. Si se superan los límites, se rechazarán los nuevos archivos adjuntos.                                                                                                                   |
| `attachment-file-size-limit`               | `NTFY_ATTACHMENT_FILE_SIZE_LIMIT`               | *tamaño*                                              | 15M | Límite de tamaño de archivos adjuntos por archivo (por ejemplo, 300k, 2M, 100M). Se rechazará el archivo adjunto más grande.                                                                                                                                       |
| `attachment-expiry-duration`               | `NTFY_ATTACHMENT_EXPIRY_DURATION`               | *duración*                                          | 3h | Duración después de la cual se eliminarán los archivos adjuntos cargados (por ejemplo, 3h, 20h). Afecta fuertemente `visitor-attachment-total-size-limit`.                                                                                               |
| `smtp-sender-addr`                         | `NTFY_SMTP_SENDER_ADDR`                         | `host:port`                                         | -                 | Dirección del servidor SMTP para permitir el envío de correo electrónico |
| `smtp-sender-user`                         | `NTFY_SMTP_SENDER_USER`                         | *cuerda*                                            | -                 | Usuario SMTP; sólo se utiliza si el envío de correo electrónico está habilitado |
| `smtp-sender-pass`                         | `NTFY_SMTP_SENDER_PASS`                         | *cuerda*                                            | -                 | Contraseña SMTP; sólo se utiliza si el envío de correo electrónico está habilitado |
| `smtp-sender-from`                         | `NTFY_SMTP_SENDER_FROM`                         | *dirección de correo electrónico*                                    | -                 | Dirección de correo electrónico del remitente SMTP; sólo se utiliza si el envío de correo electrónico está habilitado |
| `smtp-server-listen`                       | `NTFY_SMTP_SERVER_LISTEN`                       | `[ip]:port`                                         | -                 | Define la dirección IP y el puerto en el que escuchará el servidor SMTP, por ejemplo. `:25` o `1.2.3.4:25`                                                                                                                                      |
| `smtp-server-domain`                       | `NTFY_SMTP_SERVER_DOMAIN`                       | *nombre de dominio*                                       | -                 | Dominio de correo electrónico del servidor SMTP, por ejemplo. `ntfy.sh`                                                                                                                                                                                       |
| `smtp-server-addr-prefix`                  | `NTFY_SMTP_SERVER_ADDR_PREFIX`                  | `[ip]:port`                                         | -                 | Prefijo opcional para las direcciones de correo electrónico para evitar el spam, por ejemplo. `ntfy-`                                                                                                                                                          |
| `keepalive-interval`                       | `NTFY_KEEPALIVE_INTERVAL`                       | *duración*                                          | 45s | Intervalo en el que se envían mensajes keepalive al cliente. Esto es para evitar que los intermediarios cierren la conexión por inactividad. Tenga en cuenta que la aplicación de Android tiene un tiempo de espera codificado en 77s, por lo que debería ser menor que eso. |
| `manager-interval`                         | `$NTFY_MANAGER_INTERVAL`                        | *duración*                                          | 1m de | Intervalo en el que el administrador poda los mensajes antiguos, elimina los temas e imprime las estadísticas.                                                                                                                                         |
| `global-topic-limit`                       | `NTFY_GLOBAL_TOPIC_LIMIT`                       | *número*                                            | 15.000 | Limitación de velocidad: número total de temas antes de que el servidor rechace nuevos temas.                                                                                                                                                     |
| `upstream-base-url`                        | `NTFY_UPSTREAM_BASE_URL`                        | *URL*                                               | `https://ntfy.sh` | Reenviar la solicitud de sondeo a un servidor ascendente, esto es necesario para las notificaciones push de iOS para servidores autohospedados |
| `visitor-attachment-total-size-limit`      | `NTFY_VISITOR_ATTACHMENT_TOTAL_SIZE_LIMIT`      | *tamaño*                                              | 100M | Limitación de velocidad: Límite de almacenamiento total utilizado para archivos adjuntos por visitante, para todos los archivos adjuntos combinados. El almacenamiento se libera después de que expiran los archivos adjuntos. Ver `attachment-expiry-duration`.                                                 |
| `visitor-attachment-daily-bandwidth-limit` | `NTFY_VISITOR_ATTACHMENT_DAILY_BANDWIDTH_LIMIT` | *tamaño*                                              | 500M | Limitación de velocidad: Límite total de tráfico diario de descarga/carga de archivos adjuntos por visitante. Esto es para proteger sus costos de ancho de banda de la explosión.                                                                                        |
| `visitor-email-limit-burst`                | `NTFY_VISITOR_EMAIL_LIMIT_BURST`                | *número*                                            | 16 | Limitación de velocidad: Límite inicial de correos electrónicos por visitante |
| `visitor-email-limit-replenish`            | `NTFY_VISITOR_EMAIL_LIMIT_REPLENISH`            | *duración*                                          | 1h | Limitación de velocidad: Fuertemente relacionado con `visitor-email-limit-burst`: La velocidad a la que se rellena el cubo |
| `visitor-request-limit-burst`              | `NTFY_VISITOR_REQUEST_LIMIT_BURST`              | *número*                                            | 60 | Limitación de velocidad: Solicitudes GET/PUT/POST permitidas por segundo, por visitante. Esta configuración es el grupo inicial de solicitudes que cada visitante ha |
| `visitor-request-limit-replenish`          | `NTFY_VISITOR_REQUEST_LIMIT_REPLENISH`          | *duración*                                          | 5s | Limitación de velocidad: Fuertemente relacionado con `visitor-request-limit-burst`: La velocidad a la que se rellena el cubo |
| `visitor-request-limit-exempt-hosts`       | `NTFY_VISITOR_REQUEST_LIMIT_EXEMPT_HOSTS`       | *lista de hosts/IP separados por comas*                      | -                 | Limitación de velocidad: Lista de nombres de host e IP que estarán exentos de la limitación de velocidad de solicitud |
| `visitor-subscription-limit`               | `NTFY_VISITOR_SUBSCRIPTION_LIMIT`               | *número*                                            | 30 | Limitación de velocidad: Número de suscripciones por visitante (dirección IP) |
| `web-root`                                 | `NTFY_WEB_ROOT`                                 | `app`, `home` o `disable`                          | `app`             | Establece la raíz web en la página de destino (inicio), la aplicación web (aplicación) o deshabilita la aplicación web por completo (deshabilitar) |

El formato de un *duración* es: `<number>(smh)`, por ejemplo, 30s, 20m o 1h.\
El formato de un *tamaño* es: `<number>(GMK)`, por ejemplo, 1G, 200M o 4000k.

## Opciones de línea de comandos

    $ ntfy serve --help
    NAME:
       ntfy serve - Run the ntfy server

    USAGE:
       ntfy serve [OPTIONS..]

    CATEGORY:
       Server commands

    DESCRIPTION:
       Run the ntfy server and listen for incoming requests
       
       The command will load the configuration from /etc/ntfy/server.yml. Config options can 
       be overridden using the command line options.
       
       Examples:
         ntfy serve                      # Starts server in the foreground (on port 80)
         ntfy serve --listen-http :8080  # Starts server with alternate port

    OPTIONS:
       --attachment-cache-dir value, --attachment_cache_dir value                                          cache directory for attached files [$NTFY_ATTACHMENT_CACHE_DIR]
       --attachment-expiry-duration value, --attachment_expiry_duration value, -X value                    duration after which uploaded attachments will be deleted (e.g. 3h, 20h) (default: 3h) [$NTFY_ATTACHMENT_EXPIRY_DURATION]
       --attachment-file-size-limit value, --attachment_file_size_limit value, -Y value                    per-file attachment size limit (e.g. 300k, 2M, 100M) (default: 15M) [$NTFY_ATTACHMENT_FILE_SIZE_LIMIT]
       --attachment-total-size-limit value, --attachment_total_size_limit value, -A value                  limit of the on-disk attachment cache (default: 5G) [$NTFY_ATTACHMENT_TOTAL_SIZE_LIMIT]
       --auth-default-access value, --auth_default_access value, -p value                                  default permissions if no matching entries in the auth database are found (default: "read-write") [$NTFY_AUTH_DEFAULT_ACCESS]
       --auth-file value, --auth_file value, -H value                                                      auth database file used for access control [$NTFY_AUTH_FILE]
       --base-url value, --base_url value, -B value                                                        externally visible base URL for this host (e.g. https://ntfy.sh) [$NTFY_BASE_URL]
       --behind-proxy, --behind_proxy, -P                                                                  if set, use X-Forwarded-For header to determine visitor IP address (for rate limiting) (default: false) [$NTFY_BEHIND_PROXY]
       --cache-duration since, --cache_duration since, -b since                                            buffer messages for this time to allow since requests (default: 12h0m0s) [$NTFY_CACHE_DURATION]
       --cache-file value, --cache_file value, -C value                                                    cache file used for message caching [$NTFY_CACHE_FILE]
       --cache-startup-queries value, --cache_startup_queries value                                        queries run when the cache database is initialized [$NTFY_CACHE_STARTUP_QUERIES]
       --cert-file value, --cert_file value, -E value                                                      certificate file, if listen-https is set [$NTFY_CERT_FILE]
       --config value, -c value                                                                            config file (default: /etc/ntfy/server.yml) [$NTFY_CONFIG_FILE]
       --debug, -d                                                                                         enable debug logging (default: false) [$NTFY_DEBUG]
       --firebase-key-file value, --firebase_key_file value, -F value                                      Firebase credentials file; if set additionally publish to FCM topic [$NTFY_FIREBASE_KEY_FILE]
       --global-topic-limit value, --global_topic_limit value, -T value                                    total number of topics allowed (default: 15000) [$NTFY_GLOBAL_TOPIC_LIMIT]
       --keepalive-interval value, --keepalive_interval value, -k value                                    interval of keepalive messages (default: 45s) [$NTFY_KEEPALIVE_INTERVAL]
       --key-file value, --key_file value, -K value                                                        private key file, if listen-https is set [$NTFY_KEY_FILE]
       --listen-http value, --listen_http value, -l value                                                  ip:port used to as HTTP listen address (default: ":80") [$NTFY_LISTEN_HTTP]
       --listen-https value, --listen_https value, -L value                                                ip:port used to as HTTPS listen address [$NTFY_LISTEN_HTTPS]
       --listen-unix value, --listen_unix value, -U value                                                  listen on unix socket path [$NTFY_LISTEN_UNIX]
       --log-level value, --log_level value                                                                set log level (default: "INFO") [$NTFY_LOG_LEVEL]
       --manager-interval value, --manager_interval value, -m value                                        interval of for message pruning and stats printing (default: 1m0s) [$NTFY_MANAGER_INTERVAL]
       --no-log-dates, --no_log_dates                                                                      disable the date/time prefix (default: false) [$NTFY_NO_LOG_DATES]
       --smtp-sender-addr value, --smtp_sender_addr value                                                  SMTP server address (host:port) for outgoing emails [$NTFY_SMTP_SENDER_ADDR]
       --smtp-sender-from value, --smtp_sender_from value                                                  SMTP sender address (if e-mail sending is enabled) [$NTFY_SMTP_SENDER_FROM]
       --smtp-sender-pass value, --smtp_sender_pass value                                                  SMTP password (if e-mail sending is enabled) [$NTFY_SMTP_SENDER_PASS]
       --smtp-sender-user value, --smtp_sender_user value                                                  SMTP user (if e-mail sending is enabled) [$NTFY_SMTP_SENDER_USER]
       --smtp-server-addr-prefix value, --smtp_server_addr_prefix value                                    SMTP email address prefix for topics to prevent spam (e.g. 'ntfy-') [$NTFY_SMTP_SERVER_ADDR_PREFIX]
       --smtp-server-domain value, --smtp_server_domain value                                              SMTP domain for incoming e-mail, e.g. ntfy.sh [$NTFY_SMTP_SERVER_DOMAIN]
       --smtp-server-listen value, --smtp_server_listen value                                              SMTP server address (ip:port) for incoming emails, e.g. :25 [$NTFY_SMTP_SERVER_LISTEN]
       --trace                                                                                             enable tracing (very verbose, be careful) (default: false) [$NTFY_TRACE]
       --upstream-base-url value, --upstream_base_url value                                                forward poll request to an upstream server, this is needed for iOS push notifications for self-hosted servers [$NTFY_UPSTREAM_BASE_URL]
       --visitor-attachment-daily-bandwidth-limit value, --visitor_attachment_daily_bandwidth_limit value  total daily attachment download/upload bandwidth limit per visitor (default: "500M") [$NTFY_VISITOR_ATTACHMENT_DAILY_BANDWIDTH_LIMIT]
       --visitor-attachment-total-size-limit value, --visitor_attachment_total_size_limit value            total storage limit used for attachments per visitor (default: "100M") [$NTFY_VISITOR_ATTACHMENT_TOTAL_SIZE_LIMIT]
       --visitor-email-limit-burst value, --visitor_email_limit_burst value                                initial limit of e-mails per visitor (default: 16) [$NTFY_VISITOR_EMAIL_LIMIT_BURST]
       --visitor-email-limit-replenish value, --visitor_email_limit_replenish value                        interval at which burst limit is replenished (one per x) (default: 1h0m0s) [$NTFY_VISITOR_EMAIL_LIMIT_REPLENISH]
       --visitor-request-limit-burst value, --visitor_request_limit_burst value                            initial limit of requests per visitor (default: 60) [$NTFY_VISITOR_REQUEST_LIMIT_BURST]
       --visitor-request-limit-exempt-hosts value, --visitor_request_limit_exempt_hosts value              hostnames and/or IP addresses of hosts that will be exempt from the visitor request limit [$NTFY_VISITOR_REQUEST_LIMIT_EXEMPT_HOSTS]
       --visitor-request-limit-replenish value, --visitor_request_limit_replenish value                    interval at which burst limit is replenished (one per x) (default: 5s) [$NTFY_VISITOR_REQUEST_LIMIT_REPLENISH]
       --visitor-subscription-limit value, --visitor_subscription_limit value                              number of subscriptions per visitor (default: 30) [$NTFY_VISITOR_SUBSCRIPTION_LIMIT]
       --web-root value, --web_root value                                                                  sets web root to landing page (home), web app (app) or disabled (disable) (default: "app") [$NTFY_WEB_ROOT]
