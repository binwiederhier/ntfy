/* eslint-disable import/no-extraneous-dependencies */
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";
import { NetworkFirst } from "workbox-strategies";

import { getDbAsync } from "../src/app/getDb";
import { formatMessage, formatTitleWithDefault } from "../src/app/notificationUtils";

// See WebPushWorker, this is to play a sound on supported browsers,
// if the app is in the foreground
const broadcastChannel = new BroadcastChannel("web-push-broadcast");

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
  // server/types.go webPushPayload
  const data = event.data.json();
  console.log("[ServiceWorker] Received Web Push Event", { event, data });

  const { subscription_id: subscriptionId, message } = data;
  broadcastChannel.postMessage(message);

  event.waitUntil(
    (async () => {
      const db = await getDbAsync();

      await Promise.all([
        (async () => {
          await db.notifications.add({
            ...message,
            subscriptionId,
            // New marker (used for bubble indicator); cannot be boolean; Dexie index limitation
            new: 1,
          });
          const badgeCount = await db.notifications.where({ new: 1 }).count();
          console.log("[ServiceWorker] Setting new app badge count", { badgeCount });
          self.navigator.setAppBadge?.(badgeCount);
        })(),
        db.subscriptions.update(subscriptionId, {
          last: message.id,
        }),
        self.registration.showNotification(formatTitleWithDefault(message, message.topic), {
          tag: subscriptionId,
          body: formatMessage(message),
          icon: "/static/images/ntfy.png",
          data,
        }),
      ]);
    })()
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();

  const { message } = event.notification.data;

  if (message.click) {
    self.clients.openWindow(message.click);
    return;
  }

  const rootUrl = new URL(self.location.origin);
  const topicUrl = new URL(message.topic, self.location.origin);

  event.waitUntil(
    (async () => {
      const clients = await self.clients.matchAll({ type: "window" });

      const topicClient = clients.find((client) => client.url === topicUrl.toString());
      if (topicClient) {
        topicClient.focus();
        return;
      }

      const rootClient = clients.find((client) => client.url === rootUrl.toString());
      if (rootClient) {
        rootClient.focus();
        return;
      }

      self.clients.openWindow(topicUrl);
    })()
  );
});

// self.__WB_MANIFEST is default injection point
// eslint-disable-next-line no-underscore-dangle
precacheAndRoute(self.__WB_MANIFEST);

// clean old assets
cleanupOutdatedCaches();

// to allow work offline
if (import.meta.env.MODE !== "development") {
  registerRoute(new NavigationRoute(createHandlerBoundToURL("/")));
  registerRoute(({ url }) => url.pathname === "/config.js", new NetworkFirst());
}
