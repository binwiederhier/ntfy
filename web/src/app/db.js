import Dexie from "dexie";
import session from "./Session";
import sessionReplica from "./SessionReplica";

// Uses Dexie.js
// https://dexie.org/docs/API-Reference#quick-reference
//
// Notes:
// - As per docs, we only declare the indexable columns, not all columns

const createDatabase = (username) => {
  const dbName = username ? `ntfy-${username}` : "ntfy"; // IndexedDB database is based on the logged-in user
  const db = new Dexie(dbName);

  db.version(1).stores({
    subscriptions: "&id,baseUrl,[baseUrl+mutedUntil]",
    notifications: "&id,subscriptionId,time,new,[subscriptionId+new]", // compound key for query performance
    users: "&baseUrl,username",
    prefs: "&key",
  });

  return db;
};

export const dbAsync = async () => {
  const username = await sessionReplica.username();
  return createDatabase(username);
};

export const db = () => createDatabase(session.username());

export default db;
