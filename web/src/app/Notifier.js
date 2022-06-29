import {formatMessage, formatTitleWithDefault, openUrl, playSound, topicDisplayName, topicShortUrl} from "./utils";
import prefs from "./Prefs";
import subscriptionManager from "./SubscriptionManager";
import logo from "../img/ntfy.png";

/**
 * The notifier is responsible for displaying desktop notifications. Note that not all modern browsers
 * support this; most importantly, all iOS browsers do not support window.Notification.
 */
class Notifier {
    async notify(subscriptionId, notification, onClickFallback) {
        if (!this.supported()) {
            return;
        }
        const subscription = await subscriptionManager.get(subscriptionId);
        const shouldNotify = await this.shouldNotify(subscription, notification);
        if (!shouldNotify) {
            return;
        }
        const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
        const displayName = topicDisplayName(subscription);
        const message = formatMessage(notification);
        const title = formatTitleWithDefault(notification, displayName);

        // Show notification
        console.log(`[Notifier, ${shortUrl}] Displaying notification ${notification.id}: ${message}`);
        const n = new Notification(title, {
            body: message,
            icon: logo
        });
        if (notification.click) {
            n.onclick = (e) => openUrl(notification.click);
        } else {
            n.onclick = () => onClickFallback(subscription);
        }

        // Play sound
        const sound = await prefs.sound();
        if (sound && sound !== "none") {
            try {
                await playSound(sound);
            } catch (e) {
                console.log(`[Notifier, ${shortUrl}] Error playing audio`, e);
            }
        }
    }

    granted() {
        return this.supported() && Notification.permission === 'granted';
    }

    maybeRequestPermission(cb) {
        if (!this.supported()) {
            cb(false);
            return;
        }
        if (!this.granted()) {
            Notification.requestPermission().then((permission) => {
                const granted = permission === 'granted';
                cb(granted);
            });
        }
    }

    async shouldNotify(subscription, notification) {
        if (subscription.mutedUntil === 1) {
            return false;
        }
        const priority = (notification.priority) ? notification.priority : 3;
        const minPriority = await prefs.minPriority();
        if (priority < minPriority) {
            return false;
        }
        return true;
    }

    supported() {
        return this.browserSupported() && this.contextSupported();
    }

    browserSupported() {
        return 'Notification' in window;
    }

    /**
     * Returns true if this is a HTTPS site, or served over localhost. Otherwise the Notification API
     * is not supported, see https://developer.mozilla.org/en-US/docs/Web/API/notification
     */
    contextSupported() {
        return location.protocol === 'https:'
            || location.hostname.match('^127.')
            || location.hostname === 'localhost';
    }
}

const notifier = new Notifier();
export default notifier;
