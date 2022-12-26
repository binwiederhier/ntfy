import {
    accountPasswordUrl,
    accountSettingsUrl,
    accountSubscriptionSingleUrl,
    accountSubscriptionUrl,
    accountTokenUrl,
    accountUrl,
    fetchLinesIterator,
    maybeWithBasicAuth,
    maybeWithBearerAuth,
    topicShortUrl,
    topicUrl,
    topicUrlAuth,
    topicUrlJsonPoll,
    topicUrlJsonPollWithSince
} from "./utils";
import userManager from "./UserManager";
import session from "./Session";
import subscriptionManager from "./SubscriptionManager";

const delayMillis = 45000; // 45 seconds
const intervalMillis = 900000; // 15 minutes

class AccountApi {
    constructor() {
        this.timer = null;
    }

    async login(user) {
        const url = accountTokenUrl(config.baseUrl);
        console.log(`[AccountApi] Checking auth for ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBasicAuth({}, user)
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

    async logout(token) {
        const url = accountTokenUrl(config.baseUrl);
        console.log(`[AccountApi] Logging out from ${url} using token ${token}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, token)
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
            headers: maybeWithBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const account = await response.json();
        console.log(`[AccountApi] Account`, account);
        return account;
    }

    async delete() {
        const url = accountUrl(config.baseUrl);
        console.log(`[AccountApi] Deleting user account ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, session.token())
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
            headers: maybeWithBearerAuth({}, session.token()),
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
            headers: maybeWithBearerAuth({}, session.token())
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
            headers: maybeWithBearerAuth({}, session.token()),
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
            headers: maybeWithBearerAuth({}, session.token()),
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
            headers: maybeWithBearerAuth({}, session.token()),
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
            headers: maybeWithBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
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
