import config from "../app/config";
import {shortUrl} from "../app/utils";

const root = RootPath; // Defined in `public/index.html`.
const routes = {
    root: root,
    settings: root+"settings",
    subscription: root+":topic",
    subscriptionExternal: root+":baseUrl/:topic",
    forSubscription: (subscription) => {
        if (subscription.baseUrl !== window.location.origin) {
            return root+`${shortUrl(subscription.baseUrl)}/${subscription.topic}`;
        }
        return root+`${subscription.topic}`;
    }
};

export default routes;
