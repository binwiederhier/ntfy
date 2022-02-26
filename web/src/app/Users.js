class Users {
    constructor() {
        this.loaded = false; // FIXME I hate this
        this.users = new Map();
    }

    add(user) {
        this.users.set(user.baseUrl, user);
        return this;
    }

    get(baseUrl) {
        const user = this.users.get(baseUrl);
        return (user) ? user : null;
    }

    update(user) {
        return this.add(user);
    }

    remove(baseUrl) {
        this.users.delete(baseUrl);
        return this;
    }

    map(cb) {
        return Array.from(this.users.values()).map(cb);
    }

    clone() {
        const c = new Users();
        c.loaded = this.loaded;
        c.users = new Map(this.users);
        return c;
    }
}

export default Users;
