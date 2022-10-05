# Frequently asked questions (FAQ)

## Isn't this like ...?
Who knows. I didn't do a lot of research before making this. It was fun making it.

## Can I use this in my app? Will it stay free?
Yes. As long as you don't abuse it, it'll be available and free of charge. I do not plan on monetizing
the service.

## What are the uptime guarantees?
Best effort.

## What happens if there are multiple subscribers to the same topic?
As per usual with pub-sub, all subscribers receive notifications if they are subscribed to a topic.

## Will you know what topics exist, can you spy on me?
If you don't trust me or your messages are sensitive, run your own server. It's open source.
That said, the logs do contain topic names and IP addresses, but I don't use them for anything other than
troubleshooting and rate limiting. Messages are cached for the duration configured in `server.yml` (12h by default) 
to facilitate service restarts, message polling and to overcome client network disruptions.

## Can I self-host it?
Yes. The server (including this Web UI) can be self-hosted, and the Android/iOS app supports adding topics from
your own server as well. Check out the [install instructions](install.md).

## Is Firebase used?
In addition to caching messages locally and delivering them to long-polling subscribers, all messages are also
published to Firebase Cloud Messaging (FCM) (if `FirebaseKeyFile` is set, which it is on ntfy.sh). This
is to facilitate notifications on Android. 

If you do not care for Firebase, I suggest you install the [F-Droid version](https://f-droid.org/en/packages/io.heckel.ntfy/)
of the app and [self-host your own ntfy server](install.md).

## How much battery does the Android app use?
If you use the ntfy.sh server, and you don't use the [instant delivery](subscribe/phone.md#instant-delivery) feature, 
the Android/iOS app uses no additional battery, since Firebase Cloud Messaging (FCM) is used. If you use your own server, 
or you use *instant delivery* (Android only), the app has to maintain a constant connection to the server, which consumes 
about 0-1% of battery in 17h of use (on my phone). There has been a ton of testing and improvement around this. I think it's pretty 
decent now.

## What is instant delivery?
[Instant delivery](subscribe/phone.md#instant-delivery) is a feature in the Android app. If turned on, the app maintains a constant connection to the
server and listens for incoming notifications. This consumes additional battery (see above),
but delivers notifications instantly.

## Can I disable the web app? Can I protect it with a login screen?
The web app is a static website without a backend (other than the ntfy API). All data is stored locally in the browser
cache and local storage. That means it does not need to be protected with a login screen, and it poses no additional 
security risk. So technically, it does not need to be disabled.

However, if you still want to disable it, you can do so with the `web-root: disable` option in the `server.yml` file. 

Think of the ntfy web app like an Android/iOS app. It is freely available and accessible to anyone, yet useless without
a proper backend. So as long as you secure your backend with ACLs, exposing the ntfy web app to the Internet is harmless.

## Where can I donate?
I have just very recently started accepting donations via [GitHub Sponsors](https://github.com/sponsors/binwiederhier).
I would be humbled if you helped me carry the server and developer account costs. Even small donations are very much
appreciated.
