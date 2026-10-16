import * as React from "react";
import { createContext, useContext, useEffect } from "react";
import { useLiveQuery } from "dexie-react-hooks";
import prefs, { PREF_DEFAULTS } from "../app/Prefs";

// A CACHE of the user's prefs -- not the source of truth (that's the `prefs` IndexedDB table, via
// Prefs.js). Preloaded once in Layout so Settings renders instantly, and written through to
// localStorage on every change so (a) Settings is instant even on a cold load and (b) the inline
// splash script in index.html can read the theme synchronously before the bundle loads. The
// "prefcache" key is duplicated in that script -- keep them in sync.
const PREFCACHE_LOCALSTORAGE_KEY = "prefcache";

const PrefCacheContext = createContext(undefined);

// Synchronous fallback before the live query resolves. Merged over PREF_DEFAULTS so a newly-added
// pref still has a value.
const readPersistedCache = () => {
  try {
    const raw = localStorage.getItem(PREFCACHE_LOCALSTORAGE_KEY);
    if (raw) {
      return { ...PREF_DEFAULTS, ...JSON.parse(raw) };
    }
  } catch (e) {
    // malformed or unavailable storage -- fall back to defaults
  }
  return PREF_DEFAULTS;
};

export const PrefCacheProvider = ({ children }) => {
  const cache = useLiveQuery(async () => ({
    sound: await prefs.sound(),
    minPriority: await prefs.minPriority(),
    deleteAfter: await prefs.deleteAfter(),
    theme: await prefs.theme(),
    dateFormat: await prefs.dateFormat(),
    timeFormat: await prefs.timeFormat(),
    webPushEnabled: await prefs.webPushEnabled(),
    notificationAlertDismissed: await prefs.notificationAlertDismissed(),
  }));

  // Write through to localStorage on change (prefs change rarely).
  useEffect(() => {
    if (cache !== undefined) {
      try {
        localStorage.setItem(PREFCACHE_LOCALSTORAGE_KEY, JSON.stringify(cache));
      } catch (e) {
        // localStorage may be unavailable (private mode) -- the cache just isn't persisted
      }
    }
  }, [cache]);

  return <PrefCacheContext.Provider value={cache}>{children}</PrefCacheContext.Provider>;
};

// Live context once resolved; else the synchronous localStorage snapshot (instant on cold load);
// else defaults.
export const usePrefCache = () => useContext(PrefCacheContext) ?? readPersistedCache();
