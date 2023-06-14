/* eslint-disable import/no-extraneous-dependencies */
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";
import { NetworkFirst } from "workbox-strategies";

import { dbAsync } from "../src/app/db";
import { formatMessage, formatTitleWithDefault } from "../src/app/notificationUtils";

import i18n from "../src/app/i18n";

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

const isImage = (filenameOrUrl) => filenameOrUrl?.match(/\.(png|jpe?g|gif|webp)$/i) ?? false;

const icon = "/static/images/ntfy.png";

const addNotification = async (data) => {
  const db = await dbAsync();

  const { subscription_id: subscriptionId, message } = data;

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

const showNotification = async (data) => {
  const { subscription_id: subscriptionId, message } = data;

  // Please update the desktop notification in Notifier.js to match any changes here
  const image = isImage(message.attachment?.name) ? message.attachment.url : undefined;
  await self.registration.showNotification(formatTitleWithDefault(message, message.topic), {
    tag: subscriptionId,
    body: formatMessage(message),
    icon: image ?? icon,
    image,
    data,
    timestamp: message.time * 1_000,
    actions: message.actions
      ?.filter(({ action }) => action === "view" || action === "http")
      .map(({ label }) => ({
        action: label,
        title: label,
      })),
  });
};

/**
 * Handle a received web push notification
 * @param {object} data see server/types.go, type webPushPayload
 */
const handlePush = async (data) => {
  if (data.event === "subscription_expiring") {
    await self.registration.showNotification(i18n.t("web_push_subscription_expiring_title"), {
      body: i18n.t("web_push_subscription_expiring_body"),
      icon,
      data,
    });
  } else if (data.event === "message") {
    // see: web/src/app/WebPush.js
    // the service worker cannot play a sound, so if the web app
    // is running, it receives the broadcast and plays it.
    broadcastChannel.postMessage(data.message);

    await addNotification(data);
    await showNotification(data);
  } else {
    // We can't ignore the push, since permission can be revoked by the browser
    await self.registration.showNotification(i18n.t("web_push_unknown_notification_title"), {
      body: i18n.t("web_push_unknown_notification_body"),
      icon,
      data,
    });
  }
};

/**
 * Handle a user clicking on the displayed notification from `showNotification`
 * This is also called when the user clicks on an action button
 */
const handleClick = async (event) => {
  const clients = await self.clients.matchAll({ type: "window" });

  const rootUrl = new URL(self.location.origin);
  const rootClient = clients.find((client) => client.url === rootUrl.toString());

  if (event.notification.data?.event !== "message") {
    // e.g. subscription_expiring event, simply open the web app on the root route (/)
    if (rootClient) {
      rootClient.focus();
    } else {
      self.clients.openWindow(rootUrl);
    }
    event.notification.close();
  } else {
    const { message } = event.notification.data;

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
          self.registration.showNotification(`${i18n.t('notifications_actions_failed_notification')}: ${action.label} (${action.action})`, {
            body: e.message,
            icon,
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
      // - if not, an open tab on the root route (`/`)
      // - if no ntfy window is open, open a new tab on the `/:topic` route

      const topicUrl = new URL(message.topic, self.location.origin);
      const topicClient = clients.find((client) => client.url === topicUrl.toString());

      if (topicClient) {
        topicClient.focus();
      } else if (rootClient) {
        rootClient.focus();
      } else {
        self.clients.openWindow(topicUrl);
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

// see https://vite-pwa-org.netlify.app/guide/inject-manifest.html#service-worker-code
// self.__WB_MANIFEST is the workbox injection point that injects the manifest of the
// vite dist files and their revision ids, for example:
// [{"revision":"aaabbbcccdddeeefff12345","url":"/index.html"},...]
precacheAndRoute(
  // eslint-disable-next-line no-underscore-dangle
  self.__WB_MANIFEST
);

// delete any cached old dist files from previous service worker versions
cleanupOutdatedCaches();

if (import.meta.env.MODE !== "development") {
  // since the manifest only includes `/index.html`, this manually adds the root route `/`
  registerRoute(new NavigationRoute(createHandlerBoundToURL("/")));
  // the manifest excludes config.js (see vite.config.js) since the dist-file differs from the
  // actual config served by the go server. this adds it back with `NetworkFirst`, so that the
  // most recent config from the go server is cached, but the app still works if the network
  // is unavailable. this is important since there's no "refresh" button in the installed pwa
  // to force a reload.
  registerRoute(({ url }) => url.pathname === "/config.js", new NetworkFirst());
}
