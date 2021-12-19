# Frequently asked questions (FAQ)

## Isn't this like ...?
Who knows. I didn't do a lot of research before making this. It was fun making it.

## Can I use this in my app? Will it stay free?
Yes. As long as you don't abuse it, it'll be available and free of charge. I do not plan on monetizing
the service.

## What are the uptime guarantees?
Best effort.

## What happens if there are multiple subscribers to the same topic?
As per usual with pub-sub, all subscribers receive notifications if they are
subscribed to a topic.

## Will you know what topics exist, can you spy on me?
If you don't trust me or your messages are sensitive, run your own server. It's <a href="https://github.com/binwiederhier/ntfy">open source</a>.
That said, the logs do not contain any topic names or other details about you.
Messages are cached for the duration configured in `server.yml` (12h by default) to facilitate service restarts, message polling and to overcome
client network disruptions.

## Can I self-host it?
Yes. The server (including this Web UI) can be self-hosted, and the Android app supports adding topics from
your own server as well. Check out the [install instructions](install.md).

## Why is Firebase used?
In addition to caching messages locally and delivering them to long-polling subscribers, all messages are also
published to Firebase Cloud Messaging (FCM) (if `FirebaseKeyFile` is set, which it is on ntfy.sh). This
is to facilitate notifications on Android. 

If you do not care for Firebase, I suggest you install the [F-Droid version](https://f-droid.org/en/packages/io.heckel.ntfy/)
of the app and [self-host your own ntfy server](install.md).

## How much battery does the Android app use?
If you use the ntfy.sh server and you don't use the [instant delivery](subscribe/phone.md#instant-delivery) feature, 
the Android app uses no additional battery, since Firebase Cloud Messaging (FCM) is used. If you use your own server, 
or you use *instant delivery*, the app has to maintain a constant connection to the server, which consumes about 4% of
battery in 17h of use (on my phone). I use it, and it makes no difference to me.

## What is instant delivery?
[Instant delivery](subscribe/phone.md#instant-delivery) is a feature in the Android app. If turned on, the app maintains a constant connection to the
server and listens for incoming notifications. This consumes <a href="#battery-usage">additional battery</a>,
but delivers notifications instantly.

## Why is there no iOS app (yet)?
I don't have an iPhone or a Mac, so I didn't make an iOS app yet. It'd be awesome if
<a href="https://github.com/binwiederhier/ntfy/issues/4">someone else could help out</a>.
