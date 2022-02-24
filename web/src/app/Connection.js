import {shortTopicUrl, topicUrlWs} from "./utils";

const retryBackoffSeconds = [5, 10, 15, 20, 30, 45, 60, 120];

class Connection {
    constructor(subscriptionId, baseUrl, topic, since, onNotification) {
        this.subscriptionId = subscriptionId;
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.since = since;
        this.shortUrl = shortTopicUrl(baseUrl, topic);
        this.onNotification = onNotification;
        this.ws = null;
        this.retryCount = 0;
        this.retryTimeout = null;
    }

    start() {
        const since = (this.since === 0) ? "all" : this.since.toString();
        const wsUrl = topicUrlWs(this.baseUrl, this.topic, since);
        console.log(`[Connection, ${this.shortUrl}] Opening connection to ${wsUrl}`);
        this.ws = new WebSocket(wsUrl);
        this.ws.onopen = (event) => {
            console.log(`[Connection, ${this.shortUrl}] Connection established`, event);
            this.retryCount = 0;
        }
        this.ws.onmessage = (event) => {
            console.log(`[Connection, ${this.shortUrl}] Message received from server: ${event.data}`);
            try {
                const data = JSON.parse(event.data);
                const relevantAndValid =
                    data.event === 'message' &&
                    'id' in data &&
                    'time' in data &&
                    'message' in data;
                if (!relevantAndValid) {
                    console.log(`[Connection, ${this.shortUrl}] Message irrelevant or invalid. Ignoring.`);
                    return;
                }
                this.since = data.time;
                this.onNotification(this.subscriptionId, data);
            } catch (e) {
                console.log(`[Connection, ${this.shortUrl}] Error handling message: ${e}`);
            }
        };
        this.ws.onclose = (event) => {
            if (event.wasClean) {
                console.log(`[Connection, ${this.shortUrl}] Connection closed cleanly, code=${event.code} reason=${event.reason}`);
                this.ws = null;
            } else {
                const retrySeconds = retryBackoffSeconds[Math.min(this.retryCount, retryBackoffSeconds.length-1)];
                this.retryCount++;
                console.log(`[Connection, ${this.shortUrl}] Connection died, retrying in ${retrySeconds} seconds`);
                this.retryTimeout = setTimeout(() => this.start(), retrySeconds * 1000);
            }
        };
        this.ws.onerror = (event) => {
            console.log(`[Connection, ${this.shortUrl}] Error occurred: ${event}`, event);
        };
    }

    close() {
        console.log(`[Connection, ${this.shortUrl}] Closing connection`);
        const socket = this.ws;
        const retryTimeout = this.retryTimeout;
        if (socket !== null) {
            socket.close();
        }
        if (retryTimeout !== null) {
            clearTimeout(retryTimeout);
        }
        this.retryTimeout = null;
        this.ws = null;
    }
}

export default Connection;
