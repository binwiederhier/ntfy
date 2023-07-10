/* eslint-disable import/no-extraneous-dependencies */
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";
import { NetworkFirst } from "workbox-strategies";
import { clientsClaim } from "workbox-core";

import { dbAsync } from "../src/app/db";

import { toNotificationParams, icon, badge } from "../src/app/notificationUtils";
import initI18n from "../src/app/i18n";

/**
 * General docs for service workers and PWAs:
 * https://vite-pwa-org.netlify.app/guide/
 * https://developer.chrome.com/docs/workbox/
 *
 * This file uses the (event) => event.waitUntil(<promise>) pattern.
 * This is because the event handler itself cannot be async, but
 * the service worker needs to stay active while the promise completes.
 */

const broadcastChannel = new BroadcastChannel("web-push-broadcast");

const addNotification = async ({ subscriptionId, message }) => {
  const db = await dbAsync();

  await db.notifications.add({
    ...message,
    subscriptionId,
    // New marker (used for bubble indicator); cannot be boolean; Dexie index limitation
    new: 1,
  });

  await db.subscriptions.update(subscriptionId, {
    last: message.id,
  });

  const badgeCount = await db.notifications.where({ new: 1 }).count();
  console.log("[ServiceWorker] Setting new app badge count", { badgeCount });
  self.navigator.setAppBadge?.(badgeCount);
};

/**
 * Handle a received web push message and show notification.
 *
 * Since the service worker cannot play a sound, we send a broadcast to the web app, which (if it is running)
 * receives the broadcast and plays a sound (see web/src/app/WebPush.js).
 */
const handlePushMessage = async (data) => {
  const { subscription_id: subscriptionId, message } = data;

  broadcastChannel.postMessage(message); // To potentially play sound

  await addNotification({ subscriptionId, message });
  await self.registration.showNotification(
    ...toNotificationParams({
      subscriptionId,
      message,
      defaultTitle: message.topic,
      topicRoute: new URL(message.topic, self.location.origin).toString(),
    })
  );
};

/**
 * Handle a received web push subscription expiring.
 */
const handlePushSubscriptionExpiring = async (data) => {
  const t = await initI18n();

  await self.registration.showNotification(t("web_push_subscription_expiring_title"), {
    body: t("web_push_subscription_expiring_body"),
    icon,
    data,
    badge,
  });
};

/**
 * Handle unknown push message. We can't ignore the push, since
 * permission can be revoked by the browser.
 */
const handlePushUnknown = async (data) => {
  const t = await initI18n();

  await self.registration.showNotification(t("web_push_unknown_notification_title"), {
    body: t("web_push_unknown_notification_body"),
    icon,
    data,
    badge,
  });
};

/**
 * Handle a received web push notification
 * @param {object} data see server/types.go, type webPushPayload
 */
const handlePush = async (data) => {
  if (data.event === "message") {
    await handlePushMessage(data);
  } else if (data.event === "subscription_expiring") {
    await handlePushSubscriptionExpiring(data);
  } else {
    await handlePushUnknown(data);
  }
};

/**
 * Handle a user clicking on the displayed notification from `showNotification`.
 * This is also called when the user clicks on an action button.
 */
const handleClick = async (event) => {
  const t = await initI18n();

  const clients = await self.clients.matchAll({ type: "window" });

  const rootUrl = new URL(self.location.origin);
  const rootClient = clients.find((client) => client.url === rootUrl.toString());
  // perhaps open on another topic
  const fallbackClient = clients[0];

  if (!event.notification.data?.message) {
    // e.g. something other than a message, e.g. a subscription_expiring event
    // simply open the web app on the root route (/)
    if (rootClient) {
      rootClient.focus();
    } else if (fallbackClient) {
      fallbackClient.focus();
      fallbackClient.navigate(rootUrl.toString());
    } else {
      self.clients.openWindow(rootUrl);
    }
    event.notification.close();
  } else {
    const { message, topicRoute } = event.notification.data;

    if (event.action) {
      const action = event.notification.data.message.actions.find(({ label }) => event.action === label);

      if (action.action === "view") {
        self.clients.openWindow(action.url);
      } else if (action.action === "http") {
        try {
          const response = await fetch(action.url, {
            method: action.method ?? "POST",
            headers: action.headers ?? {},
            body: action.body,
          });

          if (!response.ok) {
            throw new Error(`HTTP ${response.status} ${response.statusText}`);
          }
        } catch (e) {
          console.error("[ServiceWorker] Error performing http action", e);
          self.registration.showNotification(`${t("notifications_actions_failed_notification")}: ${action.label} (${action.action})`, {
            body: e.message,
            icon,
            badge,
          });
        }
      }

      if (action.clear) {
        event.notification.close();
      }
    } else if (message.click) {
      self.clients.openWindow(message.click);

      event.notification.close();
    } else {
      // If no action was clicked, and the message doesn't have a click url:
      // - first try focus an open tab on the `/:topic` route
      // - if not, use an open tab on the root route (`/`) and navigate to the topic
      // - if not, use whichever tab we have open and navigate to the topic
      // - finally, open a new tab focused on the topic

      const topicClient = clients.find((client) => client.url === topicRoute);

      if (topicClient) {
        topicClient.focus();
      } else if (rootClient) {
        rootClient.focus();
        rootClient.navigate(topicRoute);
      } else if (fallbackClient) {
        fallbackClient.focus();
        fallbackClient.navigate(topicRoute);
      } else {
        self.clients.openWindow(topicRoute);
      }

      event.notification.close();
    }
  }
};

self.addEventListener("install", () => {
  console.log("[ServiceWorker] Installed");
  self.skipWaiting();
});

self.addEventListener("activate", () => {
  console.log("[ServiceWorker] Activated");
  self.skipWaiting();
});

// There's no good way to test this, and Chrome doesn't seem to implement this,
// so leaving it for now
self.addEventListener("pushsubscriptionchange", (event) => {
  console.log("[ServiceWorker] PushSubscriptionChange");
  console.log(event);
});

self.addEventListener("push", (event) => {
  const data = event.data.json();
  console.log("[ServiceWorker] Received Web Push Event", { event, data });
  event.waitUntil(handlePush(data));
});

self.addEventListener("notificationclick", (event) => {
  console.log("[ServiceWorker] NotificationClick");
  event.waitUntil(handleClick(event));
});

// See https://vite-pwa-org.netlify.app/guide/inject-manifest.html#service-worker-code
// self.__WB_MANIFEST is the workbox injection point that injects the manifest of the
// vite dist files and their revision ids, for example:
// [{"revision":"aaabbbcccdddeeefff12345","url":"/index.html"},...]
precacheAndRoute(
  // eslint-disable-next-line no-underscore-dangle
  self.__WB_MANIFEST
);

// Claim all open windows
clientsClaim();
// Delete any cached old dist files from previous service worker versions
cleanupOutdatedCaches();

if (!import.meta.env.DEV) {
  // we need the app_root setting, so we import the config.js file from the go server
  // this does NOT include the same base_url as the web app running in a window,
  // since we don't have access to `window` like in `src/app/config.js`
  self.importScripts("/config.js");

  // this is the fallback single-page-app route, matching vite.config.js PWA config,
  // and is served by the go web server. It is needed for the single-page-app to work.
  // https://developer.chrome.com/docs/workbox/modules/workbox-routing/#how-to-register-a-navigation-route
  registerRoute(
    new NavigationRoute(createHandlerBoundToURL("/app.html"), {
      allowlist: [
        // the app root itself, could be /, or not
        new RegExp(`^${config.app_root}$`),
      ],
    })
  );

  // the manifest excludes config.js (see vite.config.js) since the dist-file differs from the
  // actual config served by the go server. this adds it back with `NetworkFirst`, so that the
  // most recent config from the go server is cached, but the app still works if the network
  // is unavailable. this is important since there's no "refresh" button in the installed pwa
  // to force a reload.
  registerRoute(({ url }) => url.pathname === "/config.js", new NetworkFirst());
}
