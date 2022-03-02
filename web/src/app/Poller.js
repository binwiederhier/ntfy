import db from "./db";
import api from "./Api";

const delayMillis = 3000; // 3 seconds
const intervalMillis = 300000; // 5 minutes

class Poller {
    constructor() {
        this.timer = null;
    }

    startWorker() {
        if (this.timer !== null) {
            return;
        }
        this.timer = setInterval(() => this.pollAll(), intervalMillis);
        setTimeout(() => this.pollAll(), delayMillis);
    }

    async pollAll() {
        console.log(`[Poller] Polling all subscriptions`);
        const subscriptions = await db.subscriptions.toArray();
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
        const notificationsWithSubscriptionId = notifications
            .map(notification => ({ ...notification, subscriptionId: subscription.id }));
        await db.notifications.bulkPut(notificationsWithSubscriptionId); // FIXME
        await db.subscriptions.update(subscription.id, {last: notifications.at(-1).id}); // FIXME
    };
}

const poller = new Poller();
export default poller;
