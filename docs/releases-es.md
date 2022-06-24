# Notas

Los binarios para todas las versiones se pueden encontrar en las páginas de versiones de GitHub para el [Servidor ntfy](https://github.com/binwiederhier/ntfy/releases)
y el [Aplicación ntfy para Android](https://github.com/binwiederhier/ntfy-android/releases).

<!--

## ntfy Android app v1.14.0 (UNRELEASED)

**Features:**

* Polling is now done with since=<id> API, which makes deduping easier ([#165](https://github.com/binwiederhier/ntfy/issues/165))
* Turned JSON stream deprecation banner into "Use WebSockets" banner (no ticket)

**Bugs:**

* Long-click selecting of notifications doesn't scoll to the top anymore ([#235](https://github.com/binwiederhier/ntfy/issues/235), thanks to [@wunter8](https://github.com/wunter8))
* Add attachment and click URL extras to MESSAGE_RECEIVED broadcast ([#329](https://github.com/binwiederhier/ntfy/issues/329), thanks to [@wunter8](https://github.com/wunter8))
* Accessibility: Clear/choose service URL button in base URL dropdown now has a label ([#292](https://github.com/binwiederhier/ntfy/issues/292), thanks to [@mhameed](https://github.com/mhameed) for reporting)

**Additional translations:**

* Italian (thanks to [@Genio2003](https://hosted.weblate.org/user/Genio2003/))
* Dutch (thanks to [@SchoNie](https://hosted.weblate.org/user/SchoNie/))

Thank you to [@wunter8](https://github.com/wunter8) for proactively picking up some Android tickets, and fixing them! You rock!

-->

## Servidor ntfy v1.27.2

Publicado el June 23, 2022

Esta versión trae dos nuevas opciones de CLI para esperar a que finalice un comando o a que salga un PID. También añade más detalles
para realizar un seguimiento de los resultados de depuración. Aparte de otros errores, corrige un problema de rendimiento que se produjo en grandes instalaciones cada vez
minuto más o menos, debido a la recopilación de estadísticas de la competencia (las instalaciones personales probablemente no se verán afectadas por esto).

**Funciones:**

*   Agregar `cache-startup-queries` opción para permitir la personalización [Ajuste del rendimiento de SQLite](config.md#wal-for-message-cache) (sin billete)
*   ntfy CLI ahora puede [Espere un comando o PID](subscribe/cli.md#wait-for-pidcommand) antes de publicar ([#263](https://github.com/binwiederhier/ntfy/issues/263), gracias a la [ntfy original](https://github.com/dschep/ntfy) para la idea)
*   Seguimiento: registre toda la solicitud HTTP para simplificar la depuración (sin vale)
*   Permitir la configuración de la contraseña de usuario a través de `NTFY_PASSWORD` Variable env ([#327](https://github.com/binwiederhier/ntfy/pull/327)gracias a [@Kenix3](https://github.com/Kenix3))

**Bugs:**

*   Corregir solicitudes lentas debido a un bloqueo excesivo ([#338](https://github.com/binwiederhier/ntfy/issues/338))
*   Devolver HTTP 500 para `GET /_matrix/push/v1/notify` cuando `base-url` no está configurado (sin ticket)
*   No permitir la configuración `upstream-base-url` al mismo valor que `base-url` ([#334](https://github.com/binwiederhier/ntfy/issues/334)gracias a [@oester](https://github.com/oester) para la presentación de informes)
*   Arreglar `since=<id>` implementación para múltiples temas ([#336](https://github.com/binwiederhier/ntfy/issues/336)gracias a [@karmanyaahm](https://github.com/karmanyaahm) para la presentación de informes)
*   Análisis sencillo `Actions` encabezado ahora es compatible con la configuración de Android `intent=` clave ([#341](https://github.com/binwiederhier/ntfy/pull/341)gracias a [@wunter8](https://github.com/wunter8))

**Obsolescencias:**

*   El `ntfy publish --env-topic` La opción está en desuso a partir de ahora (consulte [obsolescencias](deprecations.md) para más detalles)

## Servidor ntfy v1.26.0

Publicado el June 16, 2022

Esta versión agrega una Matrix Push Gateway directamente en ntfy, para facilitar el autoalojamiento de un servidor Matrix. Las ventanas
CLI ahora está disponible a través de Scoop, y ntfy ahora es compatible de forma nativa en Uptime Kuma.

**Funciones:**

*   ntfy ahora es un [Puerta de enlace push de matriz](https://spec.matrix.org/v1.2/push-gateway-api/) (en combinación con [UnifiedPush](https://unifiedpush.org) como el [Protocolo push del proveedor](https://unifiedpush.org/developers/gateway/), [#319](https://github.com/binwiederhier/ntfy/issues/319)/[#326](https://github.com/binwiederhier/ntfy/pull/326)gracias a [@MayeulC](https://github.com/MayeulC) para la presentación de informes)
*   La CLI de Windows ya está disponible a través de [Cuchara](https://scoop.sh) ([ScoopInstaller#3594](https://github.com/ScoopInstaller/Main/pull/3594), [#311](https://github.com/binwiederhier/ntfy/pull/311), [#269](https://github.com/binwiederhier/ntfy/issues/269)gracias a [@kzshantonu](https://github.com/kzshantonu))
*   [Tiempo de actividad Kuma](https://github.com/louislam/uptime-kuma) ahora permite publicar en ntfy ([tiempo de actividad-kuma#1674](https://github.com/louislam/uptime-kuma/pull/1674)gracias a [@philippdormann](https://github.com/philippdormann))
*   Mostrar la versión ntfy en `ntfy serve` comando ([#314](https://github.com/binwiederhier/ntfy/issues/314)gracias a [@poblabs](https://github.com/poblabs))

**Bugs:**

*   Aplicación web: Mostrar la alerta "notificaciones no compatibles" en HTTP ([#323](https://github.com/binwiederhier/ntfy/issues/323)gracias a [@milksteakjellybeans](https://github.com/milksteakjellybeans) para la presentación de informes)
*   Usar la última dirección en `X-Forwarded-For` encabezado como dirección del visitante ([#328](https://github.com/binwiederhier/ntfy/issues/328))

**Documentación**

*   Añadido [ejemplo](examples.md) para [Tiempo de actividad Kuma](https://github.com/louislam/uptime-kuma) integración ([#315](https://github.com/binwiederhier/ntfy/pull/315)gracias a [@philippdormann](https://github.com/philippdormann))
*   Corregir las instrucciones de instalación de Docker ([#320](https://github.com/binwiederhier/ntfy/issues/320)gracias a [@milksteakjellybeans](https://github.com/milksteakjellybeans) para la presentación de informes)
*   Añadir comentarios aclaratorios a base-url ([#322](https://github.com/binwiederhier/ntfy/issues/322)gracias a [@milksteakjellybeans](https://github.com/milksteakjellybeans) para la presentación de informes)
*   Actualizar preguntas frecuentes para la aplicación iOS ([#321](https://github.com/binwiederhier/ntfy/issues/321)gracias a [@milksteakjellybeans](https://github.com/milksteakjellybeans) para la presentación de informes)

## Aplicación ntfy iOS v1.2

Publicado el June 16, 2022

Esta versión agrega compatibilidad con la autenticación/autorización para servidores autohospedados. También le permite
establezca el servidor como el servidor predeterminado para los nuevos temas.

**Funciones:**

*   Compatibilidad con la autenticación y la gestión de usuarios ([#277](https://github.com/binwiederhier/ntfy/issues/277))
*   Posibilidad de agregar el servidor predeterminado ([#295](https://github.com/binwiederhier/ntfy/issues/295))

**Bugs:**

*   Agregar validación para la URL del servidor autohospedada ([#290](https://github.com/binwiederhier/ntfy/issues/290))

## Servidor ntfy v1.25.2

Publicado el 2 de junio de 2022

Esta versión agrega la capacidad de establecer un nivel de registro para facilitar la depuración de sistemas activos. También resuelve un
problema de producción con algunos usuarios excesivos que dio lugar a problemas de cuota de Firebase (solo se aplica a los usuarios excesivos).
Ahora bloqueamos a los visitantes para que no usen Firebase si activan una respuesta excedida de cuota.

Además de eso, actualizamos el SDK de Firebase y ahora estamos creando la versión en GitHub Actions. También tenemos dos
más traducciones: chino/simplificado y holandés.

**Funciones:**

*   Registro avanzado, con diferentes niveles de registro y recarga en caliente del nivel de registro ([#284](https://github.com/binwiederhier/ntfy/pull/284))

**Bugs**:

*   Respetar la respuesta de "cuota excedida" de Firebase para los temas, bloquear la publicación de Firebase para el usuario durante 10 minutos ([#289](https://github.com/binwiederhier/ntfy/issues/289))
*   Corregir el encabezado de la documentación del encabezado azul debido a la actualización del tema mkdocs-material (sin ticket)

**Mantenimiento:**

*   Actualizar Firebase Admin SDK a 4.x ([#274](https://github.com/binwiederhier/ntfy/issues/274))
*   CI: Construir a partir de canalización en lugar de localmente ([#36](https://github.com/binwiederhier/ntfy/issues/36))

**Documentación**:

*   ⚠️ [Política de privacidad](privacy.md) actualizado para reflejar la función adicional de depuración/seguimiento (sin vale)
*   [Ejemplos](examples.md) para [Asistente de inicio](https://www.home-assistant.io/) ([#282](https://github.com/binwiederhier/ntfy/pull/282)gracias a [@poblabs](https://github.com/poblabs))
*   Instrucciones de instalación para [NixOS/Nix](https://ntfy.sh/docs/install/#nixos-nix) ([#282](https://github.com/binwiederhier/ntfy/pull/282)gracias a [@arjan-s](https://github.com/arjan-s))
*   Aclarar `poll_request` redacción para [Notificaciones push de iOS](https://ntfy.sh/docs/config/#ios-instant-notifications) ([#300](https://github.com/binwiederhier/ntfy/issues/300)gracias a [@prabirshrestha](https://github.com/prabirshrestha) para la presentación de informes)
*   Ejemplo de uso de ntfy con docker-compose.yml sin privilegios de root ([#304](https://github.com/binwiederhier/ntfy/pull/304)gracias a [@ksurl](https://github.com/ksurl))

**Traducciones adicionales:**

*   Chino/Simplificado (gracias a [@yufei.im](https://hosted.weblate.org/user/yufei.im/))
*   Holandés (gracias a [@SchoNie](https://hosted.weblate.org/user/SchoNie/))

## Aplicación ntfy iOS v1.1

Lanzamiento 31 May 2022

En esta versión de la aplicación iOS, agregamos prioridades de mensajes (asignadas a niveles de interrupción de iOS), etiquetas y emojis,
botones de acción para abrir sitios web o realizar solicitudes HTTP (en la vista de notificación y detalles), un clic personalizado
acción cuando se pulsa la notificación y varias otras correcciones.

También agrega soporte para servidores autohospedados (aunque aún no admite autenticación). El servidor autohospedado debe ser
configurado para reenviar solicitudes de sondeo a ntfy.sh ascendentes para que las notificaciones push funcionen (consulte [Notificaciones push de iOS](https://ntfy.sh/docs/config/#ios-instant-notifications)
para más detalles).

**Funciones:**

*   [Prioridad del mensaje](https://ntfy.sh/docs/publish/#message-priority) soporte (sin ticket)
*   [Etiquetas/emojis](https://ntfy.sh/docs/publish/#tags-emojis) soporte (sin ticket)
*   [Botones de acción](https://ntfy.sh/docs/publish/#action-buttons) soporte (sin ticket)
*   [Haga clic en la acción](https://ntfy.sh/docs/publish/#click-action) soporte (sin ticket)
*   Abrir tema cuando se hace clic en la notificación (sin ticket)
*   La notificación ahora hace un sonido y vibra (sin ticket)
*   Cancelar notificaciones al navegar al tema (sin ticket)
*   Compatibilidad con iOS 14.0 (sin ticket, [PR#1](https://github.com/binwiederhier/ntfy-ios/pull/1)gracias a [@callum-99](https://github.com/callum-99))

**Bugs:**

*   La interfaz de usuario de iOS no siempre se actualiza correctamente ([#267](https://github.com/binwiederhier/ntfy/issues/267))

## Servidor ntfy v1.24.0

Lanzamiento 28 May 2022

Esta versión del servidor ntfy trae características compatibles para la aplicación ntfy iOS. Lo más importante es que
permite la compatibilidad con servidores autohospedados en combinación con la aplicación iOS. Esto es para superar lo restrictivo
Entorno de desarrollo de Apple.

**Funciones:**

*   Envía regularmente mensajes keepalive de Firebase a ~poll topic para admitir servidores autohospedados (sin ticket)
*   Agregar filtro de suscripción para consultar mensajes exactos por ID (sin ticket)
*   Soporte para `poll_request` mensajes para apoyar [Notificaciones push de iOS](https://ntfy.sh/docs/config/#ios-instant-notifications) para servidores autohospedados (sin vale)

**Bugs:**

*   Correos electrónicos de soporte sin `Content-Type` ([#265](https://github.com/binwiederhier/ntfy/issues/265)gracias a [@dmbonsall](https://github.com/dmbonsall))

**Traducciones adicionales:**

*   Italiano (gracias a [@Genio2003](https://hosted.weblate.org/user/Genio2003/))

## ntfy aplicación iOS v1.0

Lanzamiento 25 May 2022

Esta es la primera versión de la aplicación ntfy para iOS. Solo admite ntfy.sh (sin servidores autohospedados) y solo mensajes + título
(sin prioridad, etiquetas, archivos adjuntos, ...). Agregaré rápidamente (con suerte) la mayoría de las otras características de ntfy, y luego me enfocaré
en servidores autohospedados.

La aplicación ya está disponible en el [Tienda de aplicaciones](https://apps.apple.com/us/app/ntfy/id1625396347).

**Entradas:**

*   Aplicación iOS ([#4](https://github.com/binwiederhier/ntfy/issues/4), véase también: [Resumen de TestFlight](https://github.com/binwiederhier/ntfy/issues/4#issuecomment-1133767150))

**Gracias:**

*   Gracias a todos los probadores que probaron la aplicación. Ustedes me dieron la confianza de que está listo para lanzar (aunque con
    algunos problemas conocidos que se abordarán en las versiones de seguimiento).

## Servidor ntfy v1.23.0

Lanzamiento 21 May 2022

Esta versión incluye una CLI para Windows y macOS, así como la capacidad de deshabilitar la aplicación web por completo. Además de eso,
agrega soporte para APNs, el servicio de mensajería de iOS. Esto es necesario para la aplicación iOS (que pronto se lanzará).

**Funciones:**

*   [Windows](https://ntfy.sh/docs/install/#windows) y [macOS](https://ntfy.sh/docs/install/#macos) compilaciones para el [CLI ntfy](https://ntfy.sh/docs/subscribe/cli/) ([#112](https://github.com/binwiederhier/ntfy/issues/112))
*   Posibilidad de deshabilitar la aplicación web por completo ([#238](https://github.com/binwiederhier/ntfy/issues/238)/[#249](https://github.com/binwiederhier/ntfy/pull/249)gracias a [@Curid](https://github.com/Curid))
*   Agregar configuración de APNs a los mensajes de Firebase para admitir [Aplicación iOS](https://github.com/binwiederhier/ntfy/issues/4) ([#247](https://github.com/binwiederhier/ntfy/pull/247)gracias a [@Copephobia](https://github.com/Copephobia))

**Bugs:**

*   Admite guiones bajos en las opciones de configuración de server.yml ([#255](https://github.com/binwiederhier/ntfy/issues/255)gracias a [@ajdelgado](https://github.com/ajdelgado))
*   Forzar MAKEFLAGS a --jobs=1 en `Makefile` ([#257](https://github.com/binwiederhier/ntfy/pull/257)gracias a [@oddlama](https://github.com/oddlama))

**Documentación:**

*   Error tipográfico en las instrucciones de instalación ([#252](https://github.com/binwiederhier/ntfy/pull/252)/[#251](https://github.com/binwiederhier/ntfy/issues/251)gracias a [@oddlama](https://github.com/oddlama))
*   Corregir error tipográfico en el ejemplo de servidor privado ([#262](https://github.com/binwiederhier/ntfy/pull/262)gracias a [@MayeulC](https://github.com/MayeulC))
*   [Ejemplos](examples.md) para [medusa](https://github.com/Fallenbagel/jellyseerr)/[supervisor](https://overseerr.dev/) ([#264](https://github.com/binwiederhier/ntfy/pull/264)gracias a [@Fallenbagel](https://github.com/Fallenbagel))

**Traducciones adicionales:**

*   Portugués/Brasil (gracias a [@tiagotriques](https://hosted.weblate.org/user/tiagotriques/) y [@pireshenrique22](https://hosted.weblate.org/user/pireshenrique22/))

Gracias a los muchos traductores, que ayudaron a traducir las nuevas cadenas tan rápidamente. Me siento honrado y sorprendido por su ayuda.

## ntfy Aplicación android v1.13.0

Lanzamiento 11 May 2022

Esta versión trae un diseño ligeramente alterado para la vista de detalles, con un diseño de tarjeta para hacer notificaciones más fácilmente.
distinguibles entre sí. También envía configuraciones por tema que permiten anular la prioridad mínima, el umbral de eliminación automática
e iconos personalizados. Aparte de eso, tenemos toneladas de correcciones de errores como de costumbre.

**Funciones:**

*   Configuración por suscripción, iconos de suscripción personalizados ([#155](https://github.com/binwiederhier/ntfy/issues/155)gracias a [@mztiq](https://github.com/mztiq) para la presentación de informes)
*   Tarjetas en vista de detalle de notificación ([#175](https://github.com/binwiederhier/ntfy/issues/175)gracias a [@cmeis](https://github.com/cmeis) para la presentación de informes)

**Bugs:**

*   Denominación precisa de "notificaciones de silencio" de "notificaciones de pausa" ([#224](https://github.com/binwiederhier/ntfy/issues/224)gracias a [@shadow00](https://github.com/shadow00) para la presentación de informes)
*   Hacer que los mensajes con enlaces sean seleccionables ([#226](https://github.com/binwiederhier/ntfy/issues/226)gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov) para la presentación de informes)
*   Restaurar temas o configuraciones desde la copia de seguridad no funciona ([#223](https://github.com/binwiederhier/ntfy/issues/223)gracias a [@shadow00](https://github.com/shadow00) para la presentación de informes)
*   Corregir el icono de la aplicación en versiones antiguas de Android ([#128](https://github.com/binwiederhier/ntfy/issues/128)gracias a [@shadow00](https://github.com/shadow00) para la presentación de informes)
*   Arreglar carreras en el registro de UnifiedPush ([#230](https://github.com/binwiederhier/ntfy/issues/230), gracias a @Jakob por informar)
*   Evitar que la acción de vista bloquee la aplicación ([#233](https://github.com/binwiederhier/ntfy/issues/233))
*   Evitar que los nombres e iconos de temas largos se superpongan ([#240](https://github.com/binwiederhier/ntfy/issues/240)gracias a [@cmeis](https://github.com/cmeis) para la presentación de informes)

**Traducciones adicionales:**

*   Holandés (*incompleto*gracias a [@diony](https://hosted.weblate.org/user/diony/))

**Gracias:**

Gracias a [@cmeis](https://github.com/cmeis), [@StoyanDimitrov](https://github.com/StoyanDimitrov), [@Fallenbagel](https://github.com/Fallenbagel) para las pruebas, y
Para [@Joeharrison94](https://github.com/Joeharrison94) para la entrada. Y muchas gracias a todos los traductores por ponerse al día tan rápido.

## Servidor ntfy v1.22.0

Lanzamiento 7 May 2022

Esta versión hace que la aplicación web sea más accesible para las personas con discapacidades e introduce un icono de "marcar como leída" en la aplicación web.
También corrige un curioso error con WebSockets y Apache y hace que los sonidos de notificación en la aplicación web sean un poco más silenciosos.

También hemos mejorado un poco la documentación y hemos añadido traducciones para tres idiomas más.

**Funciones:**

*   Hacer que la aplicación web sea más accesible ([#217](https://github.com/binwiederhier/ntfy/issues/217))
*   Mejor análisis de las acciones del usuario, permitiendo cotizaciones (sin ticket)
*   Agregue el botón de icono "marcar como leído" a la notificación ([#243](https://github.com/binwiederhier/ntfy/pull/243)gracias a [@wunter8](https://github.com/wunter8))

**Bugs:**

*   `Upgrade` La comprobación del encabezado ahora distingue entre mayúsculas y minúsculas ([#228](https://github.com/binwiederhier/ntfy/issues/228)gracias a [@wunter8](https://github.com/wunter8) para encontrarlo)
*   Hecho que la aplicación web suene más silenciosa ([#222](https://github.com/binwiederhier/ntfy/issues/222))
*   Agregar un mensaje de error específico de "navegación privada" para Firefox/Safari ([#208](https://github.com/binwiederhier/ntfy/issues/208)gracias a [@julianfoad](https://github.com/julianfoad) para la presentación de informes)

**Documentación:**

*   Configuración de caddy mejorada (sin ticket, gracias a @Stnby)
*   Ejemplos adicionales de varias líneas en el [publicar página](https://ntfy.sh/docs/publish/) ([#234](https://github.com/binwiederhier/ntfy/pull/234)gracias a [@aTable](https://github.com/aTable))
*   Se ha corregido el ejemplo de autenticación de PowerShell para usar UTF-8 ([#242](https://github.com/binwiederhier/ntfy/pull/242)gracias a [@SMAW](https://github.com/SMAW))

**Traducciones adicionales:**

*   Checo (gracias a [@waclaw66](https://hosted.weblate.org/user/waclaw66/))
*   Francés (gracias a [@nathanaelhoun](https://hosted.weblate.org/user/nathanaelhoun/))
*   Húngaro (gracias a [@agocsdaniel](https://hosted.weblate.org/user/agocsdaniel/))

**Gracias por probar:**

Gracias a [@wunter8](https://github.com/wunter8) para pruebas.

## ntfy Aplicación android v1.12.0

Lanzamiento 25 Abr 2022

La característica principal de esta versión de Android es [Botones de acción](https://ntfy.sh/docs/publish/#action-buttons), una característica
que permite a los usuarios añadir acciones a las notificaciones. Las acciones pueden ser ver un sitio web o una aplicación, enviar una transmisión o
enviar una solicitud HTTP.

También agregamos soporte para [ntfy:// enlaces profundos](https://ntfy.sh/docs/subscribe/phone/#ntfy-links), añadidas tres más
y se han corregido un montón de errores.

**Funciones:**

*   Notificación personalizada [botones de acción](https://ntfy.sh/docs/publish/#action-buttons) ([#134](https://github.com/binwiederhier/ntfy/issues/134),
    gracias a [@mrherman](https://github.com/mrherman) para la presentación de informes)
*   Soporte para [ntfy:// enlaces profundos](https://ntfy.sh/docs/subscribe/phone/#ntfy-links) ([#20](https://github.com/binwiederhier/ntfy/issues/20)gracias
    Para [@Copephobia](https://github.com/Copephobia) para la presentación de informes)
*   [Metadatos de Fastlane](https://hosted.weblate.org/projects/ntfy/android-fastlane/) ahora también se puede traducir ([#198](https://github.com/binwiederhier/ntfy/issues/198),
    gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov) para la presentación de informes)
*   Opción de configuración de canal para configurar la anulación de DND, sonidos, etc. ([#91](https://github.com/binwiederhier/ntfy/issues/91))

**Bugs:**

*   Validar direcciones URL al cambiar el servidor y el servidor predeterminados en la administración de usuarios ([#193](https://github.com/binwiederhier/ntfy/issues/193),
    gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov) para la presentación de informes)
*   Error al enviar la notificación de prueba en diferentes idiomas ([#209](https://github.com/binwiederhier/ntfy/issues/209),
    gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov) para la presentación de informes)
*   La casilla de verificación "\[x] Entrega instantánea en modo doze" no funciona correctamente ([#211](https://github.com/binwiederhier/ntfy/issues/211))
*   No permitir acciones GET/HEAD "http" con el cuerpo ([#221](https://github.com/binwiederhier/ntfy/issues/221)gracias a
    [@cmeis](https://github.com/cmeis) para la presentación de informes)
*   La acción "view" con "clear=true" no funciona en algunos teléfonos ([#220](https://github.com/binwiederhier/ntfy/issues/220)gracias a
    [@cmeis](https://github.com/cmeis) para la presentación de informes)
*   No agrupe la notificación de servicio en primer plano con otros ([#219](https://github.com/binwiederhier/ntfy/issues/219)gracias a
    [@s-h-a-r-d](https://github.com/s-h-a-r-d) para la presentación de informes)

**Traducciones adicionales:**

*   Checo (gracias a [@waclaw66](https://hosted.weblate.org/user/waclaw66/))
*   Francés (gracias a [@nathanaelhoun](https://hosted.weblate.org/user/nathanaelhoun/))
*   Japonés (gracias a [@shak](https://hosted.weblate.org/user/shak/))
*   Ruso (gracias a [@flamey](https://hosted.weblate.org/user/flamey/) y [@ilya.mikheev.coder](https://hosted.weblate.org/user/ilya.mikheev.coder/))

**Gracias por probar:**

Gracias a [@s-h-a-r-d](https://github.com/s-h-a-r-d) (también conocido como @Shard), [@cmeis](https://github.com/cmeis),
@poblabs, y todos los que olvidé para probar.

## Servidor ntfy v1.21.2

Lanzamiento 24 Abr 2022

En esta versión, la aplicación web obtuvo soporte de traducción y ya 🇧🇬 🇩🇪 🇺🇸 🌎 se tradujo a 9 idiomas.
También vuelve a agregar soporte para ARMv6 y agrega soporte del lado del servidor para botones de acción. [Botones de acción](https://ntfy.sh/docs/publish/#action-buttons)
es una característica que se lanzará en la aplicación de Android pronto. Permite a los usuarios agregar acciones a las notificaciones.
El soporte técnico limitado está disponible en la aplicación web.

**Funciones:**

*   Notificación personalizada [botones de acción](https://ntfy.sh/docs/publish/#action-buttons) ([#134](https://github.com/binwiederhier/ntfy/issues/134),
    gracias a [@mrherman](https://github.com/mrherman) para la presentación de informes)
*   Se ha añadido la compilación ARMv6 ([#200](https://github.com/binwiederhier/ntfy/issues/200)gracias a [@jcrubioa](https://github.com/jcrubioa) para la presentación de informes)
*   Compatibilidad con la internacionalización de aplicaciones 🇧🇬 🇩🇪 🇺🇸 🌎 web ([#189](https://github.com/binwiederhier/ntfy/issues/189))

**Bugs:**

*   Aplicación web: correcciones de cadenas de idioma inglés, descripciones adicionales para la configuración ([#203](https://github.com/binwiederhier/ntfy/issues/203)gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov))
*   Aplicación web: Mostrar mensaje de error snackbar cuando se produce un error al enviar una notificación de prueba ([#205](https://github.com/binwiederhier/ntfy/issues/205)gracias a [@cmeis](https://github.com/cmeis))
*   Aplicación web: validación básica de URL en la administración de usuarios ([#204](https://github.com/binwiederhier/ntfy/issues/204)gracias a [@cmeis](https://github.com/cmeis))
*   No permitir acciones GET/HEAD "http" con el cuerpo ([#221](https://github.com/binwiederhier/ntfy/issues/221)gracias a
    [@cmeis](https://github.com/cmeis) para la presentación de informes)

**Traducciones (aplicación web):**

*   Búlgaro (gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov))
*   Alemán (gracias a [@cmeis](https://github.com/cmeis))
*   Indonesio (gracias a [@linerly](https://hosted.weblate.org/user/linerly/))
*   Japonés (gracias a [@shak](https://hosted.weblate.org/user/shak/))
*   Bokmål noruego (gracias a [@comradekingu](https://github.com/comradekingu))
*   Ruso (gracias a [@flamey](https://hosted.weblate.org/user/flamey/) y [@ilya.mikheev.coder](https://hosted.weblate.org/user/ilya.mikheev.coder/))
*   Español (gracias a [@rogeliodh](https://github.com/rogeliodh))
*   Turco (gracias a [@ersen](https://ersen.moe/))

**Integraciones:**

[Informar](https://github.com/caronc/apprise) el soporte se lanzó completamente en [v0.9.8.2](https://github.com/caronc/apprise/releases/tag/v0.9.8.2)
de Apprise. Gracias a [@particledecay](https://github.com/particledecay) y [@caronc](https://github.com/caronc) por su fantástico trabajo.
Puede probarlo usted mismo de esta manera (uso detallado en el [Wiki de Apprise](https://github.com/caronc/apprise/wiki/Notify_ntfy)):

    pip3 install apprise
    apprise -b "Hi there" ntfys://mytopic

## ntfy Aplicación android v1.11.0

Lanzamiento 7 Abr 2022

**Funciones:**

*   Descargar archivos adjuntos a la carpeta de caché ([#181](https://github.com/binwiederhier/ntfy/issues/181))
*   Elimine regularmente los archivos adjuntos de las notificaciones eliminadas ([#142](https://github.com/binwiederhier/ntfy/issues/142))
*   Traducciones a diferentes idiomas ([#188](https://github.com/binwiederhier/ntfy/issues/188)gracias a
    [@StoyanDimitrov](https://github.com/StoyanDimitrov) para iniciar cosas)

**Bugs:**

*   IllegalStateException: Error al generar un archivo único ([#177](https://github.com/binwiederhier/ntfy/issues/177)gracias a [@Fallenbagel](https://github.com/Fallenbagel) para la presentación de informes)
*   SQLiteConstraintException: Bloqueo durante el registro UP ([#185](https://github.com/binwiederhier/ntfy/issues/185))
*   Actualizar la pantalla de preferencias después de la importación de la configuración (# 183, gracias a [@cmeis](https://github.com/cmeis) para la presentación de informes)
*   Agregue cadenas de prioridad a las cadenas.xml para que sea traducible (# 192, gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov))

**Traducciones:**

*   Mejoras en el idioma inglés (gracias a [@comradekingu](https://github.com/comradekingu))
*   Búlgaro (gracias a [@StoyanDimitrov](https://github.com/StoyanDimitrov))
*   Chino/Simplificado (gracias a [@poi](https://hosted.weblate.org/user/poi) y [@PeterCxy](https://hosted.weblate.org/user/PeterCxy))
*   Holandés (*incompleto*gracias a [@diony](https://hosted.weblate.org/user/diony))
*   Francés (gracias a [@Kusoneko](https://kusoneko.moe/) y [@mlcsthor](https://hosted.weblate.org/user/mlcsthor/))
*   Alemán (gracias a [@cmeis](https://github.com/cmeis))
*   Italiano (gracias a [@theTranslator](https://hosted.weblate.org/user/theTranslator/))
*   Indonesio (gracias a [@linerly](https://hosted.weblate.org/user/linerly/))
*   Bokmål noruego (*incompleto*gracias a [@comradekingu](https://github.com/comradekingu))
*   Portugués/Brasil (gracias a [ML:](https://hosted.weblate.org/user/LW/))
*   Español (gracias a [@rogeliodh](https://github.com/rogeliodh))
*   Turco (gracias a [@ersen](https://ersen.moe/))

**Gracias:**

*   Muchas gracias a [@cmeis](https://github.com/cmeis), [@Fallenbagel](https://github.com/Fallenbagel), [@Joeharrison94](https://github.com/Joeharrison94),
    y [@rogeliodh](https://github.com/rogeliodh) para obtener información sobre la nueva lógica de datos adjuntos y para probar la versión

## Servidor ntfy v1.20.0

Lanzamiento 6 Abr 2022

**Funciones:**:

*   Se ha añadido la barra de mensajes y el cuadro de diálogo de publicación ([#196](https://github.com/binwiederhier/ntfy/issues/196))

**Bugs:**

*   Añadido `EXPOSE 80/tcp` a Dockerfile para admitir la detección automática en [Traefik](https://traefik.io/) ([#195](https://github.com/binwiederhier/ntfy/issues/195)gracias a [@s-h-a-r-d](https://github.com/s-h-a-r-d))

**Documentación:**

*   Se ha agregado un ejemplo de docker-compose a [instrucciones de instalación](install.md#docker) ([#194](https://github.com/binwiederhier/ntfy/pull/194)gracias a [@s-h-a-r-d](https://github.com/s-h-a-r-d))

**Integraciones:**

*   [Informar](https://github.com/caronc/apprise) ha añadido la integración en ntfy ([#99](https://github.com/binwiederhier/ntfy/issues/99), [apprise#524](https://github.com/caronc/apprise/pull/524),
    gracias a [@particledecay](https://github.com/particledecay) y [@caronc](https://github.com/caronc) por su fantástico trabajo)

## Servidor ntfy v1.19.0

Lanzamiento 30 Mar 2022

**Bugs:**

*   No empaquetar binario con `upx` para armv7/arm64 debido a `illegal instruction` errores ([#191](https://github.com/binwiederhier/ntfy/issues/191)gracias a [@iexos](https://github.com/iexos))
*   No permitir comas en el nombre del tema en la publicación a través del punto de conexión GET (sin ticket)
*   Agregue "Access-Control-Allow-Origin: \*" para los archivos adjuntos (sin ticket, gracias a @FrameXX)
*   Hacer que la poda se ejecute de nuevo en la aplicación web ([#186](https://github.com/binwiederhier/ntfy/issues/186))
*   Se han añadido parámetros que faltan `delay` y `email` Para publicar como cuerpo JSON (sin vale)

**Documentación:**

*   Mejorado [publicación de correo electrónico](config.md#e-mail-publishing) documentación

## Servidor ntfy v1.18.1

Lanzamiento 21 Mar 2022\
*Esta versión no incluye características ni correcciones de errores. Es simplemente una actualización de la documentación.*

**Documentación:**

*   Revisión de [documentación para desarrolladores](https://ntfy.sh/docs/develop/)
*   Ejemplos de PowerShell para [publicar documentación](https://ntfy.sh/docs/publish/) ([#138](https://github.com/binwiederhier/ntfy/issues/138)gracias a [@Joeharrison94](https://github.com/Joeharrison94))
*   Ejemplos adicionales para [NodeRED, Gatus, Sonarr, Radarr, ...](https://ntfy.sh/docs/examples/) (gracias a [@nickexyz](https://github.com/nickexyz))
*   Correcciones en las instrucciones del desarrollador (gracias a [@Fallenbagel](https://github.com/Fallenbagel) para la presentación de informes)

## ntfy Aplicación android v1.10.0

Lanzamiento 21 Mar 2022

**Funciones:**

*   Compatibilidad con la especificación UnifiedPush 2.0 (mensajes bytes, [#130](https://github.com/binwiederhier/ntfy/issues/130))
*   Configuración de exportación/importación y suscripciones ([#115](https://github.com/binwiederhier/ntfy/issues/115)gracias [@cmeis](https://github.com/cmeis) para la presentación de informes)
*   Abra el enlace "Click" al tocar la notificación ([#110](https://github.com/binwiederhier/ntfy/issues/110)gracias [@cmeis](https://github.com/cmeis) para la presentación de informes)
*   Banner de obsolescencia de flujo JSON ([#164](https://github.com/binwiederhier/ntfy/issues/164))

**Correcciones:**

*   Mostrar horas específicas de la configuración regional, con formato AM/PM o 24h ([#140](https://github.com/binwiederhier/ntfy/issues/140)gracias [@hl2guide](https://github.com/hl2guide) para la presentación de informes)

## Servidor ntfy v1.18.0

Lanzamiento 16 Mar 2022

**Funciones:**

*   [Publicar mensajes como JSON](https://ntfy.sh/docs/publish/#publish-as-json) ([#133](https://github.com/binwiederhier/ntfy/issues/133),
    gracias [@cmeis](https://github.com/cmeis) por informar, gracias a [@Joeharrison94](https://github.com/Joeharrison94) y
    [@Fallenbagel](https://github.com/Fallenbagel) para pruebas)

**Correcciones:**

*   rpm: no sobrescriba Server.yaml en la actualización del paquete ([#166](https://github.com/binwiederhier/ntfy/issues/166)gracias [@waclaw66](https://github.com/waclaw66) para la presentación de informes)
*   Error tipográfico en [ntfy.sh/announcements](https://ntfy.sh/announcements) tema ([#170](https://github.com/binwiederhier/ntfy/pull/170)gracias a [@sandebert](https://github.com/sandebert))
*   Correcciones de URL de imagen Léame ([#156](https://github.com/binwiederhier/ntfy/pull/156)gracias a [@ChaseCares](https://github.com/ChaseCares))

**Obsolescencias:**

*   Se ha eliminado la capacidad de ejecutar el servidor como `ntfy` (a diferencia de `ntfy serve`) según [Desaprobación](deprecations.md)

## Servidor ntfy v1.17.1

Lanzamiento 12 Mar 2022

**Correcciones:**

*   Reemplazar `crypto.subtle` con `hashCode` a errores con Brave/FF-Windows (#157, gracias por informar @arminus)

## Servidor ntfy v1.17.0

Lanzamiento 11 Mar 2022

**Características y correcciones de errores:**

*   Reemplazar [aplicación web](https://ntfy.sh/app) con una aplicación web basada en React/MUI del siglo XXI (#111)
*   Interfaz de usuario web rota con autenticación (# 132, gracias por informar @arminus)
*   Enviar recursos web estáticos como `Content-Encoding: gzip`, es decir, documentos y aplicación web (sin ticket)
*   Agregar soporte para autenticación a través de `?auth=...` parámetro de consulta, utilizado por WebSocket en la aplicación web (sin vale)

## Servidor ntfy v1.16.0

Lanzamiento 27 Feb 2022

**Características y correcciones de errores:**

*   Agregar [Compatibilidad con autenticación](https://ntfy.sh/docs/subscribe/cli/#authentication) por suscribirse a CLI (#147/#148, gracias @lrabane)
*   Agregar soporte para [?desde=<id>](https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages) (#151, gracias por informar @nachotp)

**Documentación:**

*   Agregar [ejemplos de watchtower/shoutrr](https://ntfy.sh/docs/examples/#watchtower-notifications-shoutrrr) (#150, gracias @rogeliodh)
*   Agregar [Notas](https://ntfy.sh/docs/releases/)

**Notas técnicas:**

*   A partir de esta versión, los IDENTIFICADORES de mensajes tendrán una longitud de 12 caracteres (en lugar de 10 caracteres). Esto es para poder
    distinguirlos de las marcas de tiempo de Unix para #151.

## ntfy Aplicación para Android v1.9.1

Lanzamiento 16 Feb 2022

**Funciones:**

*   Función Compartir en el tema (#131, gracias u/emptymatrix por informar)
*   Capacidad para elegir un servidor predeterminado (# 127, gracias a @poblabs para informes y pruebas)
*   Eliminar automáticamente las notificaciones (#71, gracias @arjan-s por los informes)
*   Tema oscuro: Mejoras en el estilo y el contraste (#119, gracias @kzshantonu por informar)

**Correcciones:**

*   No intente descargar archivos adjuntos si ya han caducado (#135)
*   Se ha corregido el bloqueo en AddFragment como se ve por seguimiento de pila en Play Console (sin ticket)

**Otras gracias:**

*   Gracias a @rogeliodh, @cmeis y @poblabs por las pruebas

## Servidor ntfy v1.15.0

Lanzamiento 14 Feb 2022

**Características y correcciones de errores:**

*   Comprimir binarios con `upx` (#137)
*   Agregar `visitor-request-limit-exempt-hosts` para eximir a los anfitriones amigables de los límites de tarifas (#144)
*   Límite de solicitudes predeterminadas dobles por segundo de 1 por 10s a 1 por 5s (sin ticket)
*   Convertir `\n` a nueva línea para `X-Message` encabezado como función de preparación para compartir (consulte #136)
*   Reduzca el costo de bcrypt a 10 para que el tiempo de autenticación sea más razonable en servidores lentos (sin ticket)
*   Actualización de documentos para incluir [temas de prueba pública](https://ntfy.sh/docs/publish/#public-topics) (sin billete)

## Servidor ntfy v1.14.1

Lanzamiento 9 Feb 2022

**Correcciones:**

*   Arreglar la compilación de ARMv8 Docker (# 113, gracias a @djmaze)
*   No hay otros cambios significativos

## ntfy Aplicación android v1.8.1

Lanzamiento 6 Feb 2022

**Funciones:**

*   Apoyo [autenticación / control de acceso](https://ntfy.sh/docs/config/#access-control) (#19, gracias a @cmeis, @drsprite/@poblabs,
    @gedw99, @karmanyaahm, @Mek101, @gc-ss, @julianfoad, @nmoseman, Jakob, PeterCxy, Techlosopher)
*   Exportar/cargar registro ahora permite registros censurados/sin censura (sin ticket)
*   Se ha eliminado el bloqueo de activación (excepto para el envío de notificaciones, sin ticket)
*   Desliza el dedo para eliminar notificaciones (#117)

**Correcciones:**

*   Solucionar problemas de descarga en SDK 29 "Movimiento no permitido" (#116, gracias Jakob)
*   Solución para bloqueos de Android 12 (# 124, gracias @eskilop)
*   Corregir el error de lógica de reintento de WebSocket con varios servidores (sin ticket)
*   Corregir la carrera en la lógica de actualización que conduce a conexiones duplicadas (sin ticket)
*   Solucione el problema de desplazamiento en el cuadro de diálogo suscribirse al tema (# 131, gracias @arminus)
*   Corregir el color del campo de texto de la URL base en modo oscuro y el tamaño con fuentes grandes (sin ticket)
*   Corregir el color de la barra de acción en modo oscuro (hacer negro, sin ticket)

**Notas:**

*   Trabajo fundamental para la configuración por suscripción

## Servidor ntfy v1.14.0

Lanzamiento 3 Feb 2022

**Funciones**:

*   Lado del servidor para [autenticación y autorización](https://ntfy.sh/docs/config/#access-control) (#19, gracias por probar @cmeis, y por los aportes de @gedw99, @karmanyaahm, @Mek101, @gc-ss, @julianfoad, @nmoseman, Jakob, PeterCxy, Techlosopher)
*   Apoyo `NTFY_TOPIC` variable env en `ntfy publish` (#103)

**Correcciones**:

*   Los mensajes binarios unifiedPush no deben convertirse en archivos adjuntos (parte 1, #101)

**Docs**:

*   Aclaración sobre los archivos adjuntos (#118, gracias @xnumad)

## ntfy Aplicación para Android v1.7.1

Lanzamiento 21 Ene 2022

**Nuevas características:**

*   Mejoras en la batería: wakelock deshabilitado de forma predeterminada (#76)
*   Modo oscuro: permite cambiar la apariencia de la aplicación (#102)
*   Registros de informes: Copiar/exportar registros para ayudar a solucionar problemas (#94)
*   WebSockets (experimental): Use WebSockets para suscribirse a temas (#96, #100, #97)
*   Mostrar banner de optimización de la batería (#105)

**Correcciones:**

*   Soporte (parcial) para mensajes binarios de UnifiedPush (#101)

**Notas:**

*   El wakelock en primer plano ahora está deshabilitado de forma predeterminada
*   El reiniciador de servicio ahora está programado cada 3h en lugar de cada 6h

## Servidor ntfy v1.13.0

Lanzamiento 16 Ene 2022

**Funciones:**

*   [Websockets](https://ntfy.sh/docs/subscribe/api/#websockets) Extremo
*   Escuchar en el socket Unix, consulte [opción de configuración](https://ntfy.sh/docs/config/#config-options) `listen-unix`

## ntfy Aplicación android v1.6.0

Lanzamiento 14 Ene 2022

**Nuevas características:**

*   Archivos adjuntos: Enviar archivos al teléfono (#25, #15)
*   Acción de clic: Agregar una URL de acción de clic a las notificaciones (#85)
*   Optimización de la batería: permite deshabilitar el bloqueo de activación persistente (# 76, gracias @MatMaul)
*   Reconocer el certificado de CA de usuario importado para servidores autohospedados (#87, gracias @keith24)
*   Elimine las menciones de "entrega instantánea" de F-Droid para que sea menos confuso (sin boleto)

**Correcciones:**

*   La suscripción "silenciada hasta" no siempre fue respetada (#90)
*   Corregir dos rastros de pila reportados por los signos vitales de Play Console (sin ticket)
*   Truncar mensajes FCM >4.000 bytes, prefiere mensajes instantáneos (#84)

## Servidor ntfy v1.12.1

Lanzamiento 14 Ene 2022

**Correcciones:**

*   Solucionar el problema de seguridad con el pico de datos adjuntos (#93)

## Servidor ntfy v1.12.0

Lanzamiento 13 Ene 2022

**Funciones:**

*   [Accesorios](https://ntfy.sh/docs/publish/#attachments) (#25, #15)
*   [Haga clic en la acción](https://ntfy.sh/docs/publish/#click-action) (#85)
*   Aumentar la prioridad de FCM para mensajes de prioridad alta/máxima (#70)

**Correcciones:**

*   Haga que el script postinst funcione correctamente para sistemas basados en rpm (# 83, gracias @cmeis)
*   Truncar mensajes FCM de más de 4000 bytes (#84)
*   Arreglar `listen-https` puerto (sin billete)

## ntfy Aplicación android v1.5.2

Lanzamiento 3 Ene 2022

**Nuevas características:**

*   Permitir el uso de ntfy como distribuidor de UnifiedPush (#9)
*   Soporte para mensajes más largos de hasta 4096 bytes (#77)
*   Prioridad mínima: mostrar notificaciones solo si la prioridad X o superior (#79)
*   Permitir la desactivación de difusiones en la configuración global (#80)

**Correcciones:**

*   Permitir extras int/long para SEND_MESSAGE intención (#57)
*   Varias correcciones de mejora de la batería (# 76)

## Servidor ntfy v1.11.2

Lanzamiento 1 Ene 2022

**Características y correcciones de errores:**

*   Aumentar el límite de mensajes a 4096 bytes (4k) #77
*   Documentos para [UnifiedPush](https://unifiedpush.org) #9
*   Aumentar el intervalo keepalive a 55s #76
*   Aumenta la vida útil de Firebase a 3 horas #76

## Servidor ntfy v1.10.0

Lanzamiento 28 Dic 2021

**Características y correcciones de errores:**

*   [Publicar mensajes por correo electrónico](ntfy.sh/docs/publish/#e-mail-publishing) #66
*   Trabajo del lado del servidor para admitir [unifiedpush.org](https://unifiedpush.org) #64
*   Arreglando el error de Santa #65

## Versiones anteriores

Para versiones anteriores, echa un vistazo a las páginas de versiones de GitHub para el [Servidor ntfy](https://github.com/binwiederhier/ntfy/releases)
y el [Aplicación ntfy para Android](https://github.com/binwiederhier/ntfy-android/releases).
