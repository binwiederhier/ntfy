import notifier from "./Notifier";
import prefs from "./Prefs";
import getDb from "./getDb";
import { topicUrl } from "./utils";

/** @typedef {string} NotificationTypeEnum */

/** @enum {NotificationTypeEnum} */
export const NotificationType = {
  /** sound-only */
  SOUND: "sound",
  /** browser notifications when there is an active tab, via websockets */
  BROWSER: "browser",
  /** web push notifications, regardless of whether the window is open */
  BACKGROUND: "background",
};

class SubscriptionManager {
  constructor(db) {
    this.db = db;
  }

  /** All subscriptions, including "new count"; this is a JOIN, see https://dexie.org/docs/API-Reference#joining */
  async all() {
    const subscriptions = await this.db.subscriptions.toArray();
    return Promise.all(
      subscriptions.map(async (s) => ({
        ...s,
        new: await this.db.notifications.where({ subscriptionId: s.id, new: 1 }).count(),
      }))
    );
  }

  async get(subscriptionId) {
    return this.db.subscriptions.get(subscriptionId);
  }

  async notify(subscriptionId, notification, defaultClickAction) {
    const subscription = await this.get(subscriptionId);

    if (subscription.mutedUntil === 1) {
      return;
    }

    const priority = notification.priority ?? 3;
    if (priority < (await prefs.minPriority())) {
      return;
    }

    await notifier.playSound();

    // sound only
    if (subscription.notificationType === "sound") {
      return;
    }

    await notifier.notify(subscription, notification, defaultClickAction);
  }

  /**
   * @param {string} baseUrl
   * @param {string} topic
   * @param {object} opts
   * @param {boolean} opts.internal
   * @param {NotificationTypeEnum} opts.notificationType
   * @returns
   */
  async add(baseUrl, topic, opts = {}) {
    const id = topicUrl(baseUrl, topic);

    const webPushFields = opts.notificationType === "background" ? await notifier.subscribeWebPush(baseUrl, topic) : {};

    const existingSubscription = await this.get(id);
    if (existingSubscription) {
      if (webPushFields.endpoint) {
        await this.db.subscriptions.update(existingSubscription.id, {
          webPushEndpoint: webPushFields.endpoint,
        });
      }

      return existingSubscription;
    }

    const subscription = {
      id: topicUrl(baseUrl, topic),
      baseUrl,
      topic,
      mutedUntil: 0,
      last: null,
      ...opts,
      webPushEndpoint: webPushFields.endpoint,
    };

    await this.db.subscriptions.put(subscription);

    return subscription;
  }

  async syncFromRemote(remoteSubscriptions, remoteReservations) {
    console.log(`[SubscriptionManager] Syncing subscriptions from remote`, remoteSubscriptions);

    const notificationType = (await prefs.webPushDefaultEnabled()) === "enabled" ? "background" : "browser";

    // Add remote subscriptions
    const remoteIds = await Promise.all(
      remoteSubscriptions.map(async (remote) => {
        const local = await this.add(remote.base_url, remote.topic, {
          notificationType,
        });
        const reservation = remoteReservations?.find((r) => remote.base_url === config.base_url && remote.topic === r.topic) || null;

        await this.update(local.id, {
          displayName: remote.display_name, // May be undefined
          reservation, // May be null!
        });

        return local.id;
      })
    );

    // Remove local subscriptions that do not exist remotely
    const localSubscriptions = await this.db.subscriptions.toArray();

    await Promise.all(
      localSubscriptions.map(async (local) => {
        const remoteExists = remoteIds.includes(local.id);
        if (!local.internal && !remoteExists) {
          await this.remove(local);
        }
      })
    );
  }

  async updateState(subscriptionId, state) {
    this.db.subscriptions.update(subscriptionId, { state });
  }

  async remove(subscription) {
    await this.db.subscriptions.delete(subscription.id);
    await this.db.notifications.where({ subscriptionId: subscription.id }).delete();

    if (subscription.webPushEndpoint) {
      await notifier.unsubscribeWebPush(subscription);
    }
  }

  async first() {
    return this.db.subscriptions.toCollection().first(); // May be undefined
  }

  async getNotifications(subscriptionId) {
    // This is quite awkward, but it is the recommended approach as per the Dexie docs.
    // It's actually fine, because the reading and filtering is quite fast. The rendering is what's
    // killing performance. See  https://dexie.org/docs/Collection/Collection.offset()#a-better-paging-approach

    return this.db.notifications
      .orderBy("time") // Sort by time first
      .filter((n) => n.subscriptionId === subscriptionId)
      .reverse()
      .toArray();
  }

  async getAllNotifications() {
    return this.db.notifications
      .orderBy("time") // Efficient, see docs
      .reverse()
      .toArray();
  }

  /** Adds notification, or returns false if it already exists */
  async addNotification(subscriptionId, notification) {
    const exists = await this.db.notifications.get(notification.id);
    if (exists) {
      return false;
    }
    try {
      // sw.js duplicates this logic, so if you change it here, change it there too
      await this.db.notifications.add({
        ...notification,
        subscriptionId,
        // New marker (used for bubble indicator); cannot be boolean; Dexie index limitation
        new: 1,
      }); // FIXME consider put() for double tab
      await this.db.subscriptions.update(subscriptionId, {
        last: notification.id,
      });
    } catch (e) {
      console.error(`[SubscriptionManager] Error adding notification`, e);
    }
    return true;
  }

  /** Adds/replaces notifications, will not throw if they exist */
  async addNotifications(subscriptionId, notifications) {
    const notificationsWithSubscriptionId = notifications.map((notification) => ({ ...notification, subscriptionId }));
    const lastNotificationId = notifications.at(-1).id;
    await this.db.notifications.bulkPut(notificationsWithSubscriptionId);
    await this.db.subscriptions.update(subscriptionId, {
      last: lastNotificationId,
    });
  }

  async updateNotification(notification) {
    const exists = await this.db.notifications.get(notification.id);
    if (!exists) {
      return false;
    }
    try {
      await this.db.notifications.put({ ...notification });
    } catch (e) {
      console.error(`[SubscriptionManager] Error updating notification`, e);
    }
    return true;
  }

  async deleteNotification(notificationId) {
    await this.db.notifications.delete(notificationId);
  }

  async deleteNotifications(subscriptionId) {
    await this.db.notifications.where({ subscriptionId }).delete();
  }

  async markNotificationRead(notificationId) {
    await this.db.notifications.where({ id: notificationId }).modify({ new: 0 });
  }

  async markNotificationsRead(subscriptionId) {
    await this.db.notifications.where({ subscriptionId, new: 1 }).modify({ new: 0 });
  }

  async setMutedUntil(subscriptionId, mutedUntil) {
    await this.db.subscriptions.update(subscriptionId, {
      mutedUntil,
    });

    const subscription = await this.get(subscriptionId);

    if (subscription.notificationType === "background") {
      if (mutedUntil === 1) {
        await notifier.unsubscribeWebPush(subscription);
      } else {
        const webPushFields = await notifier.subscribeWebPush(subscription.baseUrl, subscription.topic);
        await this.db.subscriptions.update(subscriptionId, {
          webPushEndpoint: webPushFields.endpoint,
        });
      }
    }
  }

  /**
   *
   * @param {object} subscription
   * @param {NotificationTypeEnum} newNotificationType
   * @returns
   */
  async setNotificationType(subscription, newNotificationType) {
    const oldNotificationType = subscription.notificationType ?? "browser";

    if (oldNotificationType === newNotificationType) {
      return;
    }

    let { webPushEndpoint } = subscription;

    if (oldNotificationType === "background") {
      await notifier.unsubscribeWebPush(subscription);
      webPushEndpoint = undefined;
    } else if (newNotificationType === "background") {
      const webPushFields = await notifier.subscribeWebPush(subscription.baseUrl, subscription.topic);
      webPushEndpoint = webPushFields.webPushEndpoint;
    }

    await this.db.subscriptions.update(subscription.id, {
      notificationType: newNotificationType,
      webPushEndpoint,
    });
  }

  // for logout/delete, unsubscribe first to prevent receiving dangling notifications
  async unsubscribeAllWebPush() {
    const subscriptions = await this.db.subscriptions.where({ notificationType: "background" }).toArray();
    await Promise.all(subscriptions.map((subscription) => notifier.unsubscribeWebPush(subscription)));
  }

  async refreshWebPushSubscriptions() {
    const subscriptions = await this.db.subscriptions.where({ notificationType: "background" }).toArray();
    const browserSubscription = await (await navigator.serviceWorker.getRegistration())?.pushManager?.getSubscription();

    if (browserSubscription) {
      await Promise.all(subscriptions.map((subscription) => notifier.subscribeWebPush(subscription.baseUrl, subscription.topic)));
    } else {
      await Promise.all(subscriptions.map((subscription) => this.setNotificationType(subscription, "sound")));
    }
  }

  async setDisplayName(subscriptionId, displayName) {
    await this.db.subscriptions.update(subscriptionId, {
      displayName,
    });
  }

  async setReservation(subscriptionId, reservation) {
    await this.db.subscriptions.update(subscriptionId, {
      reservation,
    });
  }

  async update(subscriptionId, params) {
    await this.db.subscriptions.update(subscriptionId, params);
  }

  async pruneNotifications(thresholdTimestamp) {
    await this.db.notifications.where("time").below(thresholdTimestamp).delete();
  }
}

export default new SubscriptionManager(getDb());
