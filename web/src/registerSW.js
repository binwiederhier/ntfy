// eslint-disable-next-line import/no-unresolved
import { registerSW as viteRegisterSW } from "virtual:pwa-register";

// fetch new sw every hour, i.e. update app every hour while running
const intervalMS = 60 * 60 * 1000;

// https://vite-pwa-org.netlify.app/guide/periodic-sw-updates.html
const registerSW = () => {
  console.log("[ServiceWorker] Registering service worker");
  if (!("serviceWorker" in navigator)) {
    console.warn("[ServiceWorker] Service workers not supported");
    return;
  }

  viteRegisterSW({
    onRegisteredSW(swUrl, registration) {
      console.log("[ServiceWorker] Registered:", { swUrl, registration });

      if (!registration) {
        console.warn("[ServiceWorker] No registration returned");
        return;
      }

      setInterval(async () => {
        if (registration.installing || navigator?.onLine === false) return;

        const resp = await fetch(swUrl, {
          cache: "no-store",
          headers: {
            cache: "no-store",
            "cache-control": "no-cache",
          },
        });

        if (resp?.status === 200) {
          console.log("[ServiceWorker] Updating service worker");
          await registration.update();
        }
      }, intervalMS);
    },
    onRegisterError(error) {
      console.error("[ServiceWorker] Registration error:", error);
    },
  });
};

export default registerSW;
