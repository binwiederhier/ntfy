
export default class WsConnection {
    id = '';
    constructor(subscription, onChange) {
        this.id = subscription.id;
        this.subscription = subscription;
        this.onChange = onChange;
        this.ws = null;
    }
    start() {
        const socket = new WebSocket(this.subscription.wsUrl());
        socket.onopen = (event) => {
            console.log(this.id, "[open] Connection established");
        }
        socket.onmessage = (event) => {
            console.log(this.id, `[message] Data received from server: ${event.data}`);
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
                console.log('adding')
                this.subscription.addNotification(data);
                this.onChange(this.subscription);
            } catch (e) {
                console.log(this.id, `[message] Error handling message: ${e}`);
            }
        };
        socket.onclose = (event) => {
            if (event.wasClean) {
                console.log(this.id, `[close] Connection closed cleanly, code=${event.code} reason=${event.reason}`);
            } else {
                console.log(this.id, `[close] Connection died`);
                // e.g. server process killed or network down
                // event.code is usually 1006 in this case
            }
        };
        socket.onerror = (event) => {
            console.log(this.id, `[error] ${event.message}`);
        };
        this.ws = socket;
    }
    cancel() {
        if (this.ws != null) {
            this.ws.close();
        }
    }
}
