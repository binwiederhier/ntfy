# Subscribe from the Web UI

You can use the Web UI to subscribe to topics as well. Simply type in the topic name and click the *Subscribe* button.

While subscribing, you have the option to enable desktop notifications, as well as background notifications. When you
enable them for the first time, you will be prompted to allow notifications on your browser.

- **Sound only**

    If you don't enable browser notifications, a sound will play when a new notification comes in, and the tab title
    will show the number of new notifications.

- **Browser Notifications**

    This requires an active ntfy tab to be open to receive notifications. These are typically instantaneous, and will
    appear as a system notification. If you don't see these, check that your browser is allowed to show notifications
    (for example in System Settings on macOS).

    If you don't want to enable background notifications, pinning the ntfy tab on your browser is a good solution to leave
    it running.

- **Background Notifications**

    This uses the [Web Push API](https://caniuse.com/push-api). You don't need an active ntfy tab open, but in some
    cases you may need to keep your browser open.

    Background notifications are only supported on the same server hosting the web app. You cannot use another server,
    but can instead subscribe on the other server itself.

    | Browser | Platform | Browser Running | Browser Not Running | Restrictions                                            |
    | ------- | -------- | --------------- | ------------------- | ------------------------------------------------------- |
    | Chrome  | Desktop  | ✅              | ❌                  |                                                         |
    | Firefox | Desktop  | ✅              | ❌                  |                                                         |
    | Edge    | Desktop  | ✅              | ❌                  |                                                         |
    | Opera   | Desktop  | ✅              | ❌                  |                                                         |
    | Safari  | Desktop  | ✅              | ✅                  | requires Safari 16.1, macOS 13 Ventura                  |
    | Chrome  | Android  | ✅              | ✅                  |                                                         |
    | Safari  | iOS      | ⚠️               | ⚠️                   | requires iOS 16.4, only when app is added to homescreen |

    (Browsers below 1% usage not shown, look at the [Push API](https://caniuse.com/push-api) for more info)

To learn how to send messages, check out the [publishing page](../publish.md).

<div id="web-screenshots" class="screenshots">
    <a href="../../static/img/web-detail.png"><img src="../../static/img/web-detail.png"/></a> 
    <a href="../../static/img/web-notification.png"><img src="../../static/img/web-notification.png"/></a>
    <a href="../../static/img/web-subscribe.png"><img src="../../static/img/web-subscribe.png"/></a>
</div>

If topic reservations are enabled, you can claim ownership over topics and define access to it:

<div id="reserve-screenshots" class="screenshots">
    <a href="../../static/img/web-reserve-topic.png"><img src="../../static/img/web-reserve-topic.png"/></a> 
    <a href="../../static/img/web-reserve-topic-dialog.png"><img src="../../static/img/web-reserve-topic-dialog.png"/></a>
</div>

You can set your default choice for new subscriptions (for example synced account subscriptions and the default toggle state)
in the settings page:

<div id="push-settings-screenshots" class="screenshots">
    <a href="../../static/img/web-push-settings.png"><img src="../../static/img/web-push-settings.png"/></a> 
</div>
