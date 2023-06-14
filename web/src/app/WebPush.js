import { useState, useEffect } from "react";
import { useLiveQuery } from "dexie-react-hooks";
import notifier from "./Notifier";
import subscriptionManager from "./SubscriptionManager";

const intervalMillis = 13 * 60 * 1_000; // 13 minutes
const updateIntervalMillis = 60 * 60 * 1_000; // 1 hour

/**
 * Updates the Web Push subscriptions when the list of topics changes.
 */
export const useWebPushTopicListener = () => {
  const topics = useLiveQuery(() => subscriptionManager.webPushTopics());
  const [lastTopics, setLastTopics] = useState();

  useEffect(() => {
    const topicsChanged = JSON.stringify(topics) !== JSON.stringify(lastTopics);
    if (!notifier.pushPossible() || !topicsChanged) {
      return;
    }

    (async () => {
      try {
        console.log("[useWebPushTopicListener] Refreshing web push subscriptions", topics);
        await subscriptionManager.updateWebPushSubscriptions(topics);
        setLastTopics(topics);
      } catch (e) {
        console.error("[useWebPushTopicListener] Error refreshing web push subscriptions", e);
      }
    })();
  }, [topics, lastTopics]);
};

/**
 * Helper class for Web Push that does three things:
 * 1. Updates the Web Push subscriptions on a schedule
 * 2. Updates the Web Push subscriptions when the window is minimised / app switched
 * 3. Listens to the broadcast channel from the service worker to play a sound when a message comes in
 */
class WebPushWorker {
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
    notifier.playSound(); // Service Worker cannot play sound, so we do it here!
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
      await subscriptionManager.updateWebPushSubscriptions();
      this.lastUpdate = Date.now();
    }
  }
}

export const webPush = new WebPushWorker();
