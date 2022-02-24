import {shortTopicUrl, topicUrl} from './utils';

export default class Subscription {
    constructor(baseUrl, topic) {
        this.id = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.notifications = new Map(); // notification ID -> notification object
        this.deleted = new Set(); // notification IDs
    }

    addNotification(notification) {
        if (this.notifications.has(notification.id) || this.deleted.has(notification.id)) {
            return this;
        }
        this.notifications.set(notification.id, notification);
        return this;
    }

    addNotifications(notifications) {
        notifications.forEach(n => this.addNotification(n));
        return this;
    }

    deleteNotification(notificationId) {
        this.notifications.delete(notificationId);
        this.deleted.add(notificationId);
        return this;
    }

    deleteAllNotifications() {
        console.log(this.notifications);
        for (const [id] of this.notifications) {
            console.log(`delete ${id}`);
            this.deleteNotification(id);
        }
        return this;
    }

    getNotifications() {
        return Array.from(this.notifications.values());
    }

    shortUrl() {
        return shortTopicUrl(this.baseUrl, this.topic);
    }
}
