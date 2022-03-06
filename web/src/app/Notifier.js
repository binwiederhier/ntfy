import {formatMessage, formatTitleWithDefault, openUrl, playSound, topicShortUrl} from "./utils";
import prefs from "./Prefs";
import subscriptionManager from "./SubscriptionManager";

class Notifier {
    async notify(subscriptionId, notification, onClickFallback) {
        const subscription = await subscriptionManager.get(subscriptionId);
        const shouldNotify = await this.shouldNotify(subscription, notification);
        if (!shouldNotify) {
            return;
        }
        const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
        const message = formatMessage(notification);
        const title = formatTitleWithDefault(notification, shortUrl);

        // Show notification
        console.log(`[Notifier, ${shortUrl}] Displaying notification ${notification.id}: ${message}`);
        const n = new Notification(title, {
            body: message,
            icon: '/static/img/favicon.png'
        });
        if (notification.click) {
            n.onclick = (e) => openUrl(notification.click);
        } else {
            n.onclick = onClickFallback;
        }

        // Play sound
        const sound = await prefs.sound();
        if (sound && sound !== "none") {
            try {
                await playSound(sound);
            } catch (e) {
                console.log(`[Notifier, ${shortUrl}] Error playing audio`, e);
                // FIXME show no sound allowed popup
            }
        }
    }

    granted() {
        return Notification.permission === 'granted';
    }

    maybeRequestPermission(cb) {
        if (!this.granted()) {
            Notification.requestPermission().then((permission) => {
                const granted = permission === 'granted';
                cb(granted);
            });
        }
    }

    async shouldNotify(subscription, notification) {
        const priority = (notification.priority) ? notification.priority : 3;
        const minPriority = await prefs.minPriority();
        if (priority < minPriority) {
            return false;
        }
        return true;
    }
}

const notifier = new Notifier();
export default notifier;
