import { useState, useEffect } from "react";
import { useLiveQuery } from "dexie-react-hooks";
import notifier from "./Notifier";
import subscriptionManager from "./SubscriptionManager";

export const useWebPushUpdateWorker = () => {
  const topics = useLiveQuery(() => subscriptionManager.webPushTopics());
  const [lastTopics, setLastTopics] = useState();

  useEffect(() => {
    if (!notifier.pushPossible() || JSON.stringify(topics) === JSON.stringify(lastTopics)) {
      return;
    }

    (async () => {
      try {
        console.log("[useWebPushUpdateWorker] Refreshing web push subscriptions");

        await subscriptionManager.refreshWebPushSubscriptions(topics);

        setLastTopics(topics);
      } catch (e) {
        console.error("[useWebPushUpdateWorker] Error refreshing web push subscriptions", e);
      }
    })();
  }, [topics, lastTopics]);
};

const intervalMillis = 13 * 60 * 1_000; // 13 minutes
const updateIntervalMillis = 60 * 60 * 1_000; // 1 hour

class WebPushRefreshWorker {
  constructor() {
    this.timer = null;
    this.lastUpdate = null;
    this.messageHandler = this.onMessage.bind(this);
    this.visibilityHandler = this.onVisibilityChange.bind(this);
  }

  startWorker() {
    if (this.timer !== null) {
      return;
    }

    this.timer = setInterval(() => this.updateSubscriptions(), intervalMillis);

    this.broadcastChannel = new BroadcastChannel("web-push-broadcast");
    this.broadcastChannel.addEventListener("message", this.messageHandler);

    document.addEventListener("visibilitychange", this.visibilityHandler);
  }

  stopWorker() {
    clearTimeout(this.timer);

    this.broadcastChannel.removeEventListener("message", this.messageHandler);
    this.broadcastChannel.close();

    document.removeEventListener("visibilitychange", this.visibilityHandler);
  }

  onMessage() {
    notifier.playSound();
  }

  onVisibilityChange() {
    if (document.visibilityState === "visible") {
      this.updateSubscriptions();
    }
  }

  async updateSubscriptions() {
    if (!notifier.pushPossible()) {
      return;
    }

    if (!this.lastUpdate || Date.now() - this.lastUpdate > updateIntervalMillis) {
      await subscriptionManager.refreshWebPushSubscriptions();
      this.lastUpdate = Date.now();
    }
  }
}

export const webPushRefreshWorker = new WebPushRefreshWorker();
