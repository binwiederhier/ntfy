import sessionReplica from "./SessionReplica";

/**
 * Manages the logged-in user's session and access token.
 * The session replica is stored in IndexedDB so that the service worker can access it.
 */
class Session {
  constructor(replica) {
    this.replica = replica;
  }

  store(username, token) {
    localStorage.setItem("user", username);
    localStorage.setItem("token", token);
    this.replica.store(username, token);
  }

  reset() {
    localStorage.removeItem("user");
    localStorage.removeItem("token");
    this.replica.reset();
  }

  resetAndRedirect(url) {
    this.reset();
    window.location.href = url;
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

const session = new Session(sessionReplica);
export default session;
