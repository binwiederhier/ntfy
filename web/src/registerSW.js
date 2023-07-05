// eslint-disable-next-line import/no-unresolved
import { registerSW as viteRegisterSW } from "virtual:pwa-register";

// fetch new sw every hour, i.e. update app every hour while running
const intervalMS = 60 * 60 * 1000;

// https://vite-pwa-org.netlify.app/guide/periodic-sw-updates.html
const registerSW = () =>
  viteRegisterSW({
    onRegisteredSW(swUrl, registration) {
      if (!registration) {
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

        if (resp?.status === 200) await registration.update();
      }, intervalMS);
    },
  });

export default registerSW;
