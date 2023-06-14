import Dexie from "dexie";

/**
 * Manages the logged-in user's session and access token.
 * The session replica is stored in IndexedDB so that the service worker can access it.
 */
class Session {
  constructor() {
    const db = new Dexie("session-replica");
    db.version(1).stores({
      kv: "&key",
    });
    this.db = db;
  }

  async store(username, token) {
    await this.db.kv.bulkPut([
      { key: "user", value: username },
      { key: "token", value: token },
    ]);
    localStorage.setItem("user", username);
    localStorage.setItem("token", token);
  }

  async resetAndRedirect(url) {
    await this.db.delete();
    localStorage.removeItem("user");
    localStorage.removeItem("token");
    window.location.href = url;
  }

  async usernameAsync() {
    return (await this.db.kv.get({ key: "user" }))?.value;
  }

  exists() {
    return this.username() && this.token();
  }

  username() {
    return localStorage.getItem("user");
  }

  token() {
    return localStorage.getItem("token");
  }
}

const session = new Session();
export default session;
