/* eslint-disable import/no-extraneous-dependencies */
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";
import { NetworkFirst } from "workbox-strategies";

import { dbAsync } from "../src/app/db";
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

  event.waitUntil(
    (async () => {
      if (data.event === "subscription_expiring") {
        await self.registration.showNotification("Notifications will be paused", {
          body: "Open ntfy to continue receiving notifications",
          icon: "/static/images/ntfy.png",
          data,
        });
      } else if (data.event === "message") {
        const { subscription_id: subscriptionId, message } = data;
        broadcastChannel.postMessage(message);

        const db = await dbAsync();
        const image = message.attachment?.name.match(/\.(png|jpe?g|gif|webp)$/i) ? message.attachment.url : undefined;

        const actions = message.actions
          ?.filter(({ action }) => action === "view" || action === "http")
          .map(({ label }) => ({
            action: label,
            title: label,
          }));

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
          // Please update the desktop notification in Notifier.js to match any changes
          self.registration.showNotification(formatTitleWithDefault(message, message.topic), {
            tag: subscriptionId,
            body: formatMessage(message),
            icon: image ?? "/static/images/ntfy.png",
            image,
            data,
            timestamp: message.time * 1_000,
            actions,
          }),
        ]);
      } else {
        // We can't ignore the push, since permission can be revoked by the browser
        await self.registration.showNotification("Unknown notification received from server", {
          body: "You may need to update ntfy by opening the web app",
          icon: "/static/images/ntfy.png",
          data,
        });
      }
    })()
  );
});

self.addEventListener("notificationclick", (event) => {
  console.log("[ServiceWorker] NotificationClick");

  event.waitUntil(
    (async () => {
      const clients = await self.clients.matchAll({ type: "window" });

      const rootUrl = new URL(self.location.origin);
      const rootClient = clients.find((client) => client.url === rootUrl.toString());

      if (event.notification.data?.event !== "message") {
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

            if (action.clear) {
              event.notification.close();
            }
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

              if (action.clear) {
                event.notification.close();
              }
            } catch (e) {
              console.error("[ServiceWorker] Error performing http action", e);
              self.registration.showNotification(`Unsuccessful action ${action.label} (${action.action})`, {
                body: e.message,
              });
            }
          }
        } else if (message.click) {
          self.clients.openWindow(message.click);

          event.notification.close();
        } else {
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
