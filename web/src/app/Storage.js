import {topicUrl} from "./utils";
import Subscription from "./Subscription";

const LocalStorage = {
    getSubscriptions() {
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
    },
    saveSubscriptions(subscriptions) {
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
};

export default LocalStorage;
