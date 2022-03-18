import {useNavigate, useParams} from "react-router-dom";
import {useEffect, useState} from "react";
import subscriptionManager from "../app/SubscriptionManager";
import {disallowedTopic, expandSecureUrl, topicUrl} from "../app/utils";
import notifier from "../app/Notifier";
import routes from "./routes";
import connectionManager from "../app/ConnectionManager";
import poller from "../app/Poller";

/**
 * Wire connectionManager and subscriptionManager so that subscriptions are updated when the connection
 * state changes. Conversely, when the subscription changes, the connection is refreshed (which may lead
 * to the connection being re-established).
 */
export const useConnectionListeners = (subscriptions, users) => {
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
        []
    );

    useEffect(() => {
        connectionManager.refresh(subscriptions, users); // Dangle
    }, [subscriptions, users]);
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
            const baseUrl = (params.baseUrl) ? expandSecureUrl(params.baseUrl) : window.location.origin;
            console.log(`[App] Auto-subscribing to ${topicUrl(baseUrl, params.topic)}`);
            (async () => {
                const subscription = await subscriptionManager.add(baseUrl, params.topic);
                poller.pollInBackground(subscription); // Dangle!
            })();
        }
    }, [params, subscriptions, selected, hasRun]);
};

/**
 * Migrate the 'topics' item in localStorage to the subscriptionManager. This is only done once to migrate away
 * from the old web UI.
 */
export const useLocalStorageMigration = () => {
    const [hasRun, setHasRun] = useState(false);
    useEffect(() => {
        if (hasRun) {
            return;
        }
        const topicsStr = localStorage.getItem("topics");
        if (topicsStr) {
            const topics = JSON.parse(topicsStr).filter(topic => topic !== "");
            if (topics.length > 0) {
                (async () => {
                    for (const topic of topics) {
                        const baseUrl = window.location.origin;
                        const subscription = await subscriptionManager.add(baseUrl, topic);
                        poller.pollInBackground(subscription); // Dangle!
                    }
                    localStorage.removeItem("topics");
                })();
            }
        }
        setHasRun(true);
    }, []);
}
