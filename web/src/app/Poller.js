import api from "./Api";
import subscriptionManager from "./SubscriptionManager";

const delayMillis = 8000; // 8 seconds
const intervalMillis = 300000; // 5 minutes

class Poller {
    constructor() {
        this.timer = null;
    }

    startWorker() {
        if (this.timer !== null) {
            return;
        }
        console.log(`[Poller] Starting worker`);
        this.timer = setInterval(() => this.pollAll(), intervalMillis);
        setTimeout(() => this.pollAll(), delayMillis);
    }

    async pollAll() {
        console.log(`[Poller] Polling all subscriptions`);
        const subscriptions = await subscriptionManager.all();
        for (const s of subscriptions) {
            try {
                await this.poll(s);
            } catch (e) {
                console.log(`[Poller] Error polling ${s.id}`, e);
            }
        }
    }

    async poll(subscription) {
        console.log(`[Poller] Polling ${subscription.id}`);

        const since = subscription.last;
        const notifications = await api.poll(subscription.baseUrl, subscription.topic, since);
        if (!notifications || notifications.length === 0) {
            console.log(`[Poller] No new notifications found for ${subscription.id}`);
            return;
        }
        console.log(`[Poller] Adding ${notifications.length} notification(s) for ${subscription.id}`);
        await subscriptionManager.addNotifications(subscription.id, notifications);
    }

    pollInBackground(subscription) {
        const fn = async () => {
            try {
                await this.poll(subscription);
            } catch (e) {
                console.error(`[App] Error polling subscription ${subscription.id}`, e);
            }
        };
        setTimeout(() => fn(), 0);
    }
}

const poller = new Poller();
poller.startWorker();

export default poller;
