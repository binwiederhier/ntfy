class Connection {
    constructor(wsUrl, subscriptionId, onNotification) {
        this.wsUrl = wsUrl;
        this.subscriptionId = subscriptionId;
        this.onNotification = onNotification;
        this.ws = null;
    }

    start() {
        const socket = new WebSocket(this.wsUrl);
        socket.onopen = (event) => {
            console.log(`[Connection] [${this.subscriptionId}] Connection established`);
        }
        socket.onmessage = (event) => {
            console.log(`[Connection] [${this.subscriptionId}] Message received from server: ${event.data}`);
            try {
                const data = JSON.parse(event.data);
                const relevantAndValid =
                    data.event === 'message' &&
                    'id' in data &&
                    'time' in data &&
                    'message' in data;
                if (!relevantAndValid) {
                    return;
                }
                this.onNotification(this.subscriptionId, data);
            } catch (e) {
                console.log(`[Connection] [${this.subscriptionId}] Error handling message: ${e}`);
            }
        };
        socket.onclose = (event) => {
            if (event.wasClean) {
                console.log(`[Connection] [${this.subscriptionId}] Connection closed cleanly, code=${event.code} reason=${event.reason}`);
            } else {
                console.log(`[Connection] [${this.subscriptionId}] Connection died`);
            }
        };
        socket.onerror = (event) => {
            console.log(this.subscriptionId, `[Connection] [${this.subscriptionId}] ${event.message}`);
        };
        this.ws = socket;
    }

    cancel() {
        if (this.ws !== null) {
            this.ws.close();
            this.ws = null;
        }
    }
}

export default Connection;
