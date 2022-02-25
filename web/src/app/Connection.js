import {shortTopicUrl, topicUrlWs, topicUrlWsWithSince} from "./utils";

const retryBackoffSeconds = [5, 10, 15, 20, 30, 45];

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
        // Don't fetch old messages; we do that as a poll() when adding a subscription;
        // we don't want to re-trigger the main view re-render potentially hundreds of times.
        const wsUrl = (this.since === 0)
            ? topicUrlWs(this.baseUrl, this.topic)
            : topicUrlWsWithSince(this.baseUrl, this.topic, this.since.toString());
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
                this.since = data.time + 1; // Sigh. This works because on reconnect, we wait 5+ seconds anyway.
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
