import notifier from "./Notifier";
import subscriptionManager from "./SubscriptionManager";

const onMessage = () => {
  notifier.playSound();
};

const delayMillis = 2000; // 2 seconds
const intervalMillis = 300000; // 5 minutes

class WebPushWorker {
  constructor() {
    this.timer = null;
  }

  startWorker() {
    if (this.timer !== null) {
      return;
    }

    this.timer = setInterval(() => this.updateSubscriptions(), intervalMillis);
    setTimeout(() => this.updateSubscriptions(), delayMillis);

    this.broadcastChannel = new BroadcastChannel("web-push-broadcast");
    this.broadcastChannel.addEventListener("message", onMessage);
  }

  stopWorker() {
    clearTimeout(this.timer);

    this.broadcastChannel.removeEventListener("message", onMessage);
    this.broadcastChannel.close();
  }

  async updateSubscriptions() {
    try {
      console.log("[WebPushBroadcastListener] Refreshing web push subscriptions");

      await subscriptionManager.refreshWebPushSubscriptions();
    } catch (e) {
      console.error("[WebPushBroadcastListener] Error refreshing web push subscriptions", e);
    }
  }
}

export default new WebPushWorker();
