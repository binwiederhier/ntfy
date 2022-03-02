import Subscription from "./Subscription";
import Subscriptions from "./Subscriptions";
import db from "./db";

class Repository {
    loadSubscriptions() {
        console.log(`[Repository] Loading subscriptions from localStorage`);
        const subscriptions = new Subscriptions();
        subscriptions.loaded = true;
        const serialized = localStorage.getItem('subscriptions');
        if (serialized === null) {
            return subscriptions;
        }
        try {
            JSON.parse(serialized).forEach(s => {
                const subscription = new Subscription(s.baseUrl, s.topic);
                subscription.addNotifications(s.notifications);
                subscription.last = s.last; // Explicitly set, in case old notifications have been deleted
                subscriptions.add(subscription);
            });
            console.log(`[Repository] Loaded ${subscriptions.size()} subscription(s) from localStorage`);
            return subscriptions;
        } catch (e) {
            console.log(`[Repository] Unable to deserialize subscriptions: ${e.message}`);
            return subscriptions;
        }
    }

    saveSubscriptions(subscriptions) {
        if (!subscriptions.loaded) {
            return; // Avoid saving invalid state, triggered by initial useEffect hook
        }
        console.log(`[Repository] Saving ${subscriptions.size()} subscription(s) to localStorage`);
        const serialized = JSON.stringify(subscriptions.map( (id, subscription) => {
            return {
                baseUrl: subscription.baseUrl,
                topic: subscription.topic,
                notifications: subscription.getNotifications(),
                last: subscription.last
            }
        }));
        localStorage.setItem('subscriptions', serialized);
    }

    async setSelectedSubscriptionId(selectedSubscriptionId) {
        console.log(`[Repository] Saving selected subscription ${selectedSubscriptionId}`);
        db.prefs.put({key: 'selectedSubscriptionId', value: selectedSubscriptionId});
    }

    async getSelectedSubscriptionId() {
        console.log(`[Repository] Loading selected subscription ID`);
        const selectedSubscriptionId = await db.prefs.get('selectedSubscriptionId');
        return (selectedSubscriptionId) ? selectedSubscriptionId.value : "";
    }

    async setMinPriority(minPriority) {
        db.prefs.put({key: 'minPriority', value: minPriority.toString()});
    }

    async getMinPriority() {
        const minPriority = await db.prefs.get('minPriority');
        return (minPriority) ? Number(minPriority.value) : 1;
    }

    minPriority() {
        return db.prefs.get('minPriority');
    }

    async setDeleteAfter(deleteAfter) {
        db.prefs.put({key:'deleteAfter', value: deleteAfter.toString()});
    }

    async getDeleteAfter() {
        const deleteAfter = await db.prefs.get('deleteAfter');
        return (deleteAfter) ? Number(deleteAfter.value) : 604800; // Default is one week
    }

    deleteAfter() {
        return db.prefs.get('deleteAfter');
    }
}

const repository = new Repository();
export default repository;
