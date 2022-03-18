import db from "./db";

class UserManager {
    async all() {
        return db.users.toArray();
    }

    async get(baseUrl) {
        return db.users.get(baseUrl);
    }

    async save(user) {
        await db.users.put(user);
    }

    async delete(baseUrl) {
        await db.users.delete(baseUrl);
    }
}

const userManager = new UserManager();
export default userManager;
