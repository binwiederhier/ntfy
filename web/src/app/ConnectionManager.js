import Connection from "./Connection";
import {sha256} from "./utils";

class ConnectionManager {
    constructor() {
        this.connections = new Map(); // ConnectionId -> Connection (hash, see below)
    }

    async refresh(subscriptions, users, onNotification) {
        if (!subscriptions || !users) {
            return;
        }
        console.log(`[ConnectionManager] Refreshing connections`);
        const subscriptionsWithUsersAndConnectionId = await Promise.all(subscriptions
            .map(async s => {
                const [user] = users.filter(u => u.baseUrl === s.baseUrl);
                const connectionId = await makeConnectionId(s, user);
                return {...s, user, connectionId};
            }));
        const activeIds = subscriptionsWithUsersAndConnectionId.map(s => s.connectionId);
        const deletedIds = Array.from(this.connections.keys()).filter(id => !activeIds.includes(id));

        console.log(subscriptionsWithUsersAndConnectionId);
        // Create and add new connections
        subscriptionsWithUsersAndConnectionId.forEach(subscription => {
            const subscriptionId = subscription.id;
            const connectionId = subscription.connectionId;
            const added = !this.connections.get(connectionId)
            if (added) {
                const baseUrl = subscription.baseUrl;
                const topic = subscription.topic;
                const user = subscription.user;
                const since = subscription.last;
                const connection = new Connection(connectionId, subscriptionId, baseUrl, topic, user, since, onNotification);
                this.connections.set(connectionId, connection);
                console.log(`[ConnectionManager] Starting new connection ${connectionId} (subscription ${subscriptionId} with user ${user ? user.username : "anonymous"})`);
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

const makeConnectionId = async (subscription, user) => {
    const hash = (user)
        ? await sha256(`${subscription.id}|${user.username}|${user.password}`)
        : await sha256(`${subscription.id}`);
    return hash.substring(0, 10);
}

const connectionManager = new ConnectionManager();
export default connectionManager;
