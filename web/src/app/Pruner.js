import prefs from "./Prefs";
import subscriptionManager from "./SubscriptionManager";

const delayMillis = 15000; // 15 seconds
const intervalMillis = 1800000; // 30 minutes

class Pruner {
    constructor() {
        this.timer = null;
    }

    startWorker() {
        if (this.timer !== null) {
            return;
        }
        console.log(`[Pruner] Starting worker`);
        this.timer = setInterval(() => this.prune(), intervalMillis);
        setTimeout(() => this.prune(), delayMillis);
    }

    async prune() {
        const deleteAfterSeconds = await prefs.deleteAfter();
        const pruneThresholdTimestamp = Math.round(Date.now()/1000) - deleteAfterSeconds;
        if (deleteAfterSeconds === 0) {
            console.log(`[Pruner] Pruning is disabled. Skipping.`);
            return;
        }
        console.log(`[Pruner] Pruning notifications older than ${deleteAfterSeconds}s (timestamp ${pruneThresholdTimestamp})`);
        try {
            await subscriptionManager.pruneNotifications(pruneThresholdTimestamp);
        } catch (e) {
            console.log(`[Pruner] Error pruning old subscriptions`, e);
        }
    }
}

const pruner = new Pruner();
pruner.startWorker();

export default pruner;
