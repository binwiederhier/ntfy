# Using the progressive web app (PWA)
While ntfy doesn't have a native desktop app, it is built as a [progressive web app](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps) (PWA)
and thus can be installed on both desktop and mobile devices.

This gives it its own launcher (e.g. shortcut on Windows, app on macOS, launcher shortcut on Linux, home screen icon on iOS, and
launcher icon on Android), a standalone window, push notifications, and an app badge with the unread notification count.

Web app installation is supported on (see [compatibility table](https://caniuse.com/web-app-manifest) for details):

- Chrome on all platforms (Windows/Linux/macOS/Android/iOS)
- Firefox on Android natively, and on Windows/Linux ([via an extension](https://addons.mozilla.org/en-US/firefox/addon/pwas-for-firefox/))
- Edge on Windows
- Safari on macOS/iOS

<!-- TODO: (Q4 2023) Safari 17 / macOS 14 Sonoma supports installable PWAs too -->

## Installation

### Chrome/Safari on Desktop
To install and register the web app via Chrome, click the "install app" icon. After installation, you can find the app in your
app drawer:

<div id="pwa-screenshots-chrome-safari-desktop" class="screenshots">
    <a href="../../static/img/pwa-install.png"><img src="../../static/img/pwa-install.png"/></a>
    <a href="../../static/img/pwa.png"><img src="../../static/img/pwa.png"/></a> 
    <a href="../../static/img/pwa-badge.png"><img src="../../static/img/pwa-badge.png"/></a>

</div>

### Chrome on Android
For Chrome on Android, either click the "Add to Home Screen" banner at the bottom of the screen, or select "install app"
in the menu. After installation, you can find the app in your app drawer, and on your home screen.

<div id="pwa-screenshots-chrome-android" class="screenshots">
    <a href="../../static/img/pwa-install-chrome-android.jpg"><img src="../../static/img/pwa-install-chrome-android.jpg"/></a>
    <a href="../../static/img/pwa-install-chrome-android-menu.jpg"><img src="../../static/img/pwa-install-chrome-android-menu.jpg"/></a>
    <a href="../../static/img/pwa-install-chrome-android-popup.jpg"><img src="../../static/img/pwa-install-chrome-android-popup.jpg"/></a>
</div>

### Firefox on Android

XXXXXXXXXXX

### Safari on iOS
On iOS Safari, tap on the Share menu > "Add to Home Screen".

XXXXXXx

## Background notifications
Background notifications via web push are enabled by default and cannot be turned off when the app is installed, as notifications would
not be delivered reliably otherwise. You can mute topics you don't want to receive notifications for.

On desktop, you generally need either your browser or the web app open to receive notifications, though the ntfy tab doesn't need to be
open. On mobile, you don't need to have the web app open to receive notifications. Look at the [web docs](./web.md#background-notifications)
for a detailed breakdown.
