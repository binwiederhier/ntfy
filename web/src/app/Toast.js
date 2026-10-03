/**
 * Tiny in-app notice bus. Listeners are called with a plain text notice whenever the app wants to
 * surface something transient. ToastHost renders the notices as a Snackbar; the Discover view also
 * listens so it can refresh its list when a new shared topic is announced on the ~directory feed.
 */
class Toast {
  constructor() {
    this.listeners = [];
  }

  registerListener(listener) {
    this.listeners.push(listener);
  }

  resetListener(listener) {
    this.listeners = this.listeners.filter((l) => l !== listener);
  }

  show(message) {
    this.listeners.forEach((listener) => {
      try {
        listener(message);
      } catch (e) {
        console.error(`[Toast] Listener error`, e);
      }
    });
  }
}

const toast = new Toast();
export default toast;
