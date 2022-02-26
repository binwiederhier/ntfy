import {topicShortUrl, topicUrl} from './utils';

class Subscription {
    constructor(baseUrl, topic) {
        this.id = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.notifications = new Map(); // notification ID -> notification object
        this.last = 0;
    }

    addNotification(notification) {
        if (this.notifications.has(notification.id) || notification.time < this.last) {
            return false;
        }
        this.notifications.set(notification.id, notification);
        this.last = notification.time;
        return true;
    }

    addNotifications(notifications) {
        notifications.forEach(n => this.addNotification(n));
        return this;
    }

    deleteNotification(notificationId) {
        this.notifications.delete(notificationId);
        return this;
    }

    deleteAllNotifications() {
        for (const [id] of this.notifications) {
            this.deleteNotification(id);
        }
        return this;
    }

    getNotifications() {
        return Array.from(this.notifications.values());
    }

    url() {
        return topicUrl(this.baseUrl, this.topic);
    }

    shortUrl() {
        return topicShortUrl(this.baseUrl, this.topic);
    }
}

export default Subscription;
