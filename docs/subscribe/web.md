# Subscribe from the web app
The web app lets you subscribe and publish messages to ntfy topics. For ntfy.sh, the web app is available at [ntfy.sh/app](https://ntfy.sh/app).
To subscribe, simply type in the topic name and click the *Subscribe* button. **After subscribing, messages published to the topic
will appear in the web app, and pop up as a notification.**

<div id="subscribe-screenshots" class="screenshots">
    <a href="../../static/img/web-subscribe.png"><img src="../../static/img/web-subscribe.png"/></a> 
</div>

## Publish messages
To learn how to send messages, check out the [publishing page](../publish.md).

<div id="web-screenshots" class="screenshots">
    <a href="../../static/img/web-detail.png"><img src="../../static/img/web-detail.png"/></a> 
    <a href="../../static/img/web-notification.png"><img src="../../static/img/web-notification.png"/></a>
</div>

## Topic reservations
If topic reservations are enabled, you can claim ownership over topics and define access to it:

<div id="reserve-screenshots" class="screenshots">
    <a href="../../static/img/web-reserve-topic.png"><img src="../../static/img/web-reserve-topic.png"/></a> 
    <a href="../../static/img/web-reserve-topic-dialog.png"><img src="../../static/img/web-reserve-topic-dialog.png"/></a>
</div>

## Background Notifications

While subscribing, you have the option to enable background notifications on supported browsers (see "Settings" tab).

Note: If you add the web app to your homescreen (as a progressive web app, more info in the [installed web app](./installed-web-app.md)
docs), you cannot turn these off, as notifications would not be delivered reliably otherwise. You can mute topics you don't want to receive
notifications for.

**If background notifications are off:** This requires an active ntfy tab to be open to receive notifications. 
These are typically instantaneous, and will appear as a system notification. If you don't see these, check that your browser 
is allowed to show notifications (for example in System Settings on macOS). If you don't want to enable background notifications, 
**pinning the ntfy tab on your browser** is a good solution to leave it running.

**If background notifications are on:** This uses the [Web Push API](https://caniuse.com/push-api). You don't need an active 
ntfy tab open, but in some cases you may need to keep your browser open. Background notifications are only supported on the 
same server hosting the web app. You cannot use another server, but can instead subscribe on the other server itself.

If the ntfy app is not opened for more than a week, background notifications will be paused. You can resume them
by opening the app again, and will get a warning notification before they are paused.

| Browser | Platform | Browser Running | Browser Not Running | Restrictions                                            |
|---------|----------|-----------------|---------------------|---------------------------------------------------------|
| Chrome  | Desktop  | ✅               | ❌                   |                                                         |
| Firefox | Desktop  | ✅               | ❌                   |                                                         |
| Edge    | Desktop  | ✅               | ❌                   |                                                         |
| Opera   | Desktop  | ✅               | ❌                   |                                                         |
| Safari  | Desktop  | ✅               | ✅                   | requires Safari 16.1, macOS 13 Ventura                  |
| Chrome  | Android  | ✅               | ✅                   |                                                         |
| Safari  | iOS      | ⚠️              | ⚠️                  | requires iOS 16.4, only when app is added to homescreen |

(Browsers below 1% usage not shown, look at the [Push API](https://caniuse.com/push-api) for more info)
