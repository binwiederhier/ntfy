import {formatMessage, formatTitleWithFallback, topicShortUrl} from "./utils";
import prefs from "./Prefs";
import subscriptionManager from "./SubscriptionManager";

class NotificationManager {
    async notify(subscriptionId, notification, onClickFallback) {
        const subscription = await subscriptionManager.get(subscriptionId);
        const shouldNotify = await this.shouldNotify(subscription, notification);
        if (!shouldNotify) {
            return;
        }
        const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
        const message = formatMessage(notification);
        const title = formatTitleWithFallback(notification, shortUrl);

        console.log(`[NotificationManager, ${shortUrl}] Displaying notification ${notification.id}: ${message}`);
        const n = new Notification(title, {
            body: message,
            icon: '/static/img/favicon.png'
        });
        if (notification.click) {
            n.onclick = (e) => window.open(notification.click);
        } else {
            n.onclick = onClickFallback;
        }
    }

    granted() {
        return Notification.permission === 'granted';
    }

    maybeRequestPermission(cb) {
        if (!this.granted()) {
            Notification.requestPermission().then((permission) => {
                const granted = permission === 'granted';
                cb(granted);
            });
        }
    }

    async shouldNotify(subscription, notification) {
        const priority = (notification.priority) ? notification.priority : 3;
        const minPriority = await prefs.minPriority();
        if (priority < minPriority) {
            return false;
        }
        return true;
    }
}

const notificationManager = new NotificationManager();
export default notificationManager;
