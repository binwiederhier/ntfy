import Dexie from 'dexie';
import session from "./Session";

// Uses Dexie.js
// https://dexie.org/docs/API-Reference#quick-reference
//
// Notes:
// - As per docs, we only declare the indexable columns, not all columns

const dbName = (session.username()) ? `ntfy-${session.username()}` : "ntfy";
const db = new Dexie(dbName);

db.version(1).stores({
    subscriptions: '&id,baseUrl',
    notifications: '&id,subscriptionId,time,new,[subscriptionId+new]', // compound key for query performance
    users: '&baseUrl,username',
    prefs: '&key'
});

export default db;
