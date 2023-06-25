import { playSound, topicDisplayName, topicShortUrl, urlB64ToUint8Array } from "./utils";
import { toNotificationParams } from "./notificationUtils";
import prefs from "./Prefs";
import routes from "../components/routes";

/**
 * The notifier is responsible for displaying desktop notifications. Note that not all modern browsers
 * support this; most importantly, all iOS browsers do not support window.Notification.
 */
class Notifier {
  async notify(subscription, notification) {
    if (!this.supported()) {
      return;
    }

    await this.playSound();

    const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
    const defaultTitle = topicDisplayName(subscription);

    console.log(`[Notifier, ${shortUrl}] Displaying notification ${notification.id}`);

    const registration = await this.serviceWorkerRegistration();
    await registration.showNotification(
      ...toNotificationParams({
        subscriptionId: subscription.id,
        message: notification,
        defaultTitle,
        topicRoute: new URL(routes.forSubscription(subscription), window.location.origin).toString(),
      })
    );
  }

  async playSound() {
    // Play sound
    const sound = await prefs.sound();
    if (sound && sound !== "none") {
      try {
        await playSound(sound);
      } catch (e) {
        console.log(`[Notifier] Error playing audio`, e);
      }
    }
  }

  async webPushSubscription(hasWebPushTopics) {
    if (!this.pushPossible()) {
      throw new Error("Unsupported or denied");
    }
    const pushManager = await this.pushManager();
    const existingSubscription = await pushManager.getSubscription();
    if (existingSubscription) {
      return existingSubscription;
    }

    // Create a new subscription only if there are new topics to subscribe to. It is possible that Web Push
    // was previously enabled and then disabled again in which case there would be an existingSubscription.
    // If, however, it was _not_ enabled previously, we create a new subscription if it is now enabled.

    if (hasWebPushTopics) {
      return pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlB64ToUint8Array(config.web_push_public_key),
      });
    }

    return undefined;
  }

  async pushManager() {
    return (await this.serviceWorkerRegistration()).pushManager;
  }

  async serviceWorkerRegistration() {
    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
      throw new Error("No service worker registration found");
    }
    return registration;
  }

  notRequested() {
    return this.supported() && Notification.permission === "default";
  }

  granted() {
    return this.supported() && Notification.permission === "granted";
  }

  denied() {
    return this.supported() && Notification.permission === "denied";
  }

  async maybeRequestPermission() {
    if (!this.supported()) {
      return false;
    }

    return new Promise((resolve) => {
      Notification.requestPermission((permission) => {
        resolve(permission === "granted");
      });
    });
  }

  supported() {
    return this.browserSupported() && this.contextSupported();
  }

  browserSupported() {
    return "Notification" in window;
  }

  pushSupported() {
    return config.enable_web_push && "serviceWorker" in navigator && "PushManager" in window;
  }

  pushPossible() {
    return this.pushSupported() && this.contextSupported() && this.granted() && !this.iosSupportedButInstallRequired();
  }

  /**
   * Returns true if this is a HTTPS site, or served over localhost. Otherwise the Notification API
   * is not supported, see https://developer.mozilla.org/en-US/docs/Web/API/notification
   */
  contextSupported() {
    return window.location.protocol === "https:" || window.location.hostname.match("^127.") || window.location.hostname === "localhost";
  }

  iosSupportedButInstallRequired() {
    return this.pushSupported() && "standalone" in window.navigator && window.navigator.standalone === false;
  }
}

const notifier = new Notifier();
export default notifier;
