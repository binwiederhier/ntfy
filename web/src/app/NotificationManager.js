import {formatMessage, formatTitleWithFallback, topicShortUrl} from "./utils";
import repository from "./Repository";

class NotificationManager {
    notify(subscription, notification, onClickFallback) {
        if (!this.shouldNotify(subscription, notification)) {
            return;
        }
        const message = formatMessage(notification);
        const title = formatTitleWithFallback(notification, topicShortUrl(subscription.baseUrl, subscription.topic));
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

    shouldNotify(subscription, notification) {
        const priority = (notification.priority) ? notification.priority : 3;
        if (priority < repository.getMinPriority()) {
            return false;
        }
        return true;
    }
}

const notificationManager = new NotificationManager();
export default notificationManager;
