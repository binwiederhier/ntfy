import {topicUrl, shortTopicUrl, topicUrlWs} from './utils';

export default class Subscription {
    id = '';
    baseUrl = '';
    topic = '';
    notifications = [];
    lastActive = null;
    constructor(baseUrl, topic) {
        this.id = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
    }
    addNotification(notification) {
        if (notification.time === null) {
            return;
        }
        this.notifications.push(notification);
        this.lastActive = notification.time;
    }
    url() {
        return this.id;
    }
    wsUrl() {
        return topicUrlWs(this.baseUrl, this.topic);
    }
    shortUrl() {
        return shortTopicUrl(this.baseUrl, this.topic);
    }
}
