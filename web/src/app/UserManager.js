import db from "./db";
import session from "./Session";

class UserManager {
  async all() {
    const users = await db.users.toArray();
    if (session.exists()) {
      users.unshift(this.localUser());
    }
    return users;
  }

  async get(baseUrl) {
    if (session.exists() && baseUrl === config.base_url) {
      return this.localUser();
    }
    return db.users.get(baseUrl);
  }

  async save(user) {
    if (session.exists() && user.baseUrl === config.base_url) {
      return;
    }
    await db.users.put(user);
  }

  async delete(baseUrl) {
    if (session.exists() && baseUrl === config.base_url) {
      return;
    }
    await db.users.delete(baseUrl);
  }

  localUser() {
    if (!session.exists()) {
      return null;
    }
    return {
      baseUrl: config.base_url,
      username: session.username(),
      token: session.token(), // Not "password"!
    };
  }
}

const userManager = new UserManager();
export default userManager;
