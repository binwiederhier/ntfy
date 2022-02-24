import Connection from "./Connection";

export class ConnectionManager {
    constructor() {
        this.connections = new Map();
    }

    refresh(subscriptions, onNotification) {
        console.log(`[ConnectionManager] Refreshing connections`);
        const subscriptionIds = subscriptions.ids();
        const deletedIds = Array.from(this.connections.keys()).filter(id => !subscriptionIds.includes(id));

        // Create and add new connections
        subscriptions.forEach((id, subscription) => {
            const added = !this.connections.get(id)
            if (added) {
                const wsUrl = subscription.wsUrl();
                const connection = new Connection(wsUrl, id, onNotification);
                this.connections.set(id, connection);
                console.log(`[ConnectionManager] Starting new connection ${id} using URL ${wsUrl}`);
                connection.start();
            }
        });

        // Delete old connections
        deletedIds.forEach(id => {
            console.log(`[ConnectionManager] Closing connection ${id}`);
            const connection = this.connections.get(id);
            this.connections.delete(id);
            connection.cancel();
        });
    }
}

const connectionManager = new ConnectionManager();
export default connectionManager;
