import Connection from "./Connection";

class ConnectionManager {
    constructor() {
        this.connections = new Map();
    }

    refresh(subscriptions, users, onNotification) {
        console.log(`[ConnectionManager] Refreshing connections`);
        const subscriptionIds = subscriptions.ids();
        const deletedIds = Array.from(this.connections.keys()).filter(id => !subscriptionIds.includes(id));

        // Create and add new connections
        subscriptions.forEach((id, subscription) => {
            const added = !this.connections.get(id)
            if (added) {
                const baseUrl = subscription.baseUrl;
                const topic = subscription.topic;
                const user = users.get(baseUrl);
                const since = 0;
                const connection = new Connection(id, baseUrl, topic, user, since, onNotification);
                this.connections.set(id, connection);
                console.log(`[ConnectionManager] Starting new connection ${id}`);
                connection.start();
            }
        });

        // Delete old connections
        deletedIds.forEach(id => {
            console.log(`[ConnectionManager] Closing connection ${id}`);
            const connection = this.connections.get(id);
            this.connections.delete(id);
            connection.close();
        });
    }
}

const connectionManager = new ConnectionManager();
export default connectionManager;
