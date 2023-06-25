import { useState, useEffect } from "react";
import notifier from "./Notifier";
import subscriptionManager from "./SubscriptionManager";

const broadcastChannel = new BroadcastChannel("web-push-broadcast");

/**
 * Updates the Web Push subscriptions when the list of topics changes,
 * as well as plays a sound when a new broadcat message is received from
 * the service worker, since the service worker cannot play sounds.
 */
const useWebPushListener = (topics) => {
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

  useEffect(() => {
    const onMessage = () => {
      notifier.playSound(); // Service Worker cannot play sound, so we do it here!
    };

    broadcastChannel.addEventListener("message", onMessage);

    return () => {
      broadcastChannel.removeEventListener("message", onMessage);
    };
  });
};

export default useWebPushListener;
