import config from "../app/config";
import { shortUrl } from "../app/utils";

const routes = {
  app: config.app_root,
  login: "/login",
  signup: "/signup",
  account: "/account",
  settings: "/settings",
  passwordResetRequest: "/reset-password",
  passwordReset: "/account/password/reset/:token",
  emailVerify: "/account/email/verify/:token",
  subscription: "/:topic",
  subscriptionExternal: "/:baseUrl/:topic",
  forSubscription: (subscription) => {
    if (subscription.baseUrl !== config.base_url) {
      // Encode the base URL so a path in it (https://host/path) stays one route segment
      return `/${encodeURIComponent(shortUrl(subscription.baseUrl))}/${subscription.topic}`;
    }
    return `/${subscription.topic}`;
  },
};

export default routes;
