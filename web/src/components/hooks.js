import { useParams } from "react-router-dom";
import { useEffect, useMemo, useState } from "react";
import { useLiveQuery } from "dexie-react-hooks";
import subscriptionManager from "../app/SubscriptionManager";
import { disallowedTopic, expandSecureUrl, isLaunchedPWA, topicUrl } from "../app/utils";
import routes from "./routes";
import connectionManager from "../app/ConnectionManager";
import poller from "../app/Poller";
import pruner from "../app/Pruner";
import session from "../app/Session";
import accountApi from "../app/AccountApi";
import { UnauthorizedError } from "../app/errors";
import notifier from "../app/Notifier";
import prefs from "../app/Prefs";

/**
 * Wire connectionManager and subscriptionManager so that subscriptions are updated when the connection
 * state changes. Conversely, when the subscription changes, the connection is refreshed (which may lead
 * to the connection being re-established).
 *
 * When Web Push is enabled, we do not need to connect to our home server via WebSocket, since notifications
 * will be delivered via Web Push. However, we still need to connect to other servers via WebSocket, or for internal
 * topics, such as sync topics (st_...).
 */
export const useConnectionListeners = (account, subscriptions, users, webPushTopics) => {
  const wsSubscriptions = useMemo(
    () => (subscriptions && webPushTopics ? subscriptions.filter((s) => !webPushTopics.includes(s.topic)) : []),
    // wsSubscriptions should stay stable unless the list of subscription IDs changes. Without the memo, the connection
    // listener calls a refresh for no reason. This isn't a problem due to the makeConnectionId, but it triggers an
    // unnecessary recomputation for every received message.
    [JSON.stringify({ subscriptions: subscriptions?.map(({ id }) => id), webPushTopics })]
  );

  // Register listeners for incoming messages, and connection state changes
  useEffect(
    () => {
      const handleInternalMessage = async (message) => {
        console.log(`[ConnectionListener] Received message on sync topic`, message.message);
        try {
          const data = JSON.parse(message.message);
          if (data.event === "sync") {
            console.log(`[ConnectionListener] Triggering account sync`);
            await accountApi.sync();
          } else {
            console.log(`[ConnectionListener] Unknown message type. Doing nothing.`);
          }
        } catch (e) {
          console.log(`[ConnectionListener] Error parsing sync topic message`, e);
        }
      };

      const handleNotification = async (subscriptionId, notification) => {
        const added = await subscriptionManager.addNotification(subscriptionId, notification);
        if (added) {
          await subscriptionManager.notify(subscriptionId, notification);
        }
      };

      const handleMessage = async (subscriptionId, message) => {
        const subscription = await subscriptionManager.get(subscriptionId);

        // Race condition: sometimes the subscription is already unsubscribed from account
        // sync before the message is handled
        if (!subscription) {
          return;
        }

        if (subscription.internal) {
          await handleInternalMessage(message);
        } else {
          await handleNotification(subscriptionId, message);
        }
      };

      connectionManager.registerStateListener((id, state) => subscriptionManager.updateState(id, state));
      connectionManager.registerMessageListener(handleMessage);

      return () => {
        connectionManager.resetStateListener();
        connectionManager.resetMessageListener();
      };
    },
    // We have to disable dep checking for "navigate". This is fine, it never changes.

    []
  );

  // Sync topic listener: For accounts with sync_topic, subscribe to an internal topic
  useEffect(() => {
    if (!account || !account.sync_topic) {
      return;
    }
    subscriptionManager.add(config.base_url, account.sync_topic, { internal: true }); // Dangle!
  }, [account]);

  // When subscriptions or users change, refresh the connections
  useEffect(() => {
    connectionManager.refresh(wsSubscriptions, users); // Dangle
  }, [wsSubscriptions, users]);
};

/**
 * Automatically adds a subscription if we navigate to a page that has not been subscribed to.
 * This will only be run once after the initial page load.
 */
export const useAutoSubscribe = (subscriptions, selected) => {
  const [hasRun, setHasRun] = useState(false);
  const params = useParams();

  useEffect(() => {
    const loaded = subscriptions !== null && subscriptions !== undefined;
    if (!loaded || hasRun) {
      return;
    }
    setHasRun(true);
    const eligible = params.topic && !selected && !disallowedTopic(params.topic);
    if (eligible) {
      const baseUrl = params.baseUrl ? expandSecureUrl(params.baseUrl) : config.base_url;
      console.log(`[Hooks] Auto-subscribing to ${topicUrl(baseUrl, params.topic)}`);
      (async () => {
        const subscription = await subscriptionManager.add(baseUrl, params.topic);
        if (session.exists()) {
          try {
            await accountApi.addSubscription(baseUrl, params.topic);
          } catch (e) {
            console.log(`[Hooks] Auto-subscribing failed`, e);
            if (e instanceof UnauthorizedError) {
              await session.resetAndRedirect(routes.login);
            }
          }
        }
        poller.pollInBackground(subscription); // Dangle!
      })();
    }
  }, [params, subscriptions, selected, hasRun]);
};

const webPushBroadcastChannel = new BroadcastChannel("web-push-broadcast");

/**
 * Updates the Web Push subscriptions when the list of topics changes,
 * as well as plays a sound when a new broadcast message is received from
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
        console.log("[useWebPushListener] Refreshing web push subscriptions", topics);
        await subscriptionManager.updateWebPushSubscriptions(topics);
        setLastTopics(topics);
      } catch (e) {
        console.error("[useWebPushListener] Error refreshing web push subscriptions", e);
      }
    })();
  }, [topics, lastTopics]);

  useEffect(() => {
    const onMessage = () => {
      notifier.playSound(); // Service Worker cannot play sound, so we do it here!
    };

    webPushBroadcastChannel.addEventListener("message", onMessage);

    return () => {
      webPushBroadcastChannel.removeEventListener("message", onMessage);
    };
  });
};

/**
 * Hook to return a list of Web Push enabled topics using a live query. This hook will return an empty list if
 * permissions are not granted, or if the browser does not support Web Push. Notification permissions are acted upon
 * automatically.
 */
export const useWebPushTopics = () => {
  const [pushPossible, setPushPossible] = useState(notifier.pushPossible());

  useEffect(() => {
    const handler = () => {
      const newPushPossible = notifier.pushPossible();
      console.log(`[useWebPushTopics] Notification Permission changed`, { pushPossible: newPushPossible });
      setPushPossible(newPushPossible);
    };

    if ("permissions" in navigator) {
      navigator.permissions.query({ name: "notifications" }).then((permission) => {
        permission.addEventListener("change", handler);

        return () => {
          permission.removeEventListener("change", handler);
        };
      });
    }
  });

  const topics = useLiveQuery(
    async () => subscriptionManager.webPushTopics(pushPossible),
    // invalidate (reload) query when these values change
    [pushPossible]
  );

  useWebPushListener(topics);

  return topics;
};

/**
 * Watches the "display-mode" to detect if the app is running as a standalone app (PWA),
 * and enables "Web Push" if it is.
 */
export const useStandaloneWebPushAutoSubscribe = () => {
  const matchMedia = window.matchMedia("(display-mode: standalone)");
  const [isStandalone, setIsStandalone] = useState(isLaunchedPWA());

  useEffect(() => {
    const handler = (evt) => {
      console.log(`[useStandaloneAutoWebPushSubscribe] App is now running ${evt.matches ? "standalone" : "in the browser"}`);
      setIsStandalone(evt.matches);
    };

    matchMedia.addEventListener("change", handler);

    return () => {
      matchMedia.removeEventListener("change", handler);
    };
  });

  useEffect(() => {
    if (isStandalone) {
      console.log(`[useStandaloneAutoWebPushSubscribe] Turning on web push automatically`);
      prefs.setWebPushEnabled(true); // Dangle!
    }
  }, [isStandalone]);
};

/**
 * Start the poller and the pruner. This is done in a side effect as opposed to just in Pruner.js
 * and Poller.js, because side effect imports are not a thing in JS, and "Optimize imports" cleans
 * up "unused" imports. See https://github.com/binwiederhier/ntfy/issues/186.
 */

const startWorkers = () => {
  poller.startWorker();
  pruner.startWorker();
  accountApi.startWorker();
};

const stopWorkers = () => {
  poller.stopWorker();
  pruner.stopWorker();
  accountApi.stopWorker();
};

export const useBackgroundProcesses = () => {
  useStandaloneWebPushAutoSubscribe();

  useEffect(() => {
    console.log("[useBackgroundProcesses] mounting");
    startWorkers();

    return () => {
      console.log("[useBackgroundProcesses] unloading");
      stopWorkers();
    };
  }, []);
};

export const useAccountListener = (setAccount) => {
  useEffect(() => {
    accountApi.registerListener(setAccount);
    accountApi.sync(); // Dangle
    return () => {
      accountApi.resetListener();
    };
  }, []);
};
