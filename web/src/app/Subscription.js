import {topicUrl, shortTopicUrl, topicUrlWs} from './utils';

export default class Subscription {
    constructor(baseUrl, topic) {
        this.id = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.notifications = new Map();
    }

    addNotification(notification) {
        this.notifications.set(notification.id, notification);
        return this;
    }

    addNotifications(notifications) {
        notifications.forEach(n => this.addNotification(n));
        return this;
    }

    getNotifications() {
        return Array.from(this.notifications.values());
    }

    shortUrl() {
        return shortTopicUrl(this.baseUrl, this.topic);
    }
}
