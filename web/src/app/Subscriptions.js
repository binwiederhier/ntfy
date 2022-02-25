class Subscriptions {
    constructor() {
        this.subscriptions = new Map();
    }

    add(subscription) {
        this.subscriptions.set(subscription.id, subscription);
        return this;
    }

    get(subscriptionId) {
        const subscription = this.subscriptions.get(subscriptionId);
        return (subscription) ? subscription : null;
    }

    update(subscription) {
        return this.add(subscription);
    }

    remove(subscriptionId) {
        this.subscriptions.delete(subscriptionId);
        return this;
    }

    forEach(cb) {
        this.subscriptions.forEach((value, key) => cb(key, value));
    }

    map(cb) {
        return Array.from(this.subscriptions.values())
            .map(subscription => cb(subscription.id, subscription));
    }

    ids() {
        return Array.from(this.subscriptions.keys());
    }

    firstOrNull() {
        const first = this.subscriptions.values().next().value;
        return (first) ? first : null;
    }

    size() {
        return this.subscriptions.size;
    }

    clone() {
        const c = new Subscriptions();
        c.subscriptions = new Map(this.subscriptions);
        return c;
    }
}

export default Subscriptions;
