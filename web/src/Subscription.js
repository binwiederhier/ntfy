import {topicUrl, shortTopicUrl, topicUrlWs} from './utils';

export default class Subscription {
    url = '';
    baseUrl = '';
    topic = '';
    notifications = [];
    constructor(baseUrl, topic) {
        this.url = topicUrl(baseUrl, topic);
        this.baseUrl = baseUrl;
        this.topic = topic;
    }
    wsUrl() {
        return topicUrlWs(this.baseUrl, this.topic);
    }
    shortUrl() {
        return shortTopicUrl(this.baseUrl, this.topic);
    }
}
