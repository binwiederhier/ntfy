import Dexie from 'dexie';

// Uses Dexie.js
// https://dexie.org/docs/API-Reference#quick-reference
//
// Notes:
// - As per docs, we only declare the indexable columns, not all columns

const db = new Dexie('ntfy');

db.version(1).stores({
    subscriptions: '&id,baseUrl',
    notifications: '&id,subscriptionId,time',
    users: '&baseUrl,username',
    prefs: '&key'
});

export default db;
