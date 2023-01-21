import config from "../app/config";
import {shortUrl} from "../app/utils";

// Remember to also update the "disallowedTopics" list!

const routes = {
    login: "/login",
    signup: "/signup",
    resetPassword: "/reset-password", // Not used (yet)
    app: config.app_root,
    account: "/account",
    settings: "/settings",
    subscription: "/:topic",
    subscriptionExternal: "/:baseUrl/:topic",
    forSubscription: (subscription) => {
        if (subscription.baseUrl !== config.base_url) {
            return `/${shortUrl(subscription.baseUrl)}/${subscription.topic}`;
        }
        return `/${subscription.topic}`;
    }
};

export default routes;
