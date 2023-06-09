import Dexie from "dexie";

/**
 * Replica of the session in IndexedDB. This is used by the service
 * worker to access the session. This is a bit of a hack.
 */
class SessionReplica {
  constructor() {
    const db = new Dexie("session-replica");
    db.version(1).stores({
      kv: "&key",
    });
    this.db = db;
  }

  async store(username, token) {
    try {
      await this.db.kv.bulkPut([
        { key: "user", value: username },
        { key: "token", value: token },
      ]);
    } catch (e) {
      console.error("[Session] Error replicating session to IndexedDB", e);
    }
  }

  async reset() {
    try {
      await this.db.delete();
    } catch (e) {
      console.error("[Session] Error resetting session on IndexedDB", e);
    }
  }

  async username() {
    return (await this.db.kv.get({ key: "user" }))?.value;
  }
}

const sessionReplica = new SessionReplica();
export default sessionReplica;
