import config from "../app/config";
import {shortUrl} from "../app/utils";

const routes = {
    home: "/",
    pricing: "/pricing",
    login: "/login",
    signup: "/signup",
    resetPassword: "/reset-password",
    app: config.appRoot,
    account: "/account",
    settings: "/settings",
    subscription: "/:topic",
    subscriptionExternal: "/:baseUrl/:topic",
    forSubscription: (subscription) => {
        if (subscription.baseUrl !== window.location.origin) {
            return `/${shortUrl(subscription.baseUrl)}/${subscription.topic}`;
        }
        return `/${subscription.topic}`;
    }
};

export default routes;
