import {basicAuth, bearerAuth, encodeBase64Url, topicShortUrl, topicUrlWs} from "./utils";

const retryBackoffSeconds = [5, 10, 20, 30, 60, 120];

/**
 * A connection contains a single WebSocket connection for one topic. It handles its connection
 * status itself, including reconnect attempts and backoff.
 *
 * Incoming messages and state changes are forwarded via listeners.
 */
class Connection {
    constructor(connectionId, subscriptionId, baseUrl, topic, user, since, onNotification, onStateChanged) {
        this.connectionId = connectionId;
        this.subscriptionId = subscriptionId;
        this.baseUrl = baseUrl;
        this.topic = topic;
        this.user = user;
        this.since = since;
        this.shortUrl = topicShortUrl(baseUrl, topic);
        this.onNotification = onNotification;
        this.onStateChanged = onStateChanged;
        this.ws = null;
        this.retryCount = 0;
        this.retryTimeout = null;
    }

    start() {
        // Don't fetch old messages; we do that as a poll() when adding a subscription;
        // we don't want to re-trigger the main view re-render potentially hundreds of times.

        const wsUrl = this.wsUrl();
        console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Opening connection to ${wsUrl}`);

        this.ws = new WebSocket(wsUrl);
        this.ws.onopen = (event) => {
            console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Connection established`, event);
            this.retryCount = 0;
            this.onStateChanged(this.subscriptionId, ConnectionState.Connected);
        }
        this.ws.onmessage = (event) => {
            console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Message received from server: ${event.data}`);
            try {
                const data = JSON.parse(event.data);
                if (data.event === 'open') {
                    return;
                }
                const relevantAndValid =
                    data.event === 'message' &&
                    'id' in data &&
                    'time' in data &&
                    'message' in data;
                if (!relevantAndValid) {
                    console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Unexpected message. Ignoring.`);
                    return;
                }
                this.since = data.id;
                this.onNotification(this.subscriptionId, data);
            } catch (e) {
                console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Error handling message: ${e}`);
            }
        };
        this.ws.onclose = (event) => {
            if (event.wasClean) {
                console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Connection closed cleanly, code=${event.code} reason=${event.reason}`);
                this.ws = null;
            } else {
                const retrySeconds = retryBackoffSeconds[Math.min(this.retryCount, retryBackoffSeconds.length-1)];
                this.retryCount++;
                console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Connection died, retrying in ${retrySeconds} seconds`);
                this.retryTimeout = setTimeout(() => this.start(), retrySeconds * 1000);
                this.onStateChanged(this.subscriptionId, ConnectionState.Connecting);
            }
        };
        this.ws.onerror = (event) => {
            console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Error occurred: ${event}`, event);
        };
    }

    close() {
        console.log(`[Connection, ${this.shortUrl}, ${this.connectionId}] Closing connection`);
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

    wsUrl() {
        const params = [];
        if (this.since) {
            params.push(`since=${this.since}`);
        }
        if (this.user) {
            params.push(`auth=${this.authParam()}`);
        }
        const wsUrl = topicUrlWs(this.baseUrl, this.topic);
        return (params.length === 0) ? wsUrl : `${wsUrl}?${params.join('&')}`;
    }

    authParam() {
        if (this.user.password) {
            return encodeBase64Url(basicAuth(this.user.username, this.user.password));
        }
        return encodeBase64Url(bearerAuth(this.user.token));
    }
}

export class ConnectionState {
    static Connected = "connected";
    static Connecting = "connecting";
}

export default Connection;
