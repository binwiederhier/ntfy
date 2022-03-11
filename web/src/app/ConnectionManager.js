import Connection from "./Connection";
import {sha256} from "./utils";

/**
 * The connection manager keeps track of active connections (WebSocket connections, see Connection).
 *
 * Its refresh() method reconciles state changes with the target state by closing/opening connections
 * as required. This is done pretty much exactly the same way as in the Android app.
 */
class ConnectionManager {
    constructor() {
        this.connections = new Map(); // ConnectionId -> Connection (hash, see below)
        this.stateListener = null; // Fired when connection state changes
        this.notificationListener = null; // Fired when new notifications arrive
    }

    registerStateListener(listener) {
        this.stateListener = listener;
    }

    resetStateListener() {
        this.stateListener = null;
    }

    registerNotificationListener(listener) {
        this.notificationListener = listener;
    }

    resetNotificationListener() {
        this.notificationListener = null;
    }

    /**
     * This function figures out which websocket connections should be running by comparing the
     * current state of the world (connections) with the target state (targetIds).
     *
     * It uses a "connectionId", which is sha256($subscriptionId|$username|$password) to identify
     * connections. If any of them change, the connection is closed/replaced.
     */
    async refresh(subscriptions, users) {
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
        const targetIds = subscriptionsWithUsersAndConnectionId.map(s => s.connectionId);
        const deletedIds = Array.from(this.connections.keys()).filter(id => !targetIds.includes(id));

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
                const connection = new Connection(
                    connectionId,
                    subscriptionId,
                    baseUrl,
                    topic,
                    user,
                    since,
                    (subscriptionId, notification) => this.notificationReceived(subscriptionId, notification),
                    (subscriptionId, state) => this.stateChanged(subscriptionId, state)
                );
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

    stateChanged(subscriptionId, state) {
        if (this.stateListener) {
            try {
                this.stateListener(subscriptionId, state);
            } catch (e) {
                console.error(`[ConnectionManager] Error updating state of ${subscriptionId} to ${state}`, e);
            }
        }
    }

    notificationReceived(subscriptionId, notification) {
        if (this.notificationListener) {
            try {
                this.notificationListener(subscriptionId, notification);
            } catch (e) {
                console.error(`[ConnectionManager] Error handling notification for ${subscriptionId}`, e);
            }
        }
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
