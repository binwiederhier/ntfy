# Using the web app as an installed web app

While ntfy doesn't have a native desktop app, it is built as a [progressive web app](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps) (PWA)
and thus can be installed on both desktop and mobile devices.

This gives it its own launcher (e.g. shortcut on Windows, app on macOS, launcher shortcut on Linux, home screen icon on iOS, and
launcher icon on Android), a standalone window, push notifications, and an app badge with the unread notification count.

To install and register the web app in your operating system, click the "install app" icon in your browser (usually next to the
address bar). On iOS Safari, tap on the Share menu > "Add to Home Screen".

## Background Notifications

Background notifications via web push are enabled by default and cannot be turned off when the app is installed, as notifications would
not be delivered reliably otherwise. You can mute topics you don't want to receive notifications for.

On desktop, you generally need either your browser or the web app open to receive notifications, though the ntfy tab doesn't need to be
open. On mobile, you don't need to have the web app open to receive notifications. Look at the [web docs](./web.md#background-notifications)
for a detailed breakdown.

## Compatibility

<!-- TODO: (Q4 2023) Safari 17 / macOS 14 Sonoma supports installable PWAs too -->

Web app installation is supported on Chrome and Edge on desktop, as well as Chrome on Android and Safari on iOS.
Look at the [compatibility table](https://caniuse.com/web-app-manifest) for more info.

<div id="pwa-screenshots" class="screenshots">
    <a href="../../static/img/pwa.png"><img src="../../static/img/pwa.png"/></a> 
    <a href="../../static/img/pwa-install.png"><img src="../../static/img/pwa-install.png"/></a>
    <a href="../../static/img/pwa-badge.png"><img src="../../static/img/pwa-badge.png"/></a>
</div>
