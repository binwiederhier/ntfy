import {
    accountAccessSingleUrl,
    accountAccessUrl,
    accountPasswordUrl,
    accountSettingsUrl,
    accountSubscriptionSingleUrl,
    accountSubscriptionUrl,
    accountTokenUrl,
    accountUrl,
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
        const url = accountTokenUrl(config.baseUrl);
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
        const url = accountTokenUrl(config.baseUrl);
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
        const url = accountUrl(config.baseUrl);
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
        const url = accountUrl(config.baseUrl);
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

    async delete() {
        const url = accountUrl(config.baseUrl);
        console.log(`[AccountApi] Deleting user account ${url}`);
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

    async changePassword(newPassword) {
        const url = accountPasswordUrl(config.baseUrl);
        console.log(`[AccountApi] Changing account password ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: newPassword
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async extendToken() {
        const url = accountTokenUrl(config.baseUrl);
        console.log(`[AccountApi] Extending user access token ${url}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async updateSettings(payload) {
        const url = accountSettingsUrl(config.baseUrl);
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
        const url = accountSubscriptionUrl(config.baseUrl);
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
        const url = accountSubscriptionSingleUrl(config.baseUrl, remoteId);
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
        const url = accountSubscriptionSingleUrl(config.baseUrl, remoteId);
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

    async upsertAccess(topic, everyone) {
        const url = accountAccessUrl(config.baseUrl);
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
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async deleteAccess(topic) {
        const url = accountAccessSingleUrl(config.baseUrl, topic);
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

    async sync() {
        try {
            if (!session.token()) {
                return null;
            }
            console.log(`[AccountApi] Syncing account`);
            const remoteAccount = await this.get();
            if (remoteAccount.language) {
                await i18n.changeLanguage(remoteAccount.language);
            }
            if (remoteAccount.notification) {
                if (remoteAccount.notification.sound) {
                    await prefs.setSound(remoteAccount.notification.sound);
                }
                if (remoteAccount.notification.delete_after) {
                    await prefs.setDeleteAfter(remoteAccount.notification.delete_after);
                }
                if (remoteAccount.notification.min_priority) {
                    await prefs.setMinPriority(remoteAccount.notification.min_priority);
                }
            }
            if (remoteAccount.subscriptions) {
                await subscriptionManager.syncFromRemote(remoteAccount.subscriptions);
            }
            return remoteAccount;
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

export class UsernameTakenError extends Error {
    constructor(username) {
        super("Username taken");
        this.username = username;
    }
}

export class AccountCreateLimitReachedError extends Error {
    constructor() {
        super("Account creation limit reached");
    }
}

export class UnauthorizedError extends Error {
    constructor() {
        super("Unauthorized");
    }
}

const accountApi = new AccountApi();
export default accountApi;
