import Dexie from "dexie";

// Store to IndexedDB as well so that the
// service worker can access it
// TODO: Probably make everything depend on this and not use localStorage,
// but that's a larger refactoring effort for another PR

class SessionReplica {
  constructor() {
    const db = new Dexie("session-replica");

    db.version(1).stores({
      keyValueStore: "&key",
    });

    this.db = db;
  }

  async store(username, token) {
    try {
      await this.db.keyValueStore.bulkPut([
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
    return (await this.db.keyValueStore.get({ key: "user" }))?.value;
  }
}

const sessionReplica = new SessionReplica();
export default sessionReplica;
