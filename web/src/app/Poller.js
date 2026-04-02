import api from "./Api";
import prefs from "./Prefs";
import subscriptionManager from "./SubscriptionManager";
import { EVENT_MESSAGE, EVENT_MESSAGE_DELETE } from "./events";

const delayMillis = 2000; // 2 seconds
const intervalMillis = 300000; // 5 minutes

class Poller {
  constructor() {
    this.timer = null;
  }

  startWorker() {
    if (this.timer !== null) {
      return;
    }
    console.log(`[Poller] Starting worker`);
    this.timer = setInterval(() => this.pollAll(), intervalMillis);
    setTimeout(() => this.pollAll(), delayMillis);
  }

  stopWorker() {
    clearTimeout(this.timer);
  }

  async pollAll() {
    console.log(`[Poller] Polling all subscriptions`);
    const subscriptions = await subscriptionManager.all();

    await Promise.all(
      subscriptions.map(async (s) => {
        try {
          await this.poll(s);
        } catch (e) {
          console.log(`[Poller] Error polling ${s.id}`, e);
        }
      })
    );
  }

  async poll(subscription) {
    console.log(`[Poller] Polling ${subscription.id}`);

    const since = subscription.last;
    const notifications = await api.poll(subscription.baseUrl, subscription.topic, since);

    // Filter out notifications older than the prune threshold
    const deleteAfterSeconds = await prefs.deleteAfter();
    const pruneThresholdTimestamp = deleteAfterSeconds > 0 ? Math.round(Date.now() / 1000) - deleteAfterSeconds : 0;
    const recentNotifications =
      pruneThresholdTimestamp > 0 ? notifications.filter((n) => n.time >= pruneThresholdTimestamp) : notifications;

    // Find the latest notification for each sequence ID
    const latestBySequenceId = this.latestNotificationsBySequenceId(recentNotifications);

    // Delete all existing notifications for which the latest notification is marked as deleted
    const deletedSequenceIds = Object.entries(latestBySequenceId)
      .filter(([, notification]) => notification.event === EVENT_MESSAGE_DELETE)
      .map(([sequenceId]) => sequenceId);
    if (deletedSequenceIds.length > 0) {
      console.log(`[Poller] Deleting notifications with deleted sequence IDs for ${subscription.id}`, deletedSequenceIds);
      await Promise.all(
        deletedSequenceIds.map((sequenceId) => subscriptionManager.deleteNotificationBySequenceId(subscription.id, sequenceId))
      );
    }

    // Add only the latest notification for each non-deleted sequence
    const notificationsToAdd = Object.values(latestBySequenceId).filter((n) => n.event === EVENT_MESSAGE);
    if (notificationsToAdd.length > 0) {
      console.log(`[Poller] Adding ${notificationsToAdd.length} notification(s) for ${subscription.id}`);
      await subscriptionManager.addNotifications(subscription.id, notificationsToAdd);
    } else {
      console.log(`[Poller] No new notifications found for ${subscription.id}`);
    }
  }

  pollInBackground(subscription) {
    (async () => {
      try {
        await this.poll(subscription);
      } catch (e) {
        console.error(`[App] Error polling subscription ${subscription.id}`, e);
      }
    })();
  }

  /**
   * Groups notifications by sequenceId and returns only the latest (highest time) for each sequence.
   * Returns an object mapping sequenceId -> latest notification.
   */
  latestNotificationsBySequenceId(notifications) {
    const latestBySequenceId = {};
    notifications.forEach((notification) => {
      const sequenceId = notification.sequence_id || notification.id;
      if (!(sequenceId in latestBySequenceId) || notification.time >= latestBySequenceId[sequenceId].time) {
        latestBySequenceId[sequenceId] = notification;
      }
    });
    return latestBySequenceId;
  }
}

const poller = new Poller();
export default poller;
