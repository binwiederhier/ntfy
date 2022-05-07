import db from "./db";
import {topicUrl} from "./utils";

class SubscriptionManager {
    /** All subscriptions, including "new count"; this is a JOIN, see https://dexie.org/docs/API-Reference#joining */
    async all() {
        const subscriptions = await db.subscriptions.toArray();
        await Promise.all(subscriptions.map(async s => {
            s.new = await db.notifications
                .where({ subscriptionId: s.id, new: 1 })
                .count();
        }));
        return subscriptions;
    }

    async get(subscriptionId) {
        return await db.subscriptions.get(subscriptionId)
    }

    async add(baseUrl, topic) {
        const subscription = {
            id: topicUrl(baseUrl, topic),
            baseUrl: baseUrl,
            topic: topic,
            mutedUntil: 0,
            last: null
        };
        await db.subscriptions.put(subscription);
        return subscription;
    }

    async updateState(subscriptionId, state) {
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
        // This is quite awkward, but it is the recommended approach as per the Dexie docs.
        // It's actually fine, because the reading and filtering is quite fast. The rendering is what's
        // killing performance. See  https://dexie.org/docs/Collection/Collection.offset()#a-better-paging-approach

        return db.notifications
            .orderBy("time") // Sort by time first
            .filter(n => n.subscriptionId === subscriptionId)
            .reverse()
            .toArray();
    }

    async getAllNotifications() {
        return db.notifications
            .orderBy("time") // Efficient, see docs
            .reverse()
            .toArray();
    }

    /** Adds notification, or returns false if it already exists */
    async addNotification(subscriptionId, notification) {
        const exists = await db.notifications.get(notification.id);
        if (exists) {
            return false;
        }
        try {
            notification.new = 1; // New marker (used for bubble indicator); cannot be boolean; Dexie index limitation
            await db.notifications.add({ ...notification, subscriptionId }); // FIXME consider put() for double tab
            await db.subscriptions.update(subscriptionId, {
                last: notification.id
            });
        } catch (e) {
            console.error(`[SubscriptionManager] Error adding notification`, e);
        }
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

    async updateNotification(notification) {
        const exists = await db.notifications.get(notification.id);
        if (!exists) {
            return false;
        }
        try {
            await db.notifications.put({ ...notification });
        } catch (e) {
            console.error(`[SubscriptionManager] Error updating notification`, e);
        }
        return true;
    }

    async deleteNotification(notificationId) {
        await db.notifications.delete(notificationId);
    }

    async deleteNotifications(subscriptionId) {
        await db.notifications
            .where({subscriptionId: subscriptionId})
            .delete();
    }

    async markNotificationRead(notificationId) {
        await db.notifications
            .where({id: notificationId, new: 1})
            .modify({new: 0});
    }

    async markNotificationsRead(subscriptionId) {
        await db.notifications
            .where({subscriptionId: subscriptionId, new: 1})
            .modify({new: 0});
    }

    async setMutedUntil(subscriptionId, mutedUntil) {
        await db.subscriptions.update(subscriptionId, {
            mutedUntil: mutedUntil
        });
    }

    async pruneNotifications(thresholdTimestamp) {
        await db.notifications
            .where("time").below(thresholdTimestamp)
            .delete();
    }
}

const subscriptionManager = new SubscriptionManager();
export default subscriptionManager;
