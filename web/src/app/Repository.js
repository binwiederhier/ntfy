import {topicUrl} from "./utils";
import Subscription from "./Subscription";

export class Repository {
    loadSubscriptions() {
        console.log(`[Repository] Loading subscriptions from localStorage`);

        const subscriptions = {};
        const rawSubscriptions = localStorage.getItem('subscriptions');
        if (rawSubscriptions === null) {
            return {};
        }
        try {
            const serializedSubscriptions = JSON.parse(rawSubscriptions);
            serializedSubscriptions.forEach(s => {
                const subscription = new Subscription(s.baseUrl, s.topic);
                subscription.notifications = s.notifications;
                subscriptions[topicUrl(s.baseUrl, s.topic)] = subscription;
            });
            return subscriptions;
        } catch (e) {
            console.log("LocalStorage", `Unable to deserialize subscriptions: ${e.message}`)
            return {};
        }
    }

    saveSubscriptions(subscriptions) {
        return;
        console.log(`[Repository] Saving subscriptions ${subscriptions} to localStorage`);

        const serializedSubscriptions = Object.keys(subscriptions).map(k => {
            const subscription = subscriptions[k];
            return {
                baseUrl: subscription.baseUrl,
                topic: subscription.topic,
                notifications: subscription.notifications
            }
        });
        localStorage.setItem('subscriptions', JSON.stringify(serializedSubscriptions));
    }
}

const repository = new Repository();
export default repository;
