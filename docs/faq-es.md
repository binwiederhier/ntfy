# Preguntas frecuentes (FAQ)

## ¿No es así como...?

Quién sabe. No investigué mucho antes de hacer esto. Fue divertido hacerlo.

## ¿Puedo usar esto en mi aplicación? ¿Se mantendrá gratis?

Sí. Mientras no abuses de él, estará disponible y será gratuito. No planeo monetizar
el servicio.

## ¿Cuáles son las garantías de tiempo de actividad?

El mejor esfuerzo.

## ¿Qué sucede si hay varios suscriptores al mismo tema?

Como de costumbre con pub-sub, todos los suscriptores reciben notificaciones si están suscritos a un tema.

## ¿Sabrás qué temas existen, puedes espiarme?

Si no confía en mí o sus mensajes son confidenciales, ejecute su propio servidor. Es de código abierto.
Dicho esto, los registros contienen nombres de temas y direcciones IP, pero no los uso para nada más que para nada más que
solución de problemas y limitación de velocidad. Los mensajes se almacenan en caché durante la duración configurada en `server.yml` (12h por defecto)
para facilitar los reinicios del servicio, el sondeo de mensajes y para superar las interrupciones de la red del cliente.

## ¿Puedo autoalojarlo?

Sí. El servidor (incluida esta interfaz de usuario web) se puede autohospedar y la aplicación Android/iOS admite la adición de temas desde
su propio servidor también. Echa un vistazo a la [instrucciones de instalación](install.md).

## ¿Por qué se utiliza Firebase?

Además de almacenar en caché los mensajes localmente y entregarlos a los suscriptores de sondeos largos, todos los mensajes también son
publicado en Firebase Cloud Messaging (FCM) (si `FirebaseKeyFile` está configurado, que está en ntfy.sh). Éste
es para facilitar las notificaciones en Android.

Si no te importa Firebase, te sugiero que instales el [Versión F-Droid](https://f-droid.org/en/packages/io.heckel.ntfy/)
de la aplicación y [autohospede su propio servidor ntfy](install.md).

## ¿Cuánta batería usa la aplicación de Android?

Si usa el servidor ntfy.sh y no usa el [entrega instantánea](subscribe/phone.md#instant-delivery) característica
la aplicación para Android/iOS no utiliza batería adicional, ya que se utiliza Firebase Cloud Messaging (FCM). Si utiliza su propio servidor,
o utiliza *entrega instantánea* (solo Android), la aplicación tiene que mantener una conexión constante con el servidor, lo que consume
aproximadamente 0-1% de la batería en 17h de uso (en mi teléfono). Ha habido un montón de pruebas y mejoras en torno a esto. Creo que es bonito
decente ahora.

## ¿Qué es la entrega instantánea?

[Entrega instantánea](subscribe/phone.md#instant-delivery) es una característica de la aplicación de Android. Si está activada, la aplicación mantiene una conexión constante con el
y escucha las notificaciones entrantes. Esto consume batería adicional (ver arriba),
pero entrega notificaciones al instante.

## ¿Dónde puedo donar?

Muchas personas han preguntado (¡gracias por eso!), pero actualmente no estoy aceptando ninguna donación. El costo es manejable
($ 25 / mes para el alojamiento y $ 99 / año para el certificado de Apple) en este momento, y no quiero tener que sentirme obligado a
cualquiera aceptando su dinero.

Sin embargo, puedo pedir donaciones en el futuro. Después de todo, $ 400 por año no es nada ...
