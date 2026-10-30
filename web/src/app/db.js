import Dexie from "dexie";
import session from "./Session";

// Uses Dexie.js
// https://dexie.org/docs/API-Reference#quick-reference
//
// Notes:
// - As per docs, we only declare the indexable columns, not all columns

const createDatabase = (username) => {
  const dbName = username ? `ntfy-${username}` : "ntfy"; // IndexedDB database is based on the logged-in user
  const db = new Dexie(dbName);

  db.version(3).stores({
    subscriptions: "&id,baseUrl,[baseUrl+mutedUntil]",
    notifications: "&id,sequenceId,subscriptionId,time,new,[subscriptionId+new],[subscriptionId+sequenceId]",
    users: "&baseUrl,username",
    prefs: "&key",
  });

  // When another connection (e.g., service worker or another tab) wants to upgrade,
  // close this connection gracefully to allow the upgrade to proceed
  db.on("versionchange", () => {
    console.log("[db] versionchange event: closing database");
    db.close();
  });

  return db;
};

export const dbAsync = async () => {
  const username = await session.usernameAsync();
  return createDatabase(username);
};

const db = () => createDatabase(session.username());

export default db;
