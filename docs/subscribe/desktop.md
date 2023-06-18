# Using the web app as an installed web app
While ntfy doesn't have a native desktop app, it is built as a [progressive web app](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps) (PWA)
and thus can be installed on both desktop and mobile devices. This gives it its own launcher (e.g. shortcut on Windows, app on 
macOS, launcher shortcut on Linux), own window, push notifications, and an app badge with the unread notification count.

To install and register the web app in your operating system, click the "install app" icon in your browser (usually next to the
address bar). To receive background notifications, **either the browser or the installed web app must be open**.

<!-- TODO: (Q4 2023) Safari 17 / macOS 14 Sonoma supports installable PWAs too -->

Web app installation is supported on Chrome and Edge on desktop, as well as Chrome on Android and Safari on iOS.
Look at the [compatibility table](https://caniuse.com/web-app-manifest) for more info.

<div id="pwa-screenshots" class="screenshots">
    <a href="../../static/img/pwa.png"><img src="../../static/img/pwa.png"/></a> 
    <a href="../../static/img/pwa-install.png"><img src="../../static/img/pwa-install.png"/></a>
    <a href="../../static/img/pwa-badge.png"><img src="../../static/img/pwa-badge.png"/></a>
</div>
