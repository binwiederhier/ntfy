import {topicShortUrl, topicUrl} from './utils';

class Subscription {
    constructor(baseUrl, topic) {
        this.id = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.last = null; // Last message ID
    }

    addNotification(notification) {
        if (!notification.event || notification.event !== 'message') {
            return false;
        }
        this.last = notification.id;
        return true;
    }

    addNotifications(notifications) {
        notifications.forEach(n => this.addNotification(n));
        return this;
    }

    url() {
        return topicUrl(this.baseUrl, this.topic);
    }

    shortUrl() {
        return topicShortUrl(this.baseUrl, this.topic);
    }
}

export default Subscription;
