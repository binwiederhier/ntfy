import {
    accountBillingPortalUrl,
    accountBillingSubscriptionUrl,
    accountPasswordUrl,
    accountReservationSingleUrl,
    accountReservationUrl,
    accountSettingsUrl,
    accountSubscriptionSingleUrl,
    accountSubscriptionUrl,
    accountTokenUrl,
    accountUrl,
    tiersUrl,
    withBasicAuth,
    withBearerAuth
} from "./utils";
import session from "./Session";
import subscriptionManager from "./SubscriptionManager";
import i18n from "i18next";
import prefs from "./Prefs";
import routes from "../components/routes";

const delayMillis = 45000; // 45 seconds
const intervalMillis = 900000; // 15 minutes

class AccountApi {
    constructor() {
        this.timer = null;
        this.listener = null; // Fired when account is fetched from remote
    }

    registerListener(listener) {
        this.listener = listener;
    }

    resetListener() {
        this.listener = null;
    }

    async login(user) {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Checking auth for ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBasicAuth({}, user.username, user.password)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const json = await response.json();
        if (!json.token) {
            throw new Error(`Unexpected server response: Cannot find token`);
        }
        return json.token;
    }

    async logout() {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Logging out from ${url} using token ${session.token()}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async create(username, password) {
        const url = accountUrl(config.base_url);
        const body = JSON.stringify({
            username: username,
            password: password
        });
        console.log(`[AccountApi] Creating user account ${url}`);
        const response = await fetch(url, {
            method: "POST",
            body: body
        });
        if (response.status === 409) {
            throw new UsernameTakenError(username);
        } else if (response.status === 429) {
            throw new AccountCreateLimitReachedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async get() {
        const url = accountUrl(config.base_url);
        console.log(`[AccountApi] Fetching user account ${url}`);
        const response = await fetch(url, {
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const account = await response.json();
        console.log(`[AccountApi] Account`, account);
        if (this.listener) {
            this.listener(account);
        }
        return account;
    }

    async delete(password) {
        const url = accountUrl(config.base_url);
        console.log(`[AccountApi] Deleting user account ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: password
            })
        });
        if (response.status === 400) {
            throw new IncorrectPasswordError();
        } else if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async changePassword(currentPassword, newPassword) {
        const url = accountPasswordUrl(config.base_url);
        console.log(`[AccountApi] Changing account password ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: currentPassword,
                new_password: newPassword
            })
        });
        if (response.status === 400) {
            throw new IncorrectPasswordError();
        } else if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async createToken(label, expires) {
        const url = accountTokenUrl(config.base_url);
        const body = {
            label: label,
            expires: (expires > 0) ? Math.floor(Date.now() / 1000) + expires : 0
        };
        console.log(`[AccountApi] Creating user access token ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify(body)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async updateToken(token, label, expires) {
        const url = accountTokenUrl(config.base_url);
        const body = {
            token: token,
            label: label
        };
        if (expires > 0) {
            body.expires = Math.floor(Date.now() / 1000) + expires;
        }
        console.log(`[AccountApi] Creating user access token ${url}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify(body)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async extendToken() {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Extending user access token ${url}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                token: session.token(),
                expires: Math.floor(Date.now() / 1000) + 6220800 // FIXME
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async deleteToken(token) {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Deleting user access token ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({"X-Token": token}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async updateSettings(payload) {
        const url = accountSettingsUrl(config.base_url);
        const body = JSON.stringify(payload);
        console.log(`[AccountApi] Updating user account ${url}: ${body}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async addSubscription(payload) {
        const url = accountSubscriptionUrl(config.base_url);
        const body = JSON.stringify(payload);
        console.log(`[AccountApi] Adding user subscription ${url}: ${body}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const subscription = await response.json();
        console.log(`[AccountApi] Subscription`, subscription);
        return subscription;
    }

    async updateSubscription(remoteId, payload) {
        const url = accountSubscriptionSingleUrl(config.base_url, remoteId);
        const body = JSON.stringify(payload);
        console.log(`[AccountApi] Updating user subscription ${url}: ${body}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const subscription = await response.json();
        console.log(`[AccountApi] Subscription`, subscription);
        return subscription;
    }

    async deleteSubscription(remoteId) {
        const url = accountSubscriptionSingleUrl(config.base_url, remoteId);
        console.log(`[AccountApi] Removing user subscription ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async upsertReservation(topic, everyone) {
        const url = accountReservationUrl(config.base_url);
        console.log(`[AccountApi] Upserting user access to topic ${topic}, everyone=${everyone}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                topic: topic,
                everyone: everyone
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status === 409) {
            throw new TopicReservedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async deleteReservation(topic) {
        const url = accountReservationSingleUrl(config.base_url, topic);
        console.log(`[AccountApi] Removing topic reservation ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async billingTiers() {
        const url = tiersUrl(config.base_url);
        console.log(`[AccountApi] Fetching billing tiers`);
        const response = await fetch(url); // No auth needed!
        if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        return await response.json();
    }

    async createBillingSubscription(tier) {
        console.log(`[AccountApi] Creating billing subscription with ${tier}`);
        return await this.upsertBillingSubscription("POST", tier)
    }

    async updateBillingSubscription(tier) {
        console.log(`[AccountApi] Updating billing subscription with ${tier}`);
        return await this.upsertBillingSubscription("PUT", tier)
    }

    async upsertBillingSubscription(method, tier) {
        const url = accountBillingSubscriptionUrl(config.base_url);
        const response = await fetch(url, {
            method: method,
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                tier: tier
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        return await response.json();
    }

    async deleteBillingSubscription() {
        const url = accountBillingSubscriptionUrl(config.base_url);
        console.log(`[AccountApi] Cancelling billing subscription`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async createBillingPortalSession() {
        const url = accountBillingPortalUrl(config.base_url);
        console.log(`[AccountApi] Creating billing portal session`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        return await response.json();
    }

    async sync() {
        try {
            if (!session.token()) {
                return null;
            }
            console.log(`[AccountApi] Syncing account`);
            const account = await this.get();
            if (account.language) {
                await i18n.changeLanguage(account.language);
            }
            if (account.notification) {
                if (account.notification.sound) {
                    await prefs.setSound(account.notification.sound);
                }
                if (account.notification.delete_after) {
                    await prefs.setDeleteAfter(account.notification.delete_after);
                }
                if (account.notification.min_priority) {
                    await prefs.setMinPriority(account.notification.min_priority);
                }
            }
            if (account.subscriptions) {
                await subscriptionManager.syncFromRemote(account.subscriptions, account.reservations);
            }
            return account;
        } catch (e) {
            console.log(`[AccountApi] Error fetching account`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
        }
    }

    startWorker() {
        if (this.timer !== null) {
            return;
        }
        console.log(`[AccountApi] Starting worker`);
        this.timer = setInterval(() => this.runWorker(), intervalMillis);
        setTimeout(() => this.runWorker(), delayMillis);
    }

    async runWorker() {
        if (!session.token()) {
            return;
        }
        console.log(`[AccountApi] Extending user access token`);
        try {
            await this.extendToken();
        } catch (e) {
            console.log(`[AccountApi] Error extending user access token`, e);
        }
    }
}

// Maps to user.Role in user/types.go
export const Role = {
    ADMIN: "admin",
    USER: "user"
};

// Maps to server.visitorLimitBasis in server/visitor.go
export const LimitBasis = {
    IP: "ip",
    TIER: "tier"
};

// Maps to stripe.SubscriptionStatus
export const SubscriptionStatus = {
    ACTIVE: "active",
    PAST_DUE: "past_due"
};

// Maps to user.Permission in user/types.go
export const Permission = {
    READ_WRITE: "read-write",
    READ_ONLY: "read-only",
    WRITE_ONLY: "write-only",
    DENY_ALL: "deny-all"
};

export class UsernameTakenError extends Error {
    constructor(username) {
        super("Username taken");
        this.username = username;
    }
}

export class TopicReservedError extends Error {
    constructor(topic) {
        super("Topic already reserved");
        this.topic = topic;
    }
}

export class AccountCreateLimitReachedError extends Error {
    constructor() {
        super("Account creation limit reached");
    }
}

export class IncorrectPasswordError extends Error {
    constructor() {
        super("Password incorrect");
    }
}

export class UnauthorizedError extends Error {
    constructor() {
        super("Unauthorized");
    }
}

const accountApi = new AccountApi();
export default accountApi;
