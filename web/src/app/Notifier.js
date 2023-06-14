import { openUrl, playSound, topicDisplayName, topicShortUrl, urlB64ToUint8Array } from "./utils";
import { formatMessage, formatTitleWithDefault } from "./notificationUtils";
import prefs from "./Prefs";
import logo from "../img/ntfy.png";

/**
 * The notifier is responsible for displaying desktop notifications. Note that not all modern browsers
 * support this; most importantly, all iOS browsers do not support window.Notification.
 */
class Notifier {
  async notify(subscription, notification, onClickFallback) {
    if (!this.supported()) {
      return;
    }

    const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
    const displayName = topicDisplayName(subscription);
    const message = formatMessage(notification);
    const title = formatTitleWithDefault(notification, displayName);
    const image = notification.attachment?.name.match(/\.(png|jpe?g|gif|webp)$/i) ? notification.attachment.url : undefined;

    // Show notification
    console.log(`[Notifier, ${shortUrl}] Displaying notification ${notification.id}: ${message}`);
    // Please update sw.js if formatting changes
    const n = new Notification(title, {
      body: message,
      tag: subscription.id,
      icon: image ?? logo,
      image,
      timestamp: message.time * 1_000,
    });
    if (notification.click) {
      n.onclick = () => openUrl(notification.click);
    } else {
      n.onclick = () => onClickFallback(subscription);
    }
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

  async webPushSubscription() {
    if (!this.pushPossible()) {
      throw new Error("Unsupported or denied");
    }
    const pushManager = await this.pushManager();
    const existingSubscription = await pushManager.getSubscription();
    if (existingSubscription) {
      return existingSubscription;
    }

    // Create a new subscription only if Web Push is enabled. It is possible that Web Push
    // was previously enabled and then disabled again in which case there would be an existingSubscription.
    // If, however, it was _not_ enabled previously, we create a new subscription if it is now enabled.

    if (await this.pushEnabled()) {
      return pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlB64ToUint8Array(config.web_push_public_key),
      });
    }

    return undefined;
  }

  async pushManager() {
    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
      throw new Error("No service worker registration found");
    }
    return registration.pushManager;
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

  async pushEnabled() {
    const enabled = await prefs.webPushEnabled();
    return this.pushPossible() && enabled;
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
