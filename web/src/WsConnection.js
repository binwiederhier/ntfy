
export default class WsConnection {
    constructor(url) {
        this.url = url;
        this.ws = null;
    }
    start() {
        const socket = new WebSocket(this.url);
        socket.onopen = function(e) {
            console.log(this.url, "[open] Connection established");
        };
        socket.onmessage = function(event) {
            console.log(this.url, `[message] Data received from server: ${event.data}`);
        };
        socket.onclose = function(event) {
            if (event.wasClean) {
                console.log(this.url, `[close] Connection closed cleanly, code=${event.code} reason=${event.reason}`);
            } else {
                console.log(this.url, `[close] Connection died`);
                // e.g. server process killed or network down
                // event.code is usually 1006 in this case
            }
        };
        socket.onerror = function(error) {
            console.log(this.url, `[error] ${error.message}`);
        };
        this.ws = socket;
    }
    cancel() {
        if (this.ws != null) {
            this.ws.close();
        }
    }
}
