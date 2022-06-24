# Avisos de obsolescencia

Esta página se utiliza para enumerar los avisos de obsolescencia para ntfy. Los comandos y opciones obsoletos serán
**eliminado después de 1-3 meses** desde el momento en que quedaron en desuso. Cuánto tiempo la función está en desuso
antes de que se cambie el comportamiento depende de la gravedad del cambio y de lo prominente que sea la característica.

## Obsolescencias activas

### CLI ntfy: `ntfy publish --env-topic` se eliminará

> Activo desde 2022-06-20, el comportamiento cambiará al final de **Julio 2022**

El `ntfy publish --env-topic` se eliminará la opción. Todavía será posible especificar un tema a través del
`NTFY_TOPIC` variable de entorno, pero ya no será necesario especificar el `--env-topic` bandera.

\=== "Antes"
`     $ NTFY_TOPIC=mytopic ntfy publish --env-topic "this is the message"
    `

\=== "Después"
`     $ NTFY_TOPIC=mytopic ntfy publish "this is the message"
    `

## Obsolescencias anteriores

### <del>Aplicación Android: WebSockets se convertirá en el protocolo de conexión predeterminado</del>

> Activo desde 2022-03-13, el comportamiento no cambiará (desuso eliminado 2022-06-20)

Las conexiones de entrega instantánea y las conexiones a servidores autohospedados en la aplicación de Android iban a cambiar
para utilizar el protocolo WebSockets de forma predeterminada. Se decidió mantener json stream como el valor predeterminado más compatible
y agregue un banner de aviso en la aplicación de Android en su lugar.

### Aplicación Android: Uso `since=<timestamp>` En lugar de `since=<id>`

> Activo desde 2022-02-27, el comportamiento cambió con v1.14.0

La aplicación de Android comenzó a usar `since=<id>` En lugar de `since=<timestamp>`, lo que significa a partir de la aplicación de Android v1.14.0,
ya no funcionará con servidores anteriores a v1.16.0. Esto es para simplificar el manejo de la deduplicación en la aplicación de Android.

El `since=<timestamp>` endpoint seguirá funcionando. Esto es simplemente un aviso de que el comportamiento de la aplicación de Android cambiará.

### Ejecución del servidor a través de `ntfy` (en lugar de `ntfy serve`)

> Obsoleto 2021-12-17, el comportamiento cambió con v1.10.0

A medida que se agregan más comandos al `ntfy` Herramienta CLI, usando solo `ntfy` Ejecutar el servidor no es práctico
ya. Por favor, utilice `ntfy serve` en lugar de. Esto también se aplica a las imágenes de Docker, ya que también pueden ejecutar más de
solo el servidor.

\=== "Antes"
`     $ ntfy
    2021/12/17 08:16:01 Listening on :80/http
    `

\=== "Después"
`     $ ntfy serve
    2021/12/17 08:16:01 Listening on :80/http
    `
