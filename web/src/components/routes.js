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
  tokenRedeem: "/account/token/redeem",
  subscription: "/:topic",
  subscriptionExternal: "/:baseUrl/:topic",
  forSubscription: (subscription) => {
    if (subscription.baseUrl !== config.base_url) {
      return `/${shortUrl(subscription.baseUrl)}/${subscription.topic}`;
    }
    return `/${subscription.topic}`;
  },
};

export default routes;
