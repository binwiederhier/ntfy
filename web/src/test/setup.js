// Minimal browser-global stubs so the pure-logic and API modules import + run under the node
// environment, without pulling in jsdom. utils.js -> config.js does `const { config } = window;`
// at import time, and config.js falls back to window.location.origin when base_url is empty.
const config = { base_url: "https://ntfy.sh", disallowed_topics: ["app", "account", "settings"] };

globalThis.window = {
  location: { origin: "https://ntfy.sh" },
  config,
  atob: globalThis.atob, // urlB64ToUint8Array uses window.atob; Node provides global atob
};

// In the browser, `window` IS the global object, so `window.config` is also reachable as a bare
// `config` identifier -- Api.js/AccountApi.js/UserManager.js/SubscriptionManager.js rely on that.
// Node's `window` is just a plain object, so expose the same object as a real global too.
globalThis.config = config;

// utils.js -> Prefs.js -> db.js -> Session.username() reads localStorage at module load.
// Node has no localStorage; an in-memory stand-in is enough for the pure-logic tests.
const store = new Map();
globalThis.localStorage = {
  getItem: (key) => (store.has(key) ? store.get(key) : null),
  setItem: (key, value) => store.set(key, String(value)),
  removeItem: (key) => store.delete(key),
  clear: () => store.clear(),
};
