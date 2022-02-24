import Subscription from "./Subscription";
import Subscriptions from "./Subscriptions";

export class Repository {
    loadSubscriptions() {
        console.log(`[Repository] Loading subscriptions from localStorage`);

        const subscriptions = new Subscriptions();
        const serialized = localStorage.getItem('subscriptions');
        if (serialized === null) return subscriptions;

        try {
            const serializedSubscriptions = JSON.parse(serialized);
            serializedSubscriptions.forEach(s => {
                const subscription = new Subscription(s.baseUrl, s.topic);
                subscription.addNotifications(s.notifications);
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
}

const repository = new Repository();
export default repository;
