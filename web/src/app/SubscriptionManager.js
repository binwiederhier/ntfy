import db from "./db";

class SubscriptionManager {
    async all() {
        return db.subscriptions.toArray();
    }

    async get(subscriptionId) {
        return await db.subscriptions.get(subscriptionId)
    }

    async save(subscription) {
        await db.subscriptions.put(subscription);
    }

    async updateState(subscriptionId, state) {
        console.log(`Update state: ${subscriptionId} ${state}`)
        db.subscriptions.update(subscriptionId, { state: state });
    }

    async remove(subscriptionId) {
        await db.subscriptions.delete(subscriptionId);
        await db.notifications
            .where({subscriptionId: subscriptionId})
            .delete();
    }

    async first() {
        return db.subscriptions.toCollection().first(); // May be undefined
    }

    async getNotifications(subscriptionId) {
        return db.notifications
            .where({ subscriptionId: subscriptionId })
            .toArray();
    }

    /** Adds notification, or returns false if it already exists */
    async addNotification(subscriptionId, notification) {
        const exists = await db.notifications.get(notification.id);
        if (exists) {
            return false;
        }
        await db.notifications.add({ ...notification, subscriptionId });
        await db.subscriptions.update(subscriptionId, {
            last: notification.id
        });
        return true;
    }

    /** Adds/replaces notifications, will not throw if they exist */
    async addNotifications(subscriptionId, notifications) {
        const notificationsWithSubscriptionId = notifications
            .map(notification => ({ ...notification, subscriptionId }));
        const lastNotificationId = notifications.at(-1).id;
        await db.notifications.bulkPut(notificationsWithSubscriptionId);
        await db.subscriptions.update(subscriptionId, {
            last: lastNotificationId
        });
    }

    async deleteNotification(notificationId) {
        await db.notifications.delete(notificationId);
    }

    async deleteNotifications(subscriptionId) {
        await db.notifications
            .where({subscriptionId: subscriptionId})
            .delete();
    }

    async pruneNotifications(thresholdTimestamp) {
        await db.notifications
            .where("time").below(thresholdTimestamp)
            .delete();
    }
}

const subscriptionManager = new SubscriptionManager();
export default subscriptionManager;
