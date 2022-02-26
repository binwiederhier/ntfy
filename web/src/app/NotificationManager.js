import {formatMessage, formatTitle} from "./utils";

class NotificationManager {
    notify(subscription, notification, onClickFallback) {
        const message = formatMessage(notification);
        const title = formatTitle(notification);
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
}

const notificationManager = new NotificationManager();
export default notificationManager;
