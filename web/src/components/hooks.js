import {useNavigate, useParams} from "react-router-dom";
import {useEffect, useState} from "react";
import subscriptionManager from "../app/SubscriptionManager";
import {disallowedTopic, expandSecureUrl, topicUrl} from "../app/utils";
import notifier from "../app/Notifier";
import routes from "./routes";
import connectionManager from "../app/ConnectionManager";
import poller from "../app/Poller";

export const useConnectionListeners = () => {
  const navigate = useNavigate();
  useEffect(() => {
        const handleNotification = async (subscriptionId, notification) => {
          const added = await subscriptionManager.addNotification(subscriptionId, notification);
          if (added) {
            const defaultClickAction = (subscription) => navigate(routes.forSubscription(subscription));
            await notifier.notify(subscriptionId, notification, defaultClickAction)
          }
        };
        connectionManager.registerStateListener(subscriptionManager.updateState);
        connectionManager.registerNotificationListener(handleNotification);
        return () => {
          connectionManager.resetStateListener();
          connectionManager.resetNotificationListener();
        }
      },
      // We have to disable dep checking for "navigate". This is fine, it never changes.
      // eslint-disable-next-line
      []);
};

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
      const baseUrl = (params.baseUrl) ? expandSecureUrl(params.baseUrl) : window.location.origin;
      console.log(`[App] Auto-subscribing to ${topicUrl(baseUrl, params.topic)}`);
      (async () => {
        const subscription = await subscriptionManager.add(baseUrl, params.topic);
        poller.pollInBackground(subscription); // Dangle!
      })();
    }
  }, [params, subscriptions, selected, hasRun]);
};
